package main

import (
	"fmt"
	"github.com/codegangsta/cli"
	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/glfw/v3.1/glfw"
	"github.com/go-gl/mathgl/mgl32"
	"image"
	"image/color"
	"log"
	"math"
	"os"
	"runtime"
	"strings"
)

const (
	vertex = `#version 330

in vec3 vertex;
in vec2 vertTexCoord;

uniform mat4 MVP;

out vec2 fragTexCoord;

void main()
{
    fragTexCoord = vertTexCoord;
    gl_Position = MVP * vec4(vertex, 1.0);
}` + "\x00"

	fragment = `#version 330

uniform float LightLevel;
uniform sampler2D tex;

in vec2 fragTexCoord;

out vec4 outColor;

void main()
{
    float alpha = texture(tex, fragTexCoord).a;
    if (alpha == 1.0) {
        outColor = texture(tex, fragTexCoord) * LightLevel;
    } else {
        discard;
    }
}` + "\x00"
)

const (
	subsectorBit = int(0x8000)
)

type Point3 struct {
	X int16
	Y int16
	Z int16
	U float32
	V float32
}

type Point struct {
	X int16
	Y int16
}

type Mesh struct {
	texture    string
	vao        uint32
	vbo        uint32
	count      int
	lightLevel float32
}

type Scene struct {
	meshes   map[int][]Mesh // Meshes indexed by subsector ID.
	textures map[string]uint32
}

func NewScene() Scene {
	return Scene{
		meshes:   make(map[int][]Mesh),
		textures: make(map[string]uint32),
	}
}

func (scene *Scene) CacheTexture(wad *WAD, name string) error {
	_, loaded := scene.textures[name]
	if loaded {
		return nil
	}
	texture, err := loadTexture(wad, name)
	if err != nil {
		return err
	}
	scene.textures[name] = texture
	return nil
}

func NewMesh(texture string, lightLevel int16, vertices []Point3) Mesh {
	var vao uint32
	gl.GenVertexArrays(1, &vao)
	gl.BindVertexArray(vao)

	var vbo uint32
	gl.GenBuffers(1, &vbo)
	gl.BindBuffer(gl.ARRAY_BUFFER, vbo)

	vbo_data := []float32{}
	for _, vertex := range vertices {
		vbo_data = append(vbo_data, float32(vertex.X), float32(vertex.Y), float32(vertex.Z), vertex.U, vertex.V)
	}
	gl.BufferData(gl.ARRAY_BUFFER, len(vbo_data)*4, gl.Ptr(vbo_data), gl.STATIC_DRAW)

	vertexAttrib := uint32(0)
	gl.VertexAttribPointer(vertexAttrib, 3, gl.FLOAT, false, 5*4, gl.PtrOffset(0))
	gl.EnableVertexAttribArray(vertexAttrib)

	texCoordAttrib := uint32(1)
	gl.VertexAttribPointer(texCoordAttrib, 2, gl.FLOAT, false, 5*4, gl.PtrOffset(3*4))
	gl.EnableVertexAttribArray(texCoordAttrib)

	return Mesh{vao: vao, vbo: vbo, texture: texture, count: len(vbo_data), lightLevel: float32(lightLevel) / 255.0}
}

func genSubsector(wad *WAD, level *Level, ssectorId int, scene *Scene) {
	ssector := level.SSectors[ssectorId]
	for seg := ssector.StartSeg; seg < ssector.StartSeg+ssector.Numsegs; seg++ {
		genSeg(wad, level, ssectorId, int(seg), scene)
	}
}

func genSeg(wad *WAD, level *Level, ssectorId int, segId int, scene *Scene) {
	seg := level.Segs[segId]
	genLinedef(wad, level, &seg, ssectorId, int(seg.LineNum), scene)
}

func segSidedef(level *Level, seg *Seg, linedef *Linedef) *Sidedef {
	if seg.Segside == 0 {
		return &level.Sidedefs[linedef.SidedefRight]
	} else {
		if linedef.SidedefLeft == -1 {
			return nil
		}
		return &level.Sidedefs[linedef.SidedefLeft]
	}
}

