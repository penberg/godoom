package main

import (
	"fmt"
	"github.com/codegangsta/cli"
	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/glfw/v3.1/glfw"
	"github.com/go-gl/mathgl/mgl32"
	"log"
	"os"
	"runtime"
	"strings"
)

const (
	vertex = `#version 330

in vec3 vertex;

uniform mat4 MVP;

void main()
{
    gl_Position = MVP * vec4(vertex, 1.0);
}` + "\x00"

	fragment = `#version 330

out vec4 outColor;

void main()
{
    outColor = vec4(1.0, 1.0, 1.0, 1.0);
}` + "\x00"
)

const (
	subsectorBit = int(0x8000)
)

type Point3 struct {
	X int16
	Y int16
	Z int16
}

type Point struct {
	X int16
	Y int16
}

func renderSubsector(level *Level, idx int, vertices []Point3) []Point3 {
	ssector := level.SSectors[idx]
	for seg := ssector.StartSeg; seg < ssector.StartSeg+ssector.Numsegs; seg++ {
		vertices = append(vertices, renderSeg(level, int(seg), vertices)...)
	}
	return vertices
}

func renderSeg(level *Level, idx int, vertices []Point3) []Point3 {
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

func renderLinedef(level *Level, seg *Seg, idx int, vertices []Point3) []Point3 {
	linedef := level.Linedefs[idx]

	sidedef := segSidedef(level, seg, &linedef)
	if sidedef == nil {
		return vertices
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

		vertices = append(vertices, Point3{X: -start.XCoord, Y: sector.CeilingHeight, Z: start.YCoord})
		vertices = append(vertices, Point3{X: -start.XCoord, Y: oppositeSector.CeilingHeight, Z: start.YCoord})
		vertices = append(vertices, Point3{X: -end.XCoord, Y: oppositeSector.CeilingHeight, Z: end.YCoord})

		vertices = append(vertices, Point3{X: -end.XCoord, Y: oppositeSector.CeilingHeight, Z: end.YCoord})
		vertices = append(vertices, Point3{X: -end.XCoord, Y: sector.CeilingHeight, Z: end.YCoord})
		vertices = append(vertices, Point3{X: -start.XCoord, Y: sector.CeilingHeight, Z: start.YCoord})
	}

	if middleTexture != "-" {
		vertices = append(vertices, Point3{X: -start.XCoord, Y: sector.FloorHeight, Z: start.YCoord})
		vertices = append(vertices, Point3{X: -start.XCoord, Y: sector.CeilingHeight, Z: start.YCoord})
		vertices = append(vertices, Point3{X: -end.XCoord, Y: sector.CeilingHeight, Z: end.YCoord})

		vertices = append(vertices, Point3{X: -end.XCoord, Y: sector.CeilingHeight, Z: end.YCoord})
		vertices = append(vertices, Point3{X: -end.XCoord, Y: sector.FloorHeight, Z: end.YCoord})
		vertices = append(vertices, Point3{X: -start.XCoord, Y: sector.FloorHeight, Z: start.YCoord})
	}

	if lowerTexture != "-" && oppositeSidedef != nil {
		oppositeSector := level.Sectors[oppositeSidedef.SectorRef]

		vertices = append(vertices, Point3{X: -start.XCoord, Y: sector.FloorHeight, Z: start.YCoord})
		vertices = append(vertices, Point3{X: -start.XCoord, Y: oppositeSector.FloorHeight, Z: start.YCoord})
		vertices = append(vertices, Point3{X: -end.XCoord, Y: oppositeSector.FloorHeight, Z: end.YCoord})

		vertices = append(vertices, Point3{X: -end.XCoord, Y: oppositeSector.FloorHeight, Z: end.YCoord})
		vertices = append(vertices, Point3{X: -end.XCoord, Y: sector.FloorHeight, Z: end.YCoord})
		vertices = append(vertices, Point3{X: -start.XCoord, Y: sector.FloorHeight, Z: start.YCoord})
	}

	return vertices
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

func traverseBsp(level *Level, point *Point, idx int, visibility bool, vertices []Point3) []Point3 {
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
		left := traverseBsp(level, point, int(node.Child[0]), visibility, []Point3{})
		right := traverseBsp(level, point, int(node.Child[1]), visibility, []Point3{})
		vertices = append(vertices, left...)
		vertices = append(vertices, right...)
		return vertices
	}
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
		game(level, position)
	}
	app.Run(os.Args)
}

