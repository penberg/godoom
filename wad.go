// Package wad provides access to Doom's data archives also known as WAD files.
// The file format is documented in The Unofficial DOOM Specs:
// http://www.gamers.org/dhs/helpdocs/dmsp1666.html
package main

import (
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
	header       *header
	file         *os.File
	pnamesLump   int
	playpalLump  int
	pnames       []String8
	playpal      *Playpal
	textureLumps []int
	textures     map[string]Texture
	levels       map[string]int
	lumpInfos    []lumpInfo
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
	wad.playpal = playpal
	pnames, err := wad.readPatchNames()
	if err != nil {
		return nil, err
	}
	wad.pnames = pnames
	textures, err := wad.readTextureLumps()
	if err != nil {
		return nil, err
	}
	wad.textures = textures
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
	pnamesLump := -1
	playpalLump := -1
	textureLumps := make([]int, 0, 2)
	levels := map[string]int{}
	lumpInfos := make([]lumpInfo, w.header.NumLumps, w.header.NumLumps)
	for i := int32(0); i < w.header.NumLumps; i++ {
		var lumpInfo lumpInfo
		if err := binary.Read(w.file, binary.LittleEndian, &lumpInfo); err != nil {
			return err
		}
		if ToString(lumpInfo.Name) == "PNAMES" {
			pnamesLump = int(i)
		}
		if ToString(lumpInfo.Name) == "PLAYPAL" {
			playpalLump = int(i)
		}
		if ToString(lumpInfo.Name) == "TEXTURE1" || ToString(lumpInfo.Name) == "TEXTURE2" {
			textureLumps = append(textureLumps, int(i))
		}
		if ToString(lumpInfo.Name) == "THINGS" {
			levelIdx := int(i - 1)
			levelLump := lumpInfos[levelIdx]
			levels[ToString(levelLump.Name)] = levelIdx
		}
		lumpInfos[i] = lumpInfo
	}
	w.pnamesLump = pnamesLump
	w.playpalLump = playpalLump
	w.textureLumps = textureLumps
	w.levels = levels
	w.lumpInfos = lumpInfos
	return nil
}

func (w *WAD) readPlaypal() (*Playpal, error) {
	lumpInfo := w.lumpInfos[w.playpalLump]
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
	lumpInfo := w.lumpInfos[w.pnamesLump]
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

func (w *WAD) readTextureLumps() (map[string]Texture, error) {
	textures := make(map[string]Texture)
	for _, i := range w.textureLumps {
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