func segOppositeSidedef(level *Level, seg *Seg, linedef *Linedef) *Sidedef {
	if seg.Segside == 0 {
		if linedef.SidedefLeft == -1 {
			return nil
		}
		return &level.Sidedefs[linedef.SidedefLeft]
	} else {
		return &level.Sidedefs[linedef.SidedefRight]
	}
}

func genLinedef(wad *WAD, level *Level, seg *Seg, ssectorId int, linedefId int, scene *Scene) {
	meshes := scene.meshes[ssectorId]

	linedef := level.Linedefs[linedefId]

	sidedef := segSidedef(level, seg, &linedef)
	if sidedef == nil {
		return
	}
	sector := level.Sectors[sidedef.SectorRef]

	oppositeSidedef := segOppositeSidedef(level, seg, &linedef)

	start := level.Vertexes[linedef.VertexStart]
	end := level.Vertexes[linedef.VertexEnd]

	upperTexture := ToString(sidedef.UpperTexture)
	middleTexture := ToString(sidedef.MiddleTexture)
	lowerTexture := ToString(sidedef.LowerTexture)

	if upperTexture != "-" && oppositeSidedef != nil {
		oppositeSector := level.Sectors[oppositeSidedef.SectorRef]

		vertices := []Point3{}

		vertices = append(vertices, Point3{X: -start.XCoord, Y: sector.CeilingHeight, Z: start.YCoord, U: 0.0, V: 1.0})
		vertices = append(vertices, Point3{X: -start.XCoord, Y: oppositeSector.CeilingHeight, Z: start.YCoord, U: 0.0, V: 0.0})
		vertices = append(vertices, Point3{X: -end.XCoord, Y: oppositeSector.CeilingHeight, Z: end.YCoord, U: 1.0, V: 0.0})

		vertices = append(vertices, Point3{X: -end.XCoord, Y: oppositeSector.CeilingHeight, Z: end.YCoord, U: 1.0, V: 0.0})
		vertices = append(vertices, Point3{X: -end.XCoord, Y: sector.CeilingHeight, Z: end.YCoord, U: 1.0, V: 1.0})
		vertices = append(vertices, Point3{X: -start.XCoord, Y: sector.CeilingHeight, Z: start.YCoord, U: 0.0, V: 1.0})

		meshes = append(meshes, NewMesh(upperTexture, sector.Lightlevel, vertices))

		scene.CacheTexture(wad, upperTexture)
	}

	if middleTexture != "-" {
		vertices := []Point3{}

		vertices = append(vertices, Point3{X: -start.XCoord, Y: sector.FloorHeight, Z: start.YCoord, U: 0.0, V: 1.0})
		vertices = append(vertices, Point3{X: -start.XCoord, Y: sector.CeilingHeight, Z: start.YCoord, U: 0.0, V: 0.0})
		vertices = append(vertices, Point3{X: -end.XCoord, Y: sector.CeilingHeight, Z: end.YCoord, U: 1.0, V: 0.0})

		vertices = append(vertices, Point3{X: -end.XCoord, Y: sector.CeilingHeight, Z: end.YCoord, U: 1.0, V: 0.0})
		vertices = append(vertices, Point3{X: -end.XCoord, Y: sector.FloorHeight, Z: end.YCoord, U: 1.0, V: 1.0})
		vertices = append(vertices, Point3{X: -start.XCoord, Y: sector.FloorHeight, Z: start.YCoord, U: 0.0, V: 1.0})

		meshes = append(meshes, NewMesh(middleTexture, sector.Lightlevel, vertices))

		scene.CacheTexture(wad, middleTexture)
	}

	if lowerTexture != "-" && oppositeSidedef != nil {
		oppositeSector := level.Sectors[oppositeSidedef.SectorRef]

		vertices := []Point3{}

		vertices = append(vertices, Point3{X: -start.XCoord, Y: sector.FloorHeight, Z: start.YCoord, U: 0.0, V: 1.0})
		vertices = append(vertices, Point3{X: -start.XCoord, Y: oppositeSector.FloorHeight, Z: start.YCoord, U: 0.0, V: 0.0})
		vertices = append(vertices, Point3{X: -end.XCoord, Y: oppositeSector.FloorHeight, Z: end.YCoord, U: 1.0, V: 0.0})

		vertices = append(vertices, Point3{X: -end.XCoord, Y: oppositeSector.FloorHeight, Z: end.YCoord, U: 1.0, V: 0.0})
		vertices = append(vertices, Point3{X: -end.XCoord, Y: sector.FloorHeight, Z: end.YCoord, U: 1.0, V: 1.0})
		vertices = append(vertices, Point3{X: -start.XCoord, Y: sector.FloorHeight, Z: start.YCoord, U: 0.0, V: 1.0})

		meshes = append(meshes, NewMesh(lowerTexture, sector.Lightlevel, vertices))

		scene.CacheTexture(wad, lowerTexture)
	}

	scene.meshes[ssectorId] = meshes
}

