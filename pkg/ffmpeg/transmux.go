package ffmpeg

import "os"

func TransmuxMpegTsBlob(input string, output string) error {
	if _, err := os.Stat(input); err != nil {
		return err
	}

	return Ffmpeg("-i", input, "-acodec", "copy", output)
}
