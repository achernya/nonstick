package main

import (
	"log"
	"os"
	"runtime/debug"

	"github.com/achernya/nonstick/commands"
	"github.com/urfave/cli/v2"
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

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