func game(level *Level, position *Point) {
	runtime.LockOSThread()

	if err := glfw.Init(); err != nil {
		log.Fatalln("failed to initialize glfw:", err)
	}
	defer glfw.Terminate()

	glfw.WindowHint(glfw.Resizable, glfw.False)
	glfw.WindowHint(glfw.ContextVersionMajor, 3)
	glfw.WindowHint(glfw.ContextVersionMinor, 3)
	glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)
	glfw.WindowHint(glfw.OpenGLForwardCompatible, glfw.True)

	window, err := glfw.CreateWindow(640, 480, "GoDoom", nil, nil)
	if err != nil {
		panic(err)
	}

	defer window.Destroy()

	width, height := window.GetSize()

	window.MakeContextCurrent()
	glfw.SwapInterval(1)

	gl.Init()

	var vao uint32
	gl.GenVertexArrays(1, &vao)
	gl.BindVertexArray(vao)

	var vbo uint32
	gl.GenBuffers(1, &vbo)
	gl.BindBuffer(gl.ARRAY_BUFFER, vbo)

	speed := float32(5.0)

	eye := mgl32.Vec3{float32(-position.X), 0.0, float32(position.Y)}
	direction := mgl32.Vec3{0.0, 0.0, 1.0}

	vertices := traverseBsp(level, position, len(level.Nodes)-1, false, []Point3{})
	vbo_data := []float32{}
	for _, vertex := range vertices {
		vbo_data = append(vbo_data, float32(vertex.X), float32(vertex.Y), float32(vertex.Z))
	}
	gl.BufferData(gl.ARRAY_BUFFER, len(vbo_data)*4, gl.Ptr(vbo_data), gl.STATIC_DRAW)

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

	vertexAttrib := uint32(gl.GetAttribLocation(program, gl.Str("vertex\x00")))

	matrixID := gl.GetUniformLocation(program, gl.Str("MVP\x00"))

	gl.Enable(gl.DEPTH_TEST)
	gl.DepthFunc(gl.LESS)
	gl.ClearColor(0.3, 0.3, 0.3, 1.0)

	for !window.ShouldClose() {
		gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)

		gl.UseProgram(program)

		gl.VertexAttribPointer(vertexAttrib, 3, gl.FLOAT, false, 0, nil)
		gl.EnableVertexAttribArray(vertexAttrib)

		projection := mgl32.Perspective(64.0, float32(width)/float32(height), 1.0, 10000.0)
		view := mgl32.LookAt(eye.X(), eye.Y(), eye.Z(), eye.X()+direction.X(), eye.Y()+direction.Y(), eye.Z()+direction.Z(), 0.0, 1.0, 0.0)
		model := mgl32.Ident4()
		mvp := projection.Mul4(view).Mul4(model)

		gl.UniformMatrix4fv(matrixID, 1, false, &mvp[0])

		gl.DrawArrays(gl.TRIANGLES, 0, int32(len(vbo_data)))

		window.SwapBuffers()
		glfw.PollEvents()

		if window.GetKey(glfw.KeyEscape) == glfw.Press {
			window.SetShouldClose(true)
		}
		if window.GetKey(glfw.KeyUp) == glfw.Press {
			eye = eye.Add(direction.Mul(speed))
		}
		if window.GetKey(glfw.KeyDown) == glfw.Press {
			eye = eye.Sub(direction.Mul(speed))
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
