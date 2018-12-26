// Package wad provides access to Doom's data archives also known as WAD files.
// The file format is documented in The Unofficial DOOM Specs:
// http://www.gamers.org/dhs/helpdocs/dmsp1666.html
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"sort"
	"unsafe"
)

type String8 [8]byte

// WAD is a struct that represents Doom's data archive that contains
// graphics, sounds, and level data. The data is organized as named
// lumps.
type WAD struct {
	header                  *header
	file                    *os.File
	pnames                  []String8
	patches                 map[string]Image
	TransparentPaletteIndex byte
	Playpal                 *Playpal
	textures                map[string]Texture
	flats                   map[string]Flat
	levels                  map[string]int
	lumps                   map[string]int
	lumpInfos               []lumpInfo
}

type header struct {
	Magic        [4]byte
	NumLumps     int32
	InfoTableOfs int32
}

type lumpInfo struct {
	Filepos int32
	Size    int32
	Name    String8
}

type Texture struct {
	Header  *TextureHeader
	Patches []Patch
}

type TextureHeader struct {
	TexName         String8
	Masked          int32
	Width           int16
	Height          int16
	ColumnDirectory int32
	NumPatches      int16
}

type Patch struct {
	XOffset     int16
	YOffset     int16
	PNameNumber int16
	Stepdir     int16
	ColorMap    int16
}

type Image struct {
	Width  int
	Height int
	Pixels []byte
}

type PictureHeader struct {
	Width      int16
	Height     int16
	LeftOffset int16
	TopOffset  int16
}

type Flat struct {
	Data []byte
}

type Level struct {
	Things   []Thing
	Linedefs []Linedef
	Sidedefs []Sidedef
	Vertexes []Vertex
	Segs     []Seg
	SSectors []SSector
	Nodes    []Node
	Sectors  []Sector
}

type Thing struct {
	XPosition int16
	YPosition int16
	Angle     int16
	Type      int16
	Options   int16
}

type Linedef struct {
	VertexStart  int16
	VertexEnd    int16
	Flags        int16
	Function     int16
	Tag          int16
	SidedefRight int16
	SidedefLeft  int16
}

type Sidedef struct {
	XOffset       int16
	YOffset       int16
	UpperTexture  String8
	LowerTexture  String8
	MiddleTexture String8
	SectorRef     int16
}

type Vertex struct {
	XCoord int16
	YCoord int16
}

type Seg struct {
	VertexStart int16
	VertexEnd   int16
	Bams        int16
	LineNum     int16
	Segside     int16
	Segoffset   int16
}

type SSector struct {
	Numsegs  int16
	StartSeg int16
}

type BBox struct {
	Top    int16
	Bottom int16
	Left   int16
	Right  int16
}

type Node struct {
	X     int16
	Y     int16
	DX    int16
	DY    int16
	BBox  [2]BBox
	Child [2]int16
}

type Sector struct {
	FloorHeight   int16
	CeilingHeight int16
	Floorpic      String8
	Ceilingpic    String8
	Lightlevel    int16
	SpecialSector int16
	Tag           int16
}

type Reject struct {
}

type Blockmap struct {
}

type RGB struct {
	Red   uint8
	Green uint8
	Blue  uint8
}

type Palette struct {
	Table [256]RGB
}

type Playpal struct {
	Palettes [14]Palette
}

func ToString(s String8) string {
	var i int
	for i = 0; i < len(s); i++ {
		if s[i] == 0 {
			break
		}
	}
	return string(s[:i])
}

// ReadWAD reads WAD metadata to memory. It returns a WAD object that
// can be used to read individual lumps.
func ReadWAD(filename string) (*WAD, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	wad := &WAD{
		file: file,
	}
	header, err := wad.readHeader()
	if err != nil {
		return nil, err
	}
	if string(header.Magic[:]) != "IWAD" {
		return nil, fmt.Errorf("bad magic: %s\n", header.Magic)
	}
	wad.header = header
	if err := wad.readInfoTables(); err != nil {
		return nil, err
	}
	playpal, err := wad.readPlaypal()
	if err != nil {
		return nil, err
	}
	wad.Playpal = playpal
	wad.TransparentPaletteIndex = 255
	pnames, err := wad.readPatchNames()
	if err != nil {
		return nil, err
	}
	wad.pnames = pnames
	patches, err := wad.readPatchLumps()
	if err != nil {
		return nil, err
	}
	wad.patches = patches
	textures, err := wad.readTextureLumps()
	if err != nil {
		return nil, err
	}
	wad.textures = textures
	flats, err := wad.readFlatLumps()
	if err != nil {
		return nil, err
	}
	wad.flats = flats
	return wad, nil
}

