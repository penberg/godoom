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

type VertexArray struct {
	texture    string
	vao        uint32
	vbo        uint32
	count      int
	lightLevel float32
}

func NewVertexArray(texture string, lightLevel int16, vertices []Point3) VertexArray {
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

	return VertexArray{vao: vao, vbo: vbo, texture: texture, count: len(vbo_data), lightLevel: float32(lightLevel) / 255.0}
}

func renderSubsector(level *Level, idx int, vertices []VertexArray) []VertexArray {
	ssector := level.SSectors[idx]
	for seg := ssector.StartSeg; seg < ssector.StartSeg+ssector.Numsegs; seg++ {
		vertices = append(vertices, renderSeg(level, int(seg), vertices)...)
	}
	return vertices
}

func renderSeg(level *Level, idx int, vertices []VertexArray) []VertexArray {
	seg := level.Segs[idx]
	return renderLinedef(level, &seg, int(seg.LineNum), vertices)
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

func renderLinedef(level *Level, seg *Seg, idx int, vertexArrays []VertexArray) []VertexArray {
	linedef := level.Linedefs[idx]

	sidedef := segSidedef(level, seg, &linedef)
	if sidedef == nil {
		return vertexArrays
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

		vertexArrays = append(vertexArrays, NewVertexArray(upperTexture, sector.Lightlevel, vertices))
	}

	if middleTexture != "-" {
		vertices := []Point3{}

		vertices = append(vertices, Point3{X: -start.XCoord, Y: sector.FloorHeight, Z: start.YCoord, U: 0.0, V: 1.0})
		vertices = append(vertices, Point3{X: -start.XCoord, Y: sector.CeilingHeight, Z: start.YCoord, U: 0.0, V: 0.0})
		vertices = append(vertices, Point3{X: -end.XCoord, Y: sector.CeilingHeight, Z: end.YCoord, U: 1.0, V: 0.0})

		vertices = append(vertices, Point3{X: -end.XCoord, Y: sector.CeilingHeight, Z: end.YCoord, U: 1.0, V: 0.0})
		vertices = append(vertices, Point3{X: -end.XCoord, Y: sector.FloorHeight, Z: end.YCoord, U: 1.0, V: 1.0})
		vertices = append(vertices, Point3{X: -start.XCoord, Y: sector.FloorHeight, Z: start.YCoord, U: 0.0, V: 1.0})

		vertexArrays = append(vertexArrays, NewVertexArray(middleTexture, sector.Lightlevel, vertices))
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

		vertexArrays = append(vertexArrays, NewVertexArray(lowerTexture, sector.Lightlevel, vertices))
	}

	return vertexArrays
}

func pointOnSide(point *Point, node *Node) int {
	dx := point.X - node.X
	dy := point.Y - node.Y
	// Perp dot product:
	if dy*node.DX>>16-node.DY>>16*dx < 0 {
		// Point is on front side:
		return 0
	}
	// Point is on the back side:
	return 1
}

func traverseBsp(level *Level, point *Point, idx int, visibility bool, vertices []VertexArray) []VertexArray {
	if idx&subsectorBit == subsectorBit {
		if idx == -1 {
			return renderSubsector(level, 0, vertices)
		} else {
			return renderSubsector(level, int(uint16(idx) & ^uint16(subsectorBit)), vertices)
		}
	}
	node := level.Nodes[idx]

	if visibility {
		// TODO: Traverse back space if inside node's bounding box.
		side := pointOnSide(point, &node)
		return traverseBsp(level, point, int(node.Child[side]), visibility, vertices)
	} else {
		left := traverseBsp(level, point, int(node.Child[0]), visibility, []VertexArray{})
		right := traverseBsp(level, point, int(node.Child[1]), visibility, []VertexArray{})
		vertices = append(vertices, left...)
		vertices = append(vertices, right...)
		return vertices
	}
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
		game(wad, level, position)
	}
	app.Run(os.Args)
}

func game(wad *WAD, level *Level, startPos *Point) {
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

	direction := mgl32.Vec3{0.0, 0.0, 1.0}

	vertexArrays := traverseBsp(level, &Point{int16(position.X()), int16(position.Y())}, len(level.Nodes)-1, false, []VertexArray{})

	textures := map[string]uint32{}

	for _, vertexArray := range vertexArrays {
		_, loaded := textures[vertexArray.texture]
		if loaded {
			continue
		}
		texture, err := loadTexture(wad, vertexArray.texture)
		if err != nil {
			panic(err)
		}
		textures[vertexArray.texture] = texture
	}

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
			floorHeight = sector.FloorHeight + 10
		}

		eye := mgl32.Vec3{-position.X(), float32(floorHeight), position.Y()}

		width, height := window.GetFramebufferSize()
		gl.Viewport(0, 0, int32(width), int32(height))
		projection := mgl32.Perspective(64.0, float32(width)/float32(height), 1.0, 10000.0)
		view := mgl32.LookAt(eye.X(), eye.Y(), eye.Z(), eye.X()+direction.X(), eye.Y()+direction.Y(), eye.Z()+direction.Z(), 0.0, 1.0, 0.0)
		model := mgl32.Ident4()
		mvp := projection.Mul4(view).Mul4(model)

		gl.UniformMatrix4fv(matrixID, 1, false, &mvp[0])

		gl.ActiveTexture(gl.TEXTURE0)

		for _, vertexArray := range vertexArrays {
			gl.Uniform1f(lightLevelID, vertexArray.lightLevel)
			gl.BindTexture(gl.TEXTURE_2D, textures[vertexArray.texture])
			gl.BindVertexArray(vertexArray.vao)
			gl.DrawArrays(gl.TRIANGLES, 0, int32(vertexArray.count))
		}

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
			direction = mgl32.QuatRotate(0.1, mgl32.Vec3{0.0, 1.0, 0.0}).Rotate(direction)
		}
		if window.GetKey(glfw.KeyRight) == glfw.Press {
			direction = mgl32.QuatRotate(-0.1, mgl32.Vec3{0.0, 1.0, 0.0}).Rotate(direction)
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
