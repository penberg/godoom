package main

import (
	"fmt"
	"github.com/codegangsta/cli"
	"github.com/penberg/godoom/wad"
	"os"
)

const (
	subsectorBit = int(0x8000)
)

type Point struct {
	X int16
	Y int16
}

func renderSubsector(level *wad.Level, idx int) {
	fmt.Println(level.SSectors[idx])
}

func pointOnSide(point *Point, node *wad.Node) int {
	dx := point.X-node.X
	dy := point.Y-node.Y
	// Perp dot product:
	if dy*node.DX>>16 - node.DY>>16*dx < 0 {
		// Point is on front side:
		return 0
	}
	// Point is on the back side:
	return 1
}

func traverseBsp(level *wad.Level, point *Point, idx int) {
	if idx&subsectorBit == subsectorBit {
		if idx == -1 {
			renderSubsector(level, 0)
		} else {
			renderSubsector(level, int(uint16(idx) & ^uint16(subsectorBit)))
		}
		return
	}
	node := level.Nodes[idx]

	side := pointOnSide(point, &node)

	traverseBsp(level, point, int(node.Child[side]))

	// TODO: Traverse back space if inside node's bounding box.
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
		wad, err := wad.ReadWAD(file)
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
		traverseBsp(level, position, len(level.Nodes)-1)
	}
	app.Run(os.Args)
}