type bspAction func(level *Level, idx int)

func traverseBsp(level *Level, point *Point, idx int, action bspAction) {
	if idx&subsectorBit == subsectorBit {
		if idx == -1 {
			action(level, 0)
			return
		} else {
			action(level, int(uint16(idx) & ^uint16(subsectorBit)))
			return
		}
	}
	node := level.Nodes[idx]
	traverseBsp(level, point, int(node.Child[0]), action)
	traverseBsp(level, point, int(node.Child[1]), action)
}

func intersects(point *Point, bbox *BBox) bool {
	return point.X > bbox.Left && point.X < bbox.Right && point.Y > bbox.Bottom && point.Y < bbox.Top
}

func findSector(level *Level, point *Point, idx int) *Sector {
	if idx&subsectorBit == subsectorBit {
		idx = int(uint16(idx) & ^uint16(subsectorBit))
		ssector := level.SSectors[idx]
		for segIdx := ssector.StartSeg; segIdx < ssector.StartSeg+ssector.Numsegs; segIdx++ {
			seg := level.Segs[segIdx]
			linedef := level.Linedefs[seg.LineNum]
			sidedef := segSidedef(level, &seg, &linedef)
			if sidedef != nil {
				return &level.Sectors[sidedef.SectorRef]
			}
			oppositeSidedef := segOppositeSidedef(level, &seg, &linedef)
			if oppositeSidedef != nil {
				return &level.Sectors[oppositeSidedef.SectorRef]
			}
		}
	}
	node := level.Nodes[idx]
	if intersects(point, &node.BBox[0]) {
		return findSector(level, point, int(node.Child[0]))
	}
	if intersects(point, &node.BBox[1]) {
		return findSector(level, point, int(node.Child[1]))
	}
	return nil
}

func main() {
	runtime.LockOSThread()
	app := cli.NewApp()
	app.Name = "godoom"
	app.Usage = "A Doom clone written in Go!"
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "file,f",
			Usage: "WAD archive",
			Value: "doom1.wad",
		},
		cli.IntFlag{
			Name:  "level,l",
			Usage: "Level number",
			Value: 1,
		},
	}
	app.Action = func(c *cli.Context) {
		file := c.String("file")
		levelNumber := c.Int("level")
		levelIdx := levelNumber - 1
		fmt.Printf("Loading WAD archive '%s' ...\n", file)
		wad, err := ReadWAD(file)
		if err != nil {
			fmt.Printf("error: %s\n", err)
			os.Exit(1)
		}
		levelNames := wad.LevelNames()
		if len(levelNames) == 0 {
			fmt.Printf("error: No levels found!\n")
			os.Exit(1)
		}
		if levelIdx >= len(levelNames) {
			fmt.Printf("error: No such level number %d!\n", levelNumber)
			os.Exit(1)
		}
		fmt.Printf("Levels:\n")
		for i, level := range wad.LevelNames() {
			selected := ""
			if i == levelIdx {
				selected = " [*]"
			}
			fmt.Printf("  %s%s\n", level, selected)
		}
		levelName := levelNames[levelIdx]
		fmt.Printf("Loading level %s ...\n", levelName)
		level, err := wad.ReadLevel(levelName)
		if err != nil {
			fmt.Printf("error: %s\n", err)
			os.Exit(1)
		}
		player1 := level.Things[1]
		position := &Point{
			X: player1.XPosition,
			Y: player1.YPosition,
		}
		game(wad, level, position, player1.Angle)
	}
	app.Run(os.Args)
}

