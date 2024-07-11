package models

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"manifestr/pkg/utils"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
)

type Manifest struct {
	Version        int
	MediaSequence  int
	AllowCache     bool
	TargetDuration float64
	InitFile       string
	Entries        ManifestEntries
	BaseUrl        *url.URL
}

func (manifest Manifest) AllowCacheString() string {
	if manifest.AllowCache {
		return "YES"
	}
	return "NO"
}

func (manifest Manifest) DynamicInitFile() *url.URL {
	u, _ := manifest.BaseUrl.Parse(manifest.InitFile)
	return u
}

func (manifest Manifest) IsFmp4() bool {
	return manifest.InitFile != ""
}

func (manifest Manifest) DownloadAllFragments(dir string, forceDownload bool) {
	var wg sync.WaitGroup
	isFmp4 := manifest.IsFmp4()

	for _, entry := range manifest.Entries {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fileName := entry.MpegTsFilename()
			if isFmp4 {
				fileName = entry.Fmp4Filename()
			}

			fragmentUrl := entry.DynamicUrl(manifest.BaseUrl).String()
			if _, err := utils.DownloadFile(dir, fileName, fragmentUrl, forceDownload); err != nil {
				slog.Error("failed to download fragment", slog.String("url", fragmentUrl), slog.String("file", fileName))
			}
		}()
	}

	wg.Wait()
}

func ReadManifestFromFile(manifestPath string) (*Manifest, error) {
	manifestFile, err := os.Open(manifestPath)
	if err != nil {
		return nil, err
	}
	defer manifestFile.Close()

	return ReadManifest(manifestFile)
}

func ReadManifest(r io.Reader) (*Manifest, error) {
	manifest := new(Manifest)

	scanner := bufio.NewScanner(r)

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

func (manifest *Manifest) WriteLocalManifestToFile(dir string) error {
	manifestFile, err := os.Create(path.Join(dir, "local.manifest.m3u8"))
	if err != nil {
		return err
	}

	return manifest.WriteLocalManifest(manifestFile)
}

func (manifest *Manifest) WriteLocalManifest(w io.Writer) error {
	if _, err := w.Write([]byte("#EXTM3U\n")); err != nil {
		return err
	}

	if _, err := w.Write([]byte(fmt.Sprintf("#EXT-X-VERSION:%d\n", manifest.Version))); err != nil {
		return err
	}

	if _, err := w.Write([]byte(fmt.Sprintf("#EXT-X-MEDIA-SEQUENCE:%d\n", manifest.MediaSequence))); err != nil {
		return err
	}

	if _, err := w.Write([]byte(fmt.Sprintf("#EXT-X-ALLOW-CACHE:%s\n", manifest.AllowCacheString()))); err != nil {
		return err
	}

	if _, err := w.Write([]byte(fmt.Sprintf("#EXT-X-TARGETDURATION:%f\n", manifest.TargetDuration))); err != nil {
		return err
	}

	isFmp4 := manifest.IsFmp4()
	for _, entry := range manifest.Entries {
		if _, err := w.Write([]byte(fmt.Sprintf("#EXTINF:%f,\n", entry.Duration))); err != nil {
			return err
		}

		fileName := entry.MpegTsFilename()
		if isFmp4 {
			fileName = entry.Fmp4Filename()
		}
		if _, err := w.Write([]byte(fmt.Sprintf("%s\n", fileName))); err != nil {
			return err
		}
	}

	if _, err := w.Write([]byte("#EXT-X-ENDLIST\n")); err != nil {
		return err
	}

	return nil
}

type ManifestEntry struct {
	Duration float64
	Url      string
}

func (entry ManifestEntry) MpegTsFilename() string {
	return fmt.Sprintf("%s.ts", entry.FilenameWithoutExtension())
}

func (entry ManifestEntry) Fmp4Filename() string {
	return fmt.Sprintf("%s.m4s", entry.FilenameWithoutExtension())
}

func (entry ManifestEntry) FilenameWithoutExtension() string {
	return strings.TrimSuffix(path.Base(entry.Url), path.Ext(entry.Url))
}

func (entry ManifestEntry) DynamicUrl(baseUrl *url.URL) *url.URL {
	u, _ := baseUrl.Parse(entry.Url)
	return u
}

type ManifestEntries []*ManifestEntry

// Runtime returns a calculated runtime based on the reported duration of all fragments in the manifest. This may not be accurate to the actual durations of fragments.
func (entires ManifestEntries) Runtime() (runtime float64) {
	for _, entry := range entires {
		runtime += entry.Duration
	}
	return
}
