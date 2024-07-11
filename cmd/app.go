package cmd

import (
	"github.com/urfave/cli/v2"
)

// App represents the CLI application
func App(version string) *cli.App {
	app := cli.NewApp()
	app.Name = "manifestr"
	app.Version = version
	app.EnableBashCompletion = true
	app.Usage = "CLI application to download full HLS manifests and perform different ffmpeg operations."
	app.Commands = []*cli.Command{
		HlsCommand,
	}
	return app
}
