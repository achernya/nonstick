package main

import (
	"log"
	"os"
	"runtime/debug"
	
	"github.com/urfave/cli/v2"
	"github.com/achernya/nonstick/commands"
)

func main() {
	app := cli.NewApp()
	app.Name = "nonstick"
	app.Usage = "nonstick PAM IdP"
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		log.Fatalf("failed to read build information")
	}
	app.Version = bi.Main.Version
	app.Commands = commands.Commands

	app.Run(os.Args)
}
