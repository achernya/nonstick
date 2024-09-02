package commands

import (
	"fmt"

	"github.com/urfave/cli/v2"
)

var Commands = []*cli.Command{
	{
		Name:   "serve",
		Usage:  "Start web server",
		Action: serve,
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:     "port",
				Required: true,
			},
			&cli.StringFlag{
				Name:  "env",
				Value: "dev",
				Usage: "Environment to run, either 'dev', or 'prod'",
			},
			&cli.StringFlag{
				Name:     "csrf_secret",
				Value:    "",
				Required: true,
				EnvVars:  []string{"NONSTICK_CSRF_SECRET"},
				Usage:    "32-byte secret string for CSRF protection",
			},
			&cli.StringFlag{
				Name:  "login_flow",
				Value: "hydra",
				Usage: "Which login flow is in use [valid values: hydra, noop]",
				Action: func(ctx *cli.Context, v string) error {
					switch v {
					case "hydra":
						return nil
					case "noop":
						return nil
					default:
						return fmt.Errorf("flow %v not known", v)
					}
				},
			},
			&cli.BoolFlag{
				Name:  "use_dotenv",
				Value: false,
				Usage: "if true, read .env files",
			},
		},
	},
}