func (w *WAD) readHeader() (*header, error) {
	var header header
	if err := binary.Read(w.file, binary.LittleEndian, &header); err != nil {
		return nil, err
	}
	return &header, nil
}

func (w *WAD) readInfoTables() error {
	if err := w.seek(int64(w.header.InfoTableOfs)); err != nil {
		return err
	}
	lumps := map[string]int{}
	levels := map[string]int{}
	lumpInfos := make([]lumpInfo, w.header.NumLumps, w.header.NumLumps)
	for i := int32(0); i < w.header.NumLumps; i++ {
		var lumpInfo lumpInfo
		if err := binary.Read(w.file, binary.LittleEndian, &lumpInfo); err != nil {
			return err
		}
		if ToString(lumpInfo.Name) == "THINGS" {
			levelIdx := int(i - 1)
			levelLump := lumpInfos[levelIdx]
			levels[ToString(levelLump.Name)] = levelIdx
		}
		lumps[ToString(lumpInfo.Name)] = int(i)
		lumpInfos[i] = lumpInfo
	}
	w.levels = levels
	w.lumps = lumps
	w.lumpInfos = lumpInfos
	return nil
}

func (w *WAD) readPlaypal() (*Playpal, error) {
	playpalLump := w.lumps["PLAYPAL"]
	lumpInfo := w.lumpInfos[playpalLump]
	if err := w.seek(int64(lumpInfo.Filepos)); err != nil {
		return nil, err
	}
	fmt.Printf("Loading palette ...\n")
	playpal := Playpal{}
	if err := binary.Read(w.file, binary.LittleEndian, &playpal); err != nil {
		return nil, err
	}
	return &playpal, nil
}

func (w *WAD) readPatchNames() ([]String8, error) {
	pnamesLump := w.lumps["PNAMES"]
	lumpInfo := w.lumpInfos[pnamesLump]
	if err := w.seek(int64(lumpInfo.Filepos)); err != nil {
		return nil, err
	}
	var count uint32
	if err := binary.Read(w.file, binary.LittleEndian, &count); err != nil {
		return nil, err
	}
	fmt.Printf("Loading %d patches ...\n", count)
	pnames := make([]String8, count, count)
	if err := binary.Read(w.file, binary.LittleEndian, pnames); err != nil {
		return nil, err
	}
	return pnames, nil
}

func (w *WAD) readPatchLumps() (map[string]Image, error) {
	patches := make(map[string]Image)
	for _, pname := range w.pnames {
		lumpInfo := w.lumpInfos[w.lumps[ToString(pname)]]
		if err := w.seek(int64(lumpInfo.Filepos)); err != nil {
			return nil, err
		}
		lump := make([]byte, lumpInfo.Size, lumpInfo.Size)
		n, err := w.file.Read(lump)
		if err != nil {
			return nil, err
		}
		if n != int(lumpInfo.Size) {
			return nil, fmt.Errorf("Truncated lump")
		}
		reader := bytes.NewBuffer(lump[0:])
		var header PictureHeader
		if err := binary.Read(reader, binary.LittleEndian, &header); err != nil {
			return nil, err
		}
		if header.Width > 4096 || header.Height > 4096 {
			continue
		}
		offsets := make([]int32, header.Width, header.Width)
		if err := binary.Read(reader, binary.LittleEndian, offsets); err != nil {
			return nil, err
		}
		size := int(header.Width) * int(header.Height)
		pixels := make([]byte, size, size)
		for y := 0; y < int(header.Height); y++ {
			for x := 0; x < int(header.Width); x++ {
				pixels[y*int(header.Width)+x] = w.TransparentPaletteIndex
			}
		}
		for columnIndex, offset := range offsets {
			for {
				rowStart := lump[offset]
				offset += 1
				if rowStart == 255 {
					break
				}
				numPixels := lump[offset]
				offset += 1
				offset += 1 /* Padding */
				for i := 0; i < int(numPixels); i++ {
					pixelOffset := (int(rowStart)+i)*int(header.Width) + columnIndex
					pixels[pixelOffset] = lump[offset]
					offset += 1
				}
				offset += 1 /* Padding */
			}
		}
		patches[ToString(pname)] = Image{Width: int(header.Width), Height: int(header.Height), Pixels: pixels}
	}
	return patches, nil
}