func game(wad *WAD, level *Level, startPos *Point, startAngle int16) {
	runtime.LockOSThread()

	if err := glfw.Init(); err != nil {
		log.Fatalln("failed to initialize glfw:", err)
	}
	defer glfw.Terminate()

	glfw.WindowHint(glfw.Resizable, glfw.True)
	glfw.WindowHint(glfw.ContextVersionMajor, 3)
	glfw.WindowHint(glfw.ContextVersionMinor, 3)
	glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)
	glfw.WindowHint(glfw.OpenGLForwardCompatible, glfw.True)

	window, err := glfw.CreateWindow(640, 480, "GoDoom", nil, nil)
	if err != nil {
		panic(err)
	}

	defer window.Destroy()

	window.SetInputMode(glfw.CursorMode, glfw.CursorDisabled)

	window.MakeContextCurrent()
	glfw.SwapInterval(1)

	gl.Init()

	speed := float32(5.0)

	position := mgl32.Vec2{float32(startPos.X), float32(startPos.Y)}

	angle := startAngle

	fmt.Printf("Generating scene ...\n")
	scene := NewScene()
	var action bspAction = func(level *Level, idx int) {
		genSubsector(wad, level, idx, &scene)
	}
	traverseBsp(level, &Point{int16(position.X()), int16(position.Y())}, len(level.Nodes)-1, action)

	vertex_shader, err := compileShader(vertex, gl.VERTEX_SHADER)
	if err != nil {
		panic(err)
	}

	fragment_shader, err := compileShader(fragment, gl.FRAGMENT_SHADER)
	if err != nil {
		panic(err)
	}

	program := gl.CreateProgram()
	gl.AttachShader(program, vertex_shader)
	gl.AttachShader(program, fragment_shader)

	gl.DeleteShader(vertex_shader)
	gl.DeleteShader(fragment_shader)

	gl.BindFragDataLocation(program, 0, gl.Str("outColor\x00"))
	gl.LinkProgram(program)

	lightLevelID := gl.GetUniformLocation(program, gl.Str("LightLevel\x00"))
	matrixID := gl.GetUniformLocation(program, gl.Str("MVP\x00"))

	gl.Enable(gl.DEPTH_TEST)
	gl.DepthFunc(gl.LESS)
	gl.ClearColor(0.3, 0.3, 0.3, 1.0)

	floorHeight := int16(0)

	for !window.ShouldClose() {
		gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)

		gl.UseProgram(program)

		sector := findSector(level, &Point{int16(position.X()), int16(position.Y())}, len(level.Nodes)-1)
		if sector != nil {
			floorHeight = sector.FloorHeight + 30
		}

		eye := mgl32.Vec3{-position.X(), float32(floorHeight), position.Y()}

		y, x := math.Sincos(float64(angle) * math.Pi / 180)

		direction := mgl32.Vec3{float32(x), 0.0, float32(y)}

		width, height := window.GetFramebufferSize()
		gl.Viewport(0, 0, int32(width), int32(height))
		projection := mgl32.Perspective(64.0, float32(width)/float32(height), 1.0, 10000.0)
		view := mgl32.LookAt(eye.X(), eye.Y(), eye.Z(), eye.X()+direction.X(), eye.Y()+direction.Y(), eye.Z()+direction.Z(), 0.0, 1.0, 0.0)
		model := mgl32.Ident4()
		mvp := projection.Mul4(view).Mul4(model)

		gl.UniformMatrix4fv(matrixID, 1, false, &mvp[0])

		gl.ActiveTexture(gl.TEXTURE0)

		var render bspAction = func(level *Level, idx int) {
			for _, mesh := range scene.meshes[idx] {
				gl.Uniform1f(lightLevelID, mesh.lightLevel)
				gl.BindTexture(gl.TEXTURE_2D, scene.textures[mesh.texture])
				gl.BindVertexArray(mesh.vao)
				gl.DrawArrays(gl.TRIANGLES, 0, int32(mesh.count))
			}
		}
		traverseBsp(level, &Point{int16(position.X()), int16(position.Y())}, len(level.Nodes)-1, render)

		window.SwapBuffers()
		glfw.PollEvents()

		if window.GetKey(glfw.KeyEscape) == glfw.Press {
			window.SetShouldClose(true)
		}
		if window.GetKey(glfw.KeyUp) == glfw.Press {
			position = position.Add(mgl32.Vec2{-direction.X(), direction.Z()}.Mul(speed))
		}
		if window.GetKey(glfw.KeyDown) == glfw.Press {
			position = position.Sub(mgl32.Vec2{-direction.X(), direction.Z()}.Mul(speed))
		}
		if window.GetKey(glfw.KeyLeft) == glfw.Press {
			angle -= int16(speed)
		}
		if window.GetKey(glfw.KeyRight) == glfw.Press {
			angle += int16(speed)
		}
	}
}

