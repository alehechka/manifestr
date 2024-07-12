package ffmpeg

import (
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

func Ffmpeg(args ...string) error {
	if len(args) == 0 {
		return errors.New("no args provided")
	}

	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return err
	}

	if args[0] != "ffmpeg" {
		args = append([]string{"ffmpeg"}, args...)
	}

	env := os.Environ()

	slog.Debug("running ffmpeg command", slog.String("args", strings.Join(args, " ")))

	return syscall.Exec(ffmpeg, args, env)
}