func (w *WAD) readTextureLumps() (map[string]Texture, error) {
	textureLumps := make([]int, 0, 2)
	if lump, ok := w.lumps["TEXTURE1"]; ok {
		textureLumps = append(textureLumps, lump)
	}
	if lump, ok := w.lumps["TEXTURE2"]; ok {
		textureLumps = append(textureLumps, lump)
	}
	textures := make(map[string]Texture)
	for _, i := range textureLumps {
		lumpInfo := w.lumpInfos[i]
		if err := w.seek(int64(lumpInfo.Filepos)); err != nil {
			return nil, err
		}
		var count uint32
		if err := binary.Read(w.file, binary.LittleEndian, &count); err != nil {
			return nil, err
		}
		fmt.Printf("Loading %d textures ...\n", count)
		offsets := make([]int32, count, count)
		if err := binary.Read(w.file, binary.LittleEndian, offsets); err != nil {
			return nil, err
		}
		for _, offset := range offsets {
			if err := w.seek(int64(lumpInfo.Filepos + offset)); err != nil {
				return nil, err
			}
			var header TextureHeader
			if err := binary.Read(w.file, binary.LittleEndian, &header); err != nil {
				return nil, err
			}
			name := ToString(header.TexName)
			patches := make([]Patch, header.NumPatches, header.NumPatches)
			if err := binary.Read(w.file, binary.LittleEndian, patches); err != nil {
				return nil, err
			}
			texture := Texture{Header: &header, Patches: patches}
			textures[name] = texture
		}
	}
	return textures, nil
}

func (w *WAD) readFlatLumps() (map[string]Flat, error) {
	flats := make(map[string]Flat)
	startLump, ok := w.lumps["F_START"]
	if !ok {
		return nil, fmt.Errorf("F_START not found")
	}
	endLump, ok := w.lumps["F_END"]
	if !ok {
		return nil, fmt.Errorf("F_END not found")
	}
	for i := startLump; i < endLump; i++ {
		lumpInfo := w.lumpInfos[i]
		fmt.Printf("Flat: %s\n", ToString(lumpInfo.Name))
		if err := w.seek(int64(lumpInfo.Filepos)); err != nil {
			return nil, err
		}
		size := 4096
		data := make([]byte, size, size)
		if err := binary.Read(w.file, binary.LittleEndian, data); err != nil {
			return nil, err
		}
		flats[ToString(lumpInfo.Name)] = Flat{Data: data}
	}
	return flats, nil
}

func (w *WAD) seek(offset int64) error {
	off, err := w.file.Seek(offset, os.SEEK_SET)
	if err != nil {
		return err
	}
	if off != offset {
		return fmt.Errorf("seek failed")
	}
	return nil
}

func (w *WAD) LoadTexture(texname string) (*Texture, error) {
	texture := w.textures[texname]
	return &texture, nil
}

func (w *WAD) LoadImage(pnameNumber int16) (*Image, error) {
	image := w.patches[ToString(w.pnames[pnameNumber])]
	return &image, nil
}

func (w *WAD) LoadFlat(flatname string) (*Flat, error) {
	flat := w.flats[flatname]
	return &flat, nil
}

