package main

import (
	"os"
	"runtime/debug"

	"github.com/achernya/nonstick/commands"
	"github.com/urfave/cli/v2"
	"github.com/rs/zerolog/log"
)

func main() {
	log.Logger = log.With().Caller().Logger()

	app := cli.NewApp()
	app.Name = "nonstick"
	app.Usage = "nonstick PAM IdP"
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		log.Fatal().Msg("failed to read build information")
	}
	app.Version = bi.Main.Version
	app.Commands = commands.Commands

	if err := app.Run(os.Args); err != nil {
		log.Error().Err(err)
	}
}
