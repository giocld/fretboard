package player

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"fretboard/internal/config"
)

// config default until then. A value <= 0 makes EnforceCacheCap evict every
// cache entry.
// config default until then. A value <= 0 means the cache is effectively
// unlimited-off: EnforceCacheCap evicts everything.
var AudioCacheMaxGB int64 = 5

// CacheStats reports the number of audio cache entries, their total size in
// bytes, and the cache directory itself. Only files with a recognized audio
// extension count: partial downloads (*.part) and stray files are not cache
// entries and never count toward the cap.
func CacheStats() (entries int, totalBytes int64, dir string) {
	dir, err := config.AudioDir()
	if err != nil {
		return 0, 0, ""
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return 0, 0, dir
	}
	for _, ent := range ents {
		if ent.IsDir() || !isAudioExt(strings.ToLower(filepath.Ext(ent.Name()))) {
			continue
		}
		info, err := ent.Info()
		if err != nil {
			continue
		}
		totalBytes += info.Size()
		entries++
	}
	return entries, totalBytes, dir
}

// TouchCacheEntry bumps a cache file's mtime to now, marking it the
// most-recently-played. mtime is the LRU play-time proxy: EvictLRU treats
// older mtimes as less recently used. Best-effort: callers ignore failures.
func TouchCacheEntry(path string) error {
	if path == "" {
		return nil
	}
	now := time.Now()
	return os.Chtimes(path, now, now)
}

// EvictLRU deletes cache entries oldest-mtime-first until the remaining
// cache is at or under keepBytes, returning the bytes freed. Entries that
// cannot be removed stop eviction and surface the error (with the bytes
// freed so far). Non-audio files are never touched, so an in-flight
// download's *.part file cannot be evicted out from under it.
func EvictLRU(keepBytes int64) (freedBytes int64, err error) {
	if keepBytes < 0 {
		keepBytes = 0
	}
	dir, err := config.AudioDir()
	if err != nil {
		return 0, err
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	type entry struct {
		path string
		size int64
		m    time.Time
	}
	files := make([]entry, 0, len(ents))
	var total int64
	for _, ent := range ents {
		if ent.IsDir() || !isAudioExt(strings.ToLower(filepath.Ext(ent.Name()))) {
			continue
		}
		info, err := ent.Info()
		if err != nil {
			continue
		}
		files = append(files, entry{path: filepath.Join(dir, ent.Name()), size: info.Size(), m: info.ModTime()})
		total += info.Size()
	}
	slices.SortFunc(files, func(a, b entry) int {
		switch {
		case a.m.Before(b.m):
			return -1
		case a.m.After(b.m):
			return 1
		default:
			return 0
		}
	})
	for _, f := range files {
		if total <= keepBytes {
			break
		}
		if err := os.Remove(f.path); err != nil {
			return freedBytes, fmt.Errorf("evict %s: %w", f.path, err)
		}
		total -= f.size
		freedBytes += f.size
	}
	return freedBytes, nil
}

// EnforceCacheCap evicts least-recently-played entries when the cache
// exceeds AudioCacheMaxGB, bringing it back to the cap. No-op while under
// the cap. The download finalization path (DownloadYouTubeAudio in
// audio_online.go) is the intended call site; it lives outside this file's
// territory, so Wave 2 wires it there.
func EnforceCacheCap() error {
	_, total, _ := CacheStats()
	capBytes := AudioCacheMaxGB << 30 // AudioCacheMaxGB GiB
	if total <= capBytes {
		return nil
	}
	_, err := EvictLRU(capBytes)
	return err
}

// ProgressFn receives a download progress percentage (0-100) as lines
// stream in from the downloader.
type ProgressFn func(percent float64)

// OnDownloadProgress, when non-nil, is invoked with each parsed download
// percentage. Nil-safe; the download loop feeding it belongs to
// audio_online.go's territory, so it is wired there in Wave 2.
var OnDownloadProgress ProgressFn

var downloadProgressRe = regexp.MustCompile(`^\[download\]\s+(\d+(?:\.\d+)?)%`)

// ParseProgressLine extracts the percentage from a yt-dlp --newline progress
// line, e.g. "[download]  45.3% of 4.21MiB at 1.23MiB/s ETA 00:02" -> 45.3.
// Returns ok=false for non-progress lines (headers, "Destination:",
// "[youtube]" output, empty input) and for malformed percentages.
func ParseProgressLine(line string) (percent float64, ok bool) {
	m := downloadProgressRe.FindStringSubmatch(line)
	if m == nil {
		return 0, false
	}
	p, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, false
	}
	return p, true
}

// NotifyDownloadProgress parses a downloader line and forwards its
// percentage to OnDownloadProgress. Safe to call for every streamed line
// and with a nil hook; never panics on malformed input.
func NotifyDownloadProgress(line string) {
	if OnDownloadProgress == nil {
		return
	}
	if p, ok := ParseProgressLine(line); ok {
		OnDownloadProgress(p)
	}
}