// LevelNames returns an array of level names found in the WAD archive.
func (w *WAD) LevelNames() []string {
	result := []string{}
	for name := range w.levels {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

// ReadLevel reads level data from WAD archive and returns a Level struct.
func (w *WAD) ReadLevel(name string) (*Level, error) {
	level := Level{}
	levelIdx := w.levels[name]
	for i := levelIdx + 1; i < levelIdx+11; i++ {
		lumpInfo := w.lumpInfos[i]
		if err := w.seek(int64(lumpInfo.Filepos)); err != nil {
			return nil, err
		}
		name := ToString(lumpInfo.Name)
		switch name {
		case "THINGS":
			things, err := w.readThings(&lumpInfo)
			if err != nil {
				return nil, err
			}
			level.Things = things
		case "SIDEDEFS":
			sidedefs, err := w.readSidedefs(&lumpInfo)
			if err != nil {
				return nil, err
			}
			level.Sidedefs = sidedefs
		case "LINEDEFS":
			linedefs, err := w.readLinedefs(&lumpInfo)
			if err != nil {
				return nil, err
			}
			level.Linedefs = linedefs
		case "VERTEXES":
			vertexes, err := w.readVertexes(&lumpInfo)
			if err != nil {
				return nil, err
			}
			level.Vertexes = vertexes
		case "SEGS":
			segs, err := w.readSegs(&lumpInfo)
			if err != nil {
				return nil, err
			}
			level.Segs = segs
		case "SSECTORS":
			ssectors, err := w.readSSectors(&lumpInfo)
			if err != nil {
				return nil, err
			}
			level.SSectors = ssectors
		case "NODES":
			nodes, err := w.readNodes(&lumpInfo)
			if err != nil {
				return nil, err
			}
			level.Nodes = nodes
		case "SECTORS":
			sectors, err := w.readSectors(&lumpInfo)
			if err != nil {
				return nil, err
			}
			level.Sectors = sectors
		default:
			fmt.Printf("Unhandled lump %s\n", name)
		}
	}
	return &level, nil
}

func (w *WAD) readThings(lumpInfo *lumpInfo) ([]Thing, error) {
	var thing Thing
	count := int(lumpInfo.Size) / int(unsafe.Sizeof(thing))
	things := make([]Thing, count, count)
	if err := binary.Read(w.file, binary.LittleEndian, things); err != nil {
		return nil, err
	}
	return things, nil
}

func (w *WAD) readLinedefs(lumpInfo *lumpInfo) ([]Linedef, error) {
	var linedef Linedef
	count := int(lumpInfo.Size) / int(unsafe.Sizeof(linedef))
	linedefs := make([]Linedef, count, count)
	if err := binary.Read(w.file, binary.LittleEndian, linedefs); err != nil {
		return nil, err
	}
	return linedefs, nil
}

func (w *WAD) readSidedefs(lumpInfo *lumpInfo) ([]Sidedef, error) {
	var sidedef Sidedef
	count := int(lumpInfo.Size) / int(unsafe.Sizeof(sidedef))
	sidedefs := make([]Sidedef, count, count)
	if err := binary.Read(w.file, binary.LittleEndian, sidedefs); err != nil {
		return nil, err
	}
	return sidedefs, nil
}

func (w *WAD) readVertexes(lumpInfo *lumpInfo) ([]Vertex, error) {
	var vertex Vertex
	count := int(lumpInfo.Size) / int(unsafe.Sizeof(vertex))
	vertexes := make([]Vertex, count, count)
	if err := binary.Read(w.file, binary.LittleEndian, vertexes); err != nil {
		return nil, err
	}
	return vertexes, nil
}

func (w *WAD) readSegs(lumpInfo *lumpInfo) ([]Seg, error) {
	var seg Seg
	count := int(lumpInfo.Size) / int(unsafe.Sizeof(seg))
	segs := make([]Seg, count, count)
	if err := binary.Read(w.file, binary.LittleEndian, segs); err != nil {
		return nil, err
	}
	return segs, nil
}

func (w *WAD) readSSectors(lumpInfo *lumpInfo) ([]SSector, error) {
	var ssector SSector
	count := int(lumpInfo.Size) / int(unsafe.Sizeof(ssector))
	ssectors := make([]SSector, count, count)
	if err := binary.Read(w.file, binary.LittleEndian, ssectors); err != nil {
		return nil, err
	}
	return ssectors, nil
}

func (w *WAD) readNodes(lumpInfo *lumpInfo) ([]Node, error) {
	var node Node
	count := int(lumpInfo.Size) / int(unsafe.Sizeof(node))
	nodes := make([]Node, count, count)
	if err := binary.Read(w.file, binary.LittleEndian, nodes); err != nil {
		return nil, err
	}
	return nodes, nil
}

func (w *WAD) readSectors(lumpInfo *lumpInfo) ([]Sector, error) {
	var sector Sector
	count := int(lumpInfo.Size) / int(unsafe.Sizeof(sector))
	sectors := make([]Sector, count, count)
	if err := binary.Read(w.file, binary.LittleEndian, sectors); err != nil {
		return nil, err
	}
	return sectors, nil
}
