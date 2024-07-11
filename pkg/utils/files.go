package utils

import (
	"io"
	"log/slog"
	"net/http"
	"os"
	"path"
	"strings"
)

func CreateDirectoryOrTemp(directory string) (string, error) {
	if directory == "" {
		err := os.MkdirAll("./tmp", os.ModePerm)
		if err != nil {
			return "", err
		}

		return os.MkdirTemp("./tmp", "")
	}

	return directory, os.MkdirAll(directory, os.ModePerm)
}

func DownloadFile(dir string, filename string, url string, forceDownload bool) (string, error) {
	filePath := path.Join(dir, filename)

	if _, err := os.Stat(filePath); err == nil && !forceDownload {
		slog.Debug("skipping download", slog.String("file", filePath), slog.String("url", url))
		return filePath, nil
	}

	file, err := os.Create(filePath)
	if err != nil {
		return filePath, err
	}
	defer file.Close()

	if strings.HasPrefix(url, "/") {
		b, err := os.ReadFile(url)
		if err != nil {
			return filePath, err
		}
		if err := os.WriteFile(filePath, b, 0644); err != nil {
			return filePath, err
		}
	} else {
		resp, err := http.Get(url)
		if err != nil {
			return filePath, err
		}
		defer resp.Body.Close()

		if _, err := io.Copy(file, resp.Body); err != nil {
			return filePath, err
		}
	}

	return filePath, nil
}
