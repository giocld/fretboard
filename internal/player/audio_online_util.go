package player

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"fretboard/internal/config"
	"fretboard/internal/model"
)

// AudioSearchQueries returns several search phrases tuned for guitar tabs.
func AudioSearchQueries(tab *model.Tab) []string {
	if tab == nil {
		return nil
	}
	artist := strings.TrimSpace(tab.Artist)
	title := strings.TrimSpace(tab.Title)
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		for _, existing := range out {
			if strings.EqualFold(existing, s) {
				return
			}
		}
		out = append(out, s)
	}
	if artist != "" && title != "" {
		add(artist + " " + title + " guitar")
		add(artist + " " + title + " official audio")
		add(artist + " " + title + " studio")
		add(artist + " " + title)
	} else if title != "" {
		add(title + " guitar")
		add(title)
	} else if artist != "" {
		add(artist)
	}
	return out
}

// AudioSearchFallbackQueries returns second-pass queries used when the first
// pass found nothing: song-only engines and phrasing variants that rescue
// searches the primary list misses.
func AudioSearchFallbackQueries(tab *model.Tab) []string {
	if tab == nil {
		return nil
	}
	title := strings.TrimSpace(tab.Title)
	artist := strings.TrimSpace(tab.Artist)
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		for _, existing := range out {
			if strings.EqualFold(existing, s) {
				return
			}
		}
		out = append(out, s)
	}
	if title != "" {
		add(title + " official audio")
		add(title + " lyrics")
		add(title)
	} else if artist != "" {
		add(artist)
	}
	return out
}

var (
	ytSearchTimeout   = 12 * time.Second
	ytDownloadTimeout = 10 * time.Minute
	// errYtDlpMissing is shared by the search and download entry points so
	// the "install yt-dlp" hint stays identical everywhere it is surfaced.
	errYtDlpMissing = errors.New("yt-dlp not found — install yt-dlp (e.g. choco install yt-dlp, apt install yt-dlp)")
)

type ytSearchEntry struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Channel     string `json:"channel"`
	Uploader    string `json:"uploader"`
	Description string `json:"description"`
	Duration    int    `json:"duration"`
}

type ytSearchPlaylist struct {
	Entries []ytSearchEntry `json:"entries"`
}

func ytSearch(query string, limit int) ([]ytSearchEntry, error) {
	args := []string{
		"--dump-single-json",
		"--flat-playlist",
		"--no-warnings",
		"--quiet",
		fmt.Sprintf("ytsearch%d:%s", limit, query),
	}
	ctx, cancel := context.WithTimeout(context.Background(), ytSearchTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "yt-dlp", args...).Output()
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, errors.New("yt-dlp search timed out")
		}
		return nil, err
	}
	var playlist ytSearchPlaylist
	if err := json.Unmarshal(out, &playlist); err != nil {
		return nil, err
	}
	return playlist.Entries, nil
}

func sortAudioSources(sources []AudioSource) {
	// Stable so equal-score candidates keep search-engine order.
	sort.SliceStable(sources, func(i, j int) bool { return sources[i].Score > sources[j].Score })
}

func cacheAudioBasename(tab *model.Tab) string {
	names := audioNameCandidates(tab)
	if len(names) > 1 {
		return names[1]
	}
	if len(names) > 0 {
		return names[0]
	}
	return "track"
}

func cachedPathForVideo(tab *model.Tab, videoID string) string {
	dir, err := config.AudioDir()
	if err != nil {
		return ""
	}
	base := sanitizeAudioFilename(cacheAudioBasename(tab)) + " [" + videoID + "]"
	for _, ext := range audioExtensions {
		p := filepath.Join(dir, base+ext)
		if fileExists(p) {
			return p
		}
	}
	return ""
}

func sanitizeAudioFilename(name string) string {
	replacer := strings.NewReplacer(
		"/", "-", "\\", "-", ":", "-",
		"*", "", "?", "", "\"", "",
		"<", "", ">", "", "|", "",
	)
	name = strings.TrimSpace(replacer.Replace(name))
	if name == "" {
		return "track"
	}
	// Cut on rune boundaries so multibyte filenames never get split mid-rune.
	if w := utf8.RuneCountInString(name); w > 120 {
		runes := []rune(name)
		name = string(runes[:120])
	}
	return name
}
