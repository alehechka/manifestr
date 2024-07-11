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
	"time"
)

const TimeFormat = "2006-01-02T15:04:05.999Z"

type Manifest struct {
	Version          int
	MediaSequence    int
	AllowCache       bool
	TargetDuration   float64
	Bandwidth        int
	Codecs           string
	ResolutionHeight int
	ResolutionWidth  int
	Discontinuities  []Discontinuity
	BaseUrl          *url.URL
}

func (manifest Manifest) AllowCacheString() string {
	if manifest.AllowCache {
		return "YES"
	}
	return "NO"
}

func (manifest Manifest) IsFmp4() bool {
	for _, discontinuity := range manifest.Discontinuities {
		if discontinuity.InitFile != "" {
			return true
		}
	}

	return false
}

func (manifest Manifest) DownloadAllFragments(dir string, forceDownload bool) {
	var wg sync.WaitGroup
	isFmp4 := manifest.IsFmp4()

	for _, discontinuity := range manifest.Discontinuities {
		if isFmp4 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				initFileName := discontinuity.InitFileName()
				initFileUrl := discontinuity.DynamicInitFile(manifest.BaseUrl).String()
				if _, err := utils.DownloadFile(dir, initFileName, initFileUrl, forceDownload); err != nil {
					slog.Error("failed to download fragment", slog.String("url", initFileUrl), slog.String("file", initFileName))
				}
			}()
		}

		for _, entry := range discontinuity.Entries {
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
	}

	wg.Wait()
}

func ReadManifestFromFile(manifestPath string, sourceUrl string) (*Manifest, error) {
	manifestFile, err := os.Open(manifestPath)
	if err != nil {
		return nil, err
	}
	defer manifestFile.Close()

	return ReadManifest(manifestFile, sourceUrl)
}

