package main

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
)

func main() {
	manifestUrl := os.Args[1]
	if manifestUrl == "" {
		panic("no manifest url provided")
	}

	tempDir, err := createTempDir()
	if err != nil {
		panic(err)
	}

	manifestFilePath, err := downloadFile(tempDir, "manifest.m3u8", manifestUrl)
	if err != nil {
		panic(err)
	}

	manifest, err := readManifest(manifestFilePath)
	if err != nil {
		panic(err)
	}
	fmt.Println(manifest.Version, manifest.MediaSequence, manifest.AllowCache, manifest.TargetDuration, len(manifest.Entries), manifest.Entries.Runtime(), manifest.Entries[0])

	if err := manifest.Entries.DownloadAll(tempDir); err != nil {
		panic(err)
	}
}

type Manifest struct {
	Version        int
	MediaSequence  int
	AllowCache     bool
	TargetDuration float64
	Entries        ManifestEntries
}

type ManifestEntry struct {
	Duration float64
	Url      string
}

type ManifestEntries []*ManifestEntry

func (entires ManifestEntries) Runtime() (runtime float64) {
	for _, entry := range entires {
		runtime += entry.Duration
	}
	return
}

func (entries ManifestEntries) DownloadAll(dir string) error {
	var wg sync.WaitGroup

	for _, entry := range entries {
		wg.Add(1)
		go func() error {
			defer wg.Done()

			if _, err := downloadFile(dir, fmt.Sprintf("%s.ts", strings.TrimSuffix(path.Base(entry.Url), path.Ext(entry.Url))), entry.Url); err != nil {
				fmt.Println("Failed to download file: ", entry.Url, err)
				return err
			}
			return nil
		}()
	}

	wg.Wait()
	return nil
}

func readManifest(manifestPath string) (*Manifest, error) {
	manifest := new(Manifest)

	manifestFile, err := os.Open(manifestPath)
	if err != nil {
		return manifest, err
	}

	scanner := bufio.NewScanner(manifestFile)

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "#EXT-X-VERSION:") {
			manifest.Version, _ = strconv.Atoi(strings.TrimPrefix(line, "#EXT-X-VERSION:"))
		}

		if strings.HasPrefix(line, "#EXT-X-MEDIA-SEQUENCE:") {
			manifest.MediaSequence, _ = strconv.Atoi(strings.TrimPrefix(line, "#EXT-X-MEDIA-SEQUENCE:"))
		}

		if strings.HasPrefix(line, "#EXT-X-ALLOW-CACHE:") {
			manifest.AllowCache = strings.TrimPrefix(line, "#EXT-X-ALLOW-CACHE:") == "YES"
		}

		if strings.HasPrefix(line, "#EXT-X-TARGETDURATION:") {
			manifest.TargetDuration, _ = strconv.ParseFloat(strings.TrimPrefix(line, "#EXT-X-TARGETDURATION:"), 64)
		}

		if strings.HasPrefix(line, "#EXTINF:") {
			manifestEntry := new(ManifestEntry)
			manifestEntry.Duration, _ = strconv.ParseFloat(strings.TrimSuffix(strings.TrimPrefix(line, "#EXTINF:"), ","), 64)

			if !scanner.Scan() {
				break
			}
			manifestEntry.Url = scanner.Text()

			manifest.Entries = append(manifest.Entries, manifestEntry)
		}
	}

	return manifest, scanner.Err()
}

func createTempDir() (string, error) {
	if err := os.MkdirAll("./tmp", os.ModePerm); err != nil {
		return "", err
	}

	return os.MkdirTemp("./tmp", "")
}

func downloadFile(dir string, filename string, url string) (string, error) {
	filePath := path.Join(dir, filename)

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
