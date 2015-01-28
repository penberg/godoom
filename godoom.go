package main

import (
	"fmt"
	"github.com/codegangsta/cli"
	"github.com/go-gl/gl"
	glfw "github.com/go-gl/glfw3"
	"os"
	"runtime"
)

const (
	vertex = `#version 330

in vec2 position;

void main()
{
    gl_Position = vec4(position, 0.0, 1.0);
}`

	fragment = `#version 330

out vec4 outColor;

void main()
{
    outColor = vec4(1.0, 1.0, 1.0, 1.0);
}`
)

const (
	subsectorBit = int(0x8000)
)

type Point struct {
	X int16
	Y int16
}

func renderSubsector(level *Level, idx int, vertices []Vertex) []Vertex {
	ssector := level.SSectors[idx]
	for seg := ssector.StartSeg; seg < ssector.StartSeg+ssector.Numsegs; seg++ {
		vertices = append(vertices, renderSeg(level, int(seg), vertices)...)
	}
	return vertices
}

func renderSeg(level *Level, idx int, vertices []Vertex) []Vertex {
	seg := level.Segs[idx]
	return renderLinedef(level, int(seg.LineNum), vertices)
}

func renderLinedef(level *Level, idx int, vertices []Vertex) []Vertex {
	linedef := level.Linedefs[idx]
	vertices = append(vertices, level.Vertexes[linedef.VertexStart])
	vertices = append(vertices, level.Vertexes[linedef.VertexEnd])
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

func traverseBsp(level *Level, point *Point, idx int, visibility bool, vertices []Vertex) []Vertex {
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
		left := traverseBsp(level, point, int(node.Child[0]), visibility, []Vertex{})
		right := traverseBsp(level, point, int(node.Child[1]), visibility, []Vertex{})
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

func errorCallback(err glfw.ErrorCode, desc string) {
	fmt.Printf("%v: %v\n", err, desc)
}

func game(level *Level, position *Point) {
	runtime.LockOSThread()

	glfw.SetErrorCallback(errorCallback)

	if !glfw.Init() {
		panic("Can't init glfw!")
	}
	defer glfw.Terminate()

	monitor, err := glfw.GetPrimaryMonitor()
	if err != nil {
		panic(err)
	}

	videoMode, err := monitor.GetVideoMode()
	if err != nil {
		panic(err)
	}

	glfw.WindowHint(glfw.ContextVersionMajor, 3)
	glfw.WindowHint(glfw.ContextVersionMinor, 3)
	glfw.WindowHint(glfw.OpenglForwardCompatible, glfw.True)
	glfw.WindowHint(glfw.OpenglProfile, glfw.OpenglCoreProfile)

	window, err := glfw.CreateWindow(videoMode.Width, videoMode.Height, "GoDoom", monitor, nil)
	if err != nil {
		panic(err)
	}

	defer window.Destroy()

	window.MakeContextCurrent()
	glfw.SwapInterval(1)

	gl.Init()

	vao := gl.GenVertexArray()
	vao.Bind()

	vbo := gl.GenBuffer()
	vbo.Bind(gl.ARRAY_BUFFER)

	vertices := traverseBsp(level, position, len(level.Nodes)-1, false, []Vertex{})

	vbo_data := []float32{}
	for _, vertex := range vertices {
		vbo_data = append(vbo_data, float32(vertex.XCoord)/32767.0*5.0, float32(vertex.YCoord)/32767.0*5.0, 0.0)
	}

	gl.BufferData(gl.ARRAY_BUFFER, len(vbo_data)*4, vbo_data, gl.STATIC_DRAW)

	vertex_shader := gl.CreateShader(gl.VERTEX_SHADER)
	vertex_shader.Source(vertex)
	vertex_shader.Compile()
	fmt.Println(vertex_shader.GetInfoLog())
	defer vertex_shader.Delete()

	fragment_shader := gl.CreateShader(gl.FRAGMENT_SHADER)
	fragment_shader.Source(fragment)
	fragment_shader.Compile()
	fmt.Println(fragment_shader.GetInfoLog())
	defer fragment_shader.Delete()

	program := gl.CreateProgram()
	program.AttachShader(vertex_shader)
	program.AttachShader(fragment_shader)

	program.BindFragDataLocation(0, "outColor")
	program.Link()
	program.Use()
	defer program.Delete()

	positionAttrib := program.GetAttribLocation("position")
	positionAttrib.AttribPointer(3, gl.FLOAT, false, 0, nil)
	positionAttrib.EnableArray()
	defer positionAttrib.DisableArray()

	gl.ClearColor(0.3, 0.3, 0.3, 1.0)

	for !window.ShouldClose() {
		gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)
		gl.DrawArrays(gl.LINES, 0, len(vbo_data))

		window.SwapBuffers()
		glfw.PollEvents()

		if window.GetKey(glfw.KeyEscape) == glfw.Press {
			window.SetShouldClose(true)
		}
	}
}
