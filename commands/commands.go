package commands

import (
	"github.com/urfave/cli/v2"
)

var Commands = []*cli.Command{
	{
		Name:   "serve",
		Usage:  "Start web server",
		Action: serve,
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name: "port",
				//Required: true,
			},
			&cli.StringFlag{
				Name:  "env",
				Value: "dev",
				Usage: "Environment to run, either 'dev', or 'prod'",
			},
		},
	},
}