func compileShader(source string, shaderType uint32) (uint32, error) {
	shader := gl.CreateShader(shaderType)

	csource := gl.Str(source)
	gl.ShaderSource(shader, 1, &csource, nil)
	gl.CompileShader(shader)

	var status int32
	gl.GetShaderiv(shader, gl.COMPILE_STATUS, &status)
	if status == gl.FALSE {
		var logLength int32
		gl.GetShaderiv(shader, gl.INFO_LOG_LENGTH, &logLength)

		log := strings.Repeat("\x00", int(logLength+1))
		gl.GetShaderInfoLog(shader, logLength, nil, gl.Str(log))

		return 0, fmt.Errorf("failed to compile %v: %v", source, log)
	}

	return shader, nil
}

func loadTexture(wad *WAD, texname string) (uint32, error) {
	texture, err := wad.LoadTexture(texname)
	if err != nil {
		return 0, err
	}
	if texture.Header == nil {
		// FIXME: Why do we have a texture with no header?
		return 0, nil
	}
	bounds := image.Rect(0, 0, int(texture.Header.Width), int(texture.Header.Height))
	rgba := image.NewRGBA(bounds)
	if rgba.Stride != rgba.Rect.Size().X*4 {
		return 0, fmt.Errorf("unsupported stride")
	}
	for _, patch := range texture.Patches {
		image, err := wad.LoadImage(patch.PNameNumber)
		if err != nil {
			return 0, err
		}
		for y := 0; y < image.Height; y++ {
			for x := 0; x < image.Width; x++ {
				pixel := image.Pixels[y*image.Width+x]
				var alpha uint8
				if pixel == wad.TransparentPaletteIndex {
					alpha = 0
				} else {
					alpha = 255
				}
				rgb := wad.Playpal.Palettes[0].Table[pixel]
				rgba.Set(int(patch.XOffset)+x, int(patch.YOffset)+y, color.RGBA{rgb.Red, rgb.Green, rgb.Blue, alpha})
			}
		}
	}

	var texId uint32
	gl.GenTextures(1, &texId)
	gl.ActiveTexture(gl.TEXTURE0)
	gl.BindTexture(gl.TEXTURE_2D, texId)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	gl.TexImage2D(
		gl.TEXTURE_2D,
		0,
		gl.RGBA,
		int32(rgba.Rect.Size().X),
		int32(rgba.Rect.Size().Y),
		0,
		gl.RGBA,
		gl.UNSIGNED_BYTE,
		gl.Ptr(rgba.Pix))
	return texId, nil
}
