package player

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
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

// ExpectedDuration estimates the tab's playing time from its written
// content — bars × beats per bar × 60/BPM — the length signal used to rank
// online candidates. BPM comes from the tab metadata (default 120); beats
// per bar comes from the first bar carrying tick or column data (default
// 4/4 when the tab has neither). Returns 0 for a tab with no bars.
func ExpectedDuration(tab *model.Tab) time.Duration {
	if tab == nil || len(tab.Bars) == 0 {
		return 0
	}
	bpm := TabBPM(tab)
	if bpm <= 0 {
		bpm = DefaultBPM
	}
	beatsPerBar := 4
	for _, b := range tab.Bars {
		if n := barBeatCount(b); n > 0 {
			beatsPerBar = n
			break
		}
	}
	secs := float64(len(tab.Bars)) * float64(beatsPerBar) * 60.0 / float64(bpm)
	return time.Duration(secs * float64(time.Second))
}

// barBeatCount returns how many quarter-note beats a bar spans. GP-imported
// per-column ticks are authoritative; otherwise the bar's written column
// span at the sixteenth-note-per-column rule (the same heuristic restBarTicks
// uses) gives the meter, so partial rhythm rows never skew the count.
// Returns 0 when the bar carries no data.
func barBeatCount(bar model.Bar) int {
	var ticks int
	for _, t := range bar.ColumnTicks {
		ticks += t
	}
	if ticks > 0 {
		beats := (ticks + ticksPerQuarter/2) / ticksPerQuarter
		if beats < 1 {
			beats = 1
		}
		return beats
	}
	cols := maxColumns(bar.Strings)
	if cols > 0 {
		return (cols + 3) / 4 // ceil(cols/4): 16 cols → 4/4, 12 → 3/4
	}
	return 0
}

// smartMatchScore layers the smart-matching terms on top of the base
// ScoreYouTubeResult score and returns a human-readable fragment describing
// what matched ("" when nothing did). Terms: expected-duration closeness
// (within ±30% the boost grows with closeness), transformed-audio keyword
// penalties (nightcore, sped up, slowed, 8D, karaoke, reaction, remix), the
// cover rule (a cover is only acceptable when the performing channel is the
// tab's own artist), and channel reputation (VEVO, Topic, artist channel).
func smartMatchScore(tab *model.Tab, src *AudioSource, title, channel string) string {
	if tab == nil || src == nil {
		return ""
	}
	score := src.Score
	var frags []string

	// Expected-duration closeness: a candidate whose length is within ±30%
	// of the tab's expected length gets a boost proportional to how close
	// it is (up to 60 for an exact match).
	if exp := ExpectedDuration(tab); exp > 0 && src.Duration > 0 {
		ratio := float64(src.Duration) / float64(exp)
		if ratio >= 0.7 && ratio <= 1.3 {
			closeness := 1 - math.Abs(ratio-1)
			score += int(closeness * 60)
			frags = append(frags, fmt.Sprintf("duration %s ≈ tab %s", formatDuration(src.Duration), formatDuration(exp)))
		}
	}

	// Hyphens are normalized to spaces so "sped-up" matches "sped up".
	t := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(title), "-", " "))
	ch := strings.ToLower(strings.TrimSpace(channel))
	artist := strings.ToLower(strings.TrimSpace(tab.Artist))

	// Transformed-audio and non-performance uploads: a strong per-keyword
	// weight so one bad token can never be outweighed by generic keyword
	// noise in the base score.
	var penalized []string
	for _, kw := range []string{"nightcore", "sped up", "slowed", "8d", "karaoke", "reaction", "remix"} {
		if strings.Contains(t, kw) {
			score -= 70
			penalized = append(penalized, kw)
		}
	}
	// "cover" is a penalty unless the performing channel is the tab's own
	// artist: an artist-channel cover is usually a re-recording, everyone
	// else's cover fights the tab's arrangement.
	if strings.Contains(t, "cover") && (artist == "" || !strings.Contains(ch, artist)) {
		score -= 50
		penalized = append(penalized, "cover")
	}
	if len(penalized) > 0 {
		frags = append(frags, "penalized: "+strings.Join(penalized, ", "))
	}

	// Channel reputation: official auto-generated and artist channels are
	// the most likely to carry the studio recording.
	if strings.Contains(ch, "vevo") || strings.Contains(ch, "topic") {
		score += 30
		frags = append(frags, "official channel")
	} else if artist != "" && strings.Contains(ch, artist) {
		score += 30
		frags = append(frags, "artist channel")
	}

	src.Score = score
	return strings.Join(frags, " · ")
}

// onlineSourceFromEntry builds one ranked AudioSource from a search entry,
// applying the smart-match refinement. It returns the pick-reason fragment
// describing what matched.
func onlineSourceFromEntry(tab *model.Tab, e ytSearchEntry, strictOK bool) (AudioSource, string) {
	channel := e.Channel
	if channel == "" {
		channel = e.Uploader
	}
	cat := ClassifyAudioCandidate(tab.Artist, tab.Title, e.Title, channel, e.Description)
	dur := time.Duration(e.Duration) * time.Second
	path := cachedPathForVideo(tab, e.ID)
	if fileExists(path) {
		if probed, err := ProbeDuration(path); err == nil && probed > 0 {
			dur = probed
		}
	}
	src := AudioSource{
		ID:       "yt:" + e.ID,
		Kind:     SourceOnline,
		Label:    e.Title,
		Path:     path,
		VideoID:  e.ID,
		Duration: dur,
		Detail:   formatDuration(dur) + " · " + channel + " · online",
		Category: cat,
		StrictOK: strictOK,
	}
	reason := smartMatchScore(tab, &src, e.Title, channel)
	return src, reason
}
