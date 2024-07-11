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

const (
	TagOpener           string = "#EXTM3U"
	TagBandwidth        string = "##X-BANDWIDTH:"
	TagCodecs           string = "##X-CODECS:"
	TagResolution       string = "##X-RESOLUTION:"
	TagVersion          string = "#EXT-X-VERSION:"
	TagMediaSequence    string = "#EXT-X-MEDIA-SEQUENCE:"
	TagAllowCache       string = "#EXT-X-ALLOW-CACHE:"
	TagTargetDuration   string = "#EXT-X-TARGETDURATION:"
	TagDiscontinuity    string = "#EXT-X-DISCONTINUITY"
	TagProgramDateTime  string = "#EXT-X-PROGRAM-DATE-TIME:"
	TagInitFile         string = "#EXT-X-MAP:URI="
	TagFragmentDuration string = "#EXTINF:"
	TagEndList          string = "#EXT-X-ENDLIST"
)

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

		if strings.HasPrefix(line, TagBandwidth) {
			manifest.Bandwidth, _ = strconv.Atoi(strings.TrimPrefix(line, TagBandwidth))
			continue
		}

		if strings.HasPrefix(line, TagCodecs) {
			manifest.Codecs = strings.TrimPrefix(line, TagCodecs)
			continue
		}

		if strings.HasPrefix(line, TagResolution) {
			resolution := strings.Split(strings.TrimPrefix(line, TagResolution), "x")
			manifest.ResolutionWidth, _ = strconv.Atoi(resolution[0])
			if len(resolution) > 1 {
				manifest.ResolutionHeight, _ = strconv.Atoi(resolution[1])
			}
			continue
		}

		if strings.HasPrefix(line, TagVersion) {
			manifest.Version, _ = strconv.Atoi(strings.TrimPrefix(line, TagVersion))
			continue
		}

		if strings.HasPrefix(line, TagMediaSequence) {
			manifest.MediaSequence, _ = strconv.Atoi(strings.TrimPrefix(line, TagMediaSequence))
			continue
		}

		if strings.HasPrefix(line, TagAllowCache) {
			manifest.AllowCache = strings.TrimPrefix(line, TagAllowCache) == "YES"
			continue
		}

		if strings.HasPrefix(line, TagTargetDuration) {
			manifest.TargetDuration, _ = strconv.ParseFloat(strings.TrimPrefix(line, TagTargetDuration), 64)
			continue
		}

		if line == TagDiscontinuity {
			manifest.Discontinuities = append(manifest.Discontinuities, Discontinuity{})
			continue
		}

		lastIndex := len(manifest.Discontinuities) - 1
		if strings.HasPrefix(line, TagProgramDateTime) {
			var err error
			manifest.Discontinuities[lastIndex].ProgramDateTime, err = time.Parse(TimeFormat, strings.TrimPrefix(line, TagProgramDateTime))
			if err != nil {
				slog.Error("failed to parse program date time", slog.String("error", err.Error()), slog.Time("time", manifest.Discontinuities[lastIndex].ProgramDateTime))
			}
			continue
		}

		if strings.HasPrefix(line, TagInitFile) {
			initFileName := strings.TrimPrefix(line, TagInitFile)
			initFileName = strings.TrimPrefix(initFileName, `"`)
			initFileName = strings.TrimSuffix(initFileName, `"`)
			manifest.Discontinuities[lastIndex].InitFile = initFileName
			continue
		}

		if strings.HasPrefix(line, TagFragmentDuration) {
			manifestEntry := new(ManifestEntry)
			manifestEntry.Duration, _ = strconv.ParseFloat(strings.TrimSuffix(strings.TrimPrefix(line, TagFragmentDuration), ","), 64)

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
	if _, err := w.Write([]byte(TagOpener + "\n")); err != nil {
		return err
	}

	if manifest.Bandwidth != 0 {
		if _, err := w.Write([]byte(fmt.Sprintf("%s%d\n", TagBandwidth, manifest.Bandwidth))); err != nil {
			return err
		}
	}

	if manifest.Codecs != "" {
		if _, err := w.Write([]byte(fmt.Sprintf("%s%s\n", TagCodecs, manifest.Codecs))); err != nil {
			return err
		}
	}

	if manifest.ResolutionWidth != 0 && manifest.ResolutionHeight != 0 {
		if _, err := w.Write([]byte(fmt.Sprintf("%s%dx%d\n", TagResolution, manifest.ResolutionWidth, manifest.ResolutionHeight))); err != nil {
			return err
		}
	}

	if _, err := w.Write([]byte(fmt.Sprintf("%s%d\n", TagVersion, manifest.Version))); err != nil {
		return err
	}

	if _, err := w.Write([]byte(fmt.Sprintf("%s%d\n", TagMediaSequence, manifest.MediaSequence))); err != nil {
		return err
	}

	if _, err := w.Write([]byte(fmt.Sprintf("%s%s\n", TagAllowCache, manifest.AllowCacheString()))); err != nil {
		return err
	}

	if _, err := w.Write([]byte(fmt.Sprintf("%s%f\n", TagTargetDuration, manifest.TargetDuration))); err != nil {
		return err
	}

	isFmp4 := manifest.IsFmp4()
	for index, discontinuity := range manifest.Discontinuities {
		if index > 0 {
			if _, err := w.Write([]byte(TagDiscontinuity + "\n")); err != nil {
				return err
			}
		}
		if _, err := w.Write([]byte(fmt.Sprintf("%s%s\n", TagProgramDateTime, discontinuity.ProgramDateTime.Format(TimeFormat)))); err != nil {
			return err
		}
		if _, err := w.Write([]byte(fmt.Sprintf("%s\"%s\"\n", TagInitFile, discontinuity.InitFileName()))); err != nil {
			return err
		}

		for _, entry := range discontinuity.Entries {
			if _, err := w.Write([]byte(fmt.Sprintf("%s%f,\n", TagFragmentDuration, entry.Duration))); err != nil {
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

	if _, err := w.Write([]byte(TagEndList + "\n")); err != nil {
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