func ReadManifest(r io.Reader, sourceUrl string) (*Manifest, error) {
	manifest := new(Manifest)

	manifest.BaseUrl, _ = url.Parse(sourceUrl)
	manifest.BaseUrl.Path = strings.TrimSuffix(manifest.BaseUrl.Path, path.Base(manifest.BaseUrl.Path))

	scanner := bufio.NewScanner(r)

	manifest.Discontinuities = make([]Discontinuity, 1)
	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "##X-BANDWIDTH:") {
			manifest.Bandwidth, _ = strconv.Atoi(strings.TrimPrefix(line, "##X-BANDWIDTH:"))
			continue
		}

		if strings.HasPrefix(line, "##X-CODECS:") {
			manifest.Codecs = strings.TrimPrefix(line, "##X-CODECS:")
			continue
		}

		if strings.HasPrefix(line, "##X-RESOLUTION:") {
			resolution := strings.Split(strings.TrimPrefix(line, "##X-RESOLUTION:"), "x")
			manifest.ResolutionWidth, _ = strconv.Atoi(resolution[0])
			if len(resolution) > 1 {
				manifest.ResolutionHeight, _ = strconv.Atoi(resolution[1])
			}
			continue
		}

		if strings.HasPrefix(line, "#EXT-X-VERSION:") {
			manifest.Version, _ = strconv.Atoi(strings.TrimPrefix(line, "#EXT-X-VERSION:"))
			continue
		}

		if strings.HasPrefix(line, "#EXT-X-MEDIA-SEQUENCE:") {
			manifest.MediaSequence, _ = strconv.Atoi(strings.TrimPrefix(line, "#EXT-X-MEDIA-SEQUENCE:"))
			continue
		}

		if strings.HasPrefix(line, "#EXT-X-ALLOW-CACHE:") {
			manifest.AllowCache = strings.TrimPrefix(line, "#EXT-X-ALLOW-CACHE:") == "YES"
			continue
		}

		if strings.HasPrefix(line, "#EXT-X-TARGETDURATION:") {
			manifest.TargetDuration, _ = strconv.ParseFloat(strings.TrimPrefix(line, "#EXT-X-TARGETDURATION:"), 64)
			continue
		}

		if line == "#EXT-X-DISCONTINUITY" {
			manifest.Discontinuities = append(manifest.Discontinuities, Discontinuity{})
			continue
		}

		lastIndex := len(manifest.Discontinuities) - 1
		if strings.HasPrefix(line, "#EXT-X-PROGRAM-DATE-TIME:") {
			var err error
			manifest.Discontinuities[lastIndex].ProgramDateTime, err = time.Parse(TimeFormat, strings.TrimPrefix(line, "#EXT-X-PROGRAM-DATE-TIME:"))
			if err != nil {
				slog.Error("failed to parse program date time", slog.String("error", err.Error()), slog.Time("time", manifest.Discontinuities[lastIndex].ProgramDateTime))
			}
			continue
		}

		if strings.HasPrefix(line, "#EXT-X-MAP:URI=") {
			initFileName := strings.TrimPrefix(line, "#EXT-X-MAP:URI=")
			initFileName = strings.TrimPrefix(initFileName, `"`)
			initFileName = strings.TrimSuffix(initFileName, `"`)
			manifest.Discontinuities[lastIndex].InitFile = initFileName
			continue
		}

		if strings.HasPrefix(line, "#EXTINF:") {
			manifestEntry := new(ManifestEntry)
			manifestEntry.Duration, _ = strconv.ParseFloat(strings.TrimSuffix(strings.TrimPrefix(line, "#EXTINF:"), ","), 64)

			if !scanner.Scan() {
				break
			}
			manifestEntry.Url = scanner.Text()

			manifest.Discontinuities[lastIndex].Entries = append(manifest.Discontinuities[lastIndex].Entries, manifestEntry)
			continue
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

	if manifest.Bandwidth != 0 {
		if _, err := w.Write([]byte(fmt.Sprintf("##X-BANDWIDTH:%d\n", manifest.Bandwidth))); err != nil {
			return err
		}
	}

	if manifest.Codecs != "" {
		if _, err := w.Write([]byte(fmt.Sprintf("##X-CODECS:%s\n", manifest.Codecs))); err != nil {
			return err
		}
	}

	if manifest.ResolutionWidth != 0 && manifest.ResolutionHeight != 0 {
		if _, err := w.Write([]byte(fmt.Sprintf("##X-RESOLUTION:%dx%d\n", manifest.ResolutionWidth, manifest.ResolutionHeight))); err != nil {
			return err
		}
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
	for index, discontinuity := range manifest.Discontinuities {
		if index > 0 {
			if _, err := w.Write([]byte("#EXT-X-DISCONTINUITY\n")); err != nil {
				return err
			}
		}
		if _, err := w.Write([]byte(fmt.Sprintf("#EXT-X-PROGRAM-DATE-TIME:%s\n", discontinuity.ProgramDateTime.Format(TimeFormat)))); err != nil {
			return err
		}
		if _, err := w.Write([]byte(fmt.Sprintf("#EXT-X-MAP:URI=\"%s\"\n", discontinuity.InitFileName()))); err != nil {
			return err
		}

		for _, entry := range discontinuity.Entries {
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

type Discontinuity struct {
	ProgramDateTime time.Time
	InitFile        string
	Entries         ManifestEntries
}

func (discontinuity Discontinuity) DynamicInitFile(baseUrl *url.URL) *url.URL {
	u, _ := baseUrl.Parse(discontinuity.InitFile)
	return u
}

func (discontinuity Discontinuity) InitFileName() string {
	return fmt.Sprintf("%s.mp4", strings.TrimSuffix(discontinuity.InitFile, path.Ext(discontinuity.InitFile)))
}

type ManifestEntries []*ManifestEntry

// Runtime returns a calculated runtime based on the reported duration of all fragments in the manifest. This may not be accurate to the actual durations of fragments.
func (entires ManifestEntries) Runtime() (runtime float64) {
	for _, entry := range entires {
		runtime += entry.Duration
	}
	return
}
