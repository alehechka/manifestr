package cmd

import (
	"errors"
	"fmt"
	"manifestr/pkg/models"
	"manifestr/pkg/utils"

	"github.com/urfave/cli/v2"
)

const (
	ArgDirectory     = "directory"
	ArgForceDownload = "force-download"
)

var hlsFlags = []cli.Flag{
	&cli.StringFlag{
		Name:    ArgDirectory,
		Aliases: []string{"d", "dir"},
		Usage:   fmt.Sprintf("Specify a directory to download files to and/or use as an existing location to skip downloading files that exist (see --%s for more details).", ArgForceDownload),
	},
	&cli.BoolFlag{
		Name:  ArgForceDownload,
		Usage: fmt.Sprintf("Used in conjunction with --%s to force download all files in a manifest when they exist in the provided directory.", ArgDirectory),
	},
}

func hls(ctx *cli.Context) (err error) {
	// slog.SetLogLoggerLevel(slog.LevelDebug)

	manifestUrl := ctx.Args().Get(0)
	if manifestUrl == "" {
		return errors.New("no manifest url provided")
	}

	forceDownload := ctx.Bool(ArgForceDownload)
	directory, err := utils.CreateDirectoryOrTemp(ctx.String(ArgDirectory))
	if err != nil {
		return err
	}

	manifestPath, err := utils.DownloadFile(directory, "original.manifest.m3u8", manifestUrl, forceDownload)
	if err != nil {
		return err
	}

	manifest, err := models.ReadManifestFromFile(manifestPath, manifestUrl)
	if err != nil {
		return err
	}

	if err := manifest.WriteLocalManifestToFile(directory); err != nil {
		return err
	}

	manifest.DownloadAllFragments(directory, forceDownload)

	if _, err := manifest.ConcatToMp4s(directory); err != nil {
		return err
	}

	return
}

var HlsCommand = &cli.Command{
	Name:   "hls",
	Usage:  "Run the application against a given HLS manifest url",
	Action: hls,
	Flags:  hlsFlags,
}
