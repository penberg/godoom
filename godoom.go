package main

import (
	"fmt"
	"github.com/codegangsta/cli"
	"github.com/penberg/godoom/wad"
	"os"
)

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
		_, err = wad.ReadLevel(levelName)
		if err != nil {
			fmt.Printf("error: %s\n", err)
			os.Exit(1)
		}
	}
	app.Run(os.Args)
}
