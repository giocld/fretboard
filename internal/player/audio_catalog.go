package player

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"fretboard/internal/model"
)

// AudioSourceKind identifies how a backing track is provided.
type AudioSourceKind string

const (
	SourceMIDI   AudioSourceKind = "midi"
	SourceLocal  AudioSourceKind = "local"
	SourceOnline AudioSourceKind = "online"
)

// AudioSource is one selectable playback option for a tab.
type AudioSource struct {
	ID       string
	Kind     AudioSourceKind
	Label    string
	Path     string
	VideoID  string
	Duration time.Duration
	Score    int
	Detail   string
}

// AudioCatalog lists MIDI, local files, and ranked online matches.
type AudioCatalog struct {
	Sources []AudioSource
}

// Selected returns the source at idx or nil.
func (c AudioCatalog) Selected(idx int) *AudioSource {
	if idx < 0 || idx >= len(c.Sources) {
		return nil
	}
	s := c.Sources[idx]
	return &s
}

// FindByID returns the index of a source with the given id.
func (c AudioCatalog) FindByID(id string) int {
	for i, s := range c.Sources {
		if s.ID == id {
			return i
		}
	}
	return -1
}

// SetSourcePath records a resolved filesystem path for a catalog entry.
func (c *AudioCatalog) SetSourcePath(idx int, path string) {
	if c == nil || idx < 0 || idx >= len(c.Sources) {
		return
	}
	c.Sources[idx].Path = path
}

// BestIndex returns the highest-scoring non-MIDI source, or 0.
func (c AudioCatalog) BestIndex() int {
	best := -1
	bestScore := -1<<31 - 1
	for i, s := range c.Sources {
		if s.Kind == SourceMIDI {
			continue
		}
		if s.Score > bestScore {
			bestScore = s.Score
			best = i
		}
	}
	if best >= 0 {
		return best
	}
	return 0
}

// BuildAudioCatalog gathers local files and online search hits for a tab.
func BuildAudioCatalog(tab *model.Tab, tabPath string, extraDirs []string, searchOnline bool) (AudioCatalog, error) {
	cat := AudioCatalog{
		Sources: []AudioSource{{
			ID:     "midi",
			Kind:   SourceMIDI,
			Label:  "MIDI synthesizer",
			Detail: "fluidsynth — follows tab BPM",
			Score:  0,
		}},
	}
	if tab == nil {
		return cat, nil
	}

	seen := map[string]struct{}{"midi": {}}
	addLocal := func(path string) {
		if path == "" {
			return
		}
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		dur, _ := ProbeDuration(path)
		cat.Sources = append(cat.Sources, AudioSource{
			ID:       "local:" + path,
			Kind:     SourceLocal,
			Label:    filepath.Base(path),
			Path:     path,
			Duration: dur,
			Score:    100,
			Detail:   formatDuration(dur) + " · local file",
		})
	}

	if path := FindAudio(tab, tabPath, extraDirs); path != "" {
		addLocal(path)
	}

	if searchOnline && OnlineAudioAvailable() && AudioSearchQuery(tab) != "" {
		online, err := SearchOnlineCandidates(tab, 8)
		if err != nil {
			return cat, err
		}
		for _, src := range online {
			if _, ok := seen[src.ID]; ok {
				continue
			}
			seen[src.ID] = struct{}{}
			cat.Sources = append(cat.Sources, src)
		}
	}

	return cat, nil
}

func formatDuration(d time.Duration) string {
	if d <= 0 {
		return "?:??"
	}
	sec := int(d.Round(time.Second) / time.Second)
	return fmt.Sprintf("%d:%02d", sec/60, sec%60)
}

// ScoreYouTubeResult ranks a search hit for guitar-tab playback.
func ScoreYouTubeResult(tab *model.Tab, title, channel, description string, durationSec int) int {
	if tab == nil {
		return 0
	}
	score := 0
	artist := strings.ToLower(strings.TrimSpace(tab.Artist))
	song := strings.ToLower(strings.TrimSpace(tab.Title))
	t := strings.ToLower(title)
	ch := strings.ToLower(channel)
	desc := strings.ToLower(description)

	if artist != "" {
		if strings.Contains(ch, artist) || strings.Contains(t, artist) {
			score += 18
		}
	}
	if song != "" && strings.Contains(t, song) {
		score += 18
	}

	for _, kw := range []string{"guitar", "acoustic", "electric", "official audio", "official video", "studio", "original"} {
		if strings.Contains(t, kw) || strings.Contains(desc, kw) {
			score += 6
		}
	}
	for _, kw := range []string{"lesson", "tutorial", "how to play", "how to", "tabs", "tab ", "chord chart", "reaction", "interview", "explained", "breakdown"} {
		if strings.Contains(t, kw) {
			score -= 25
		}
	}
	for _, kw := range []string{"orchestral", "piano", "drum cover", "bass only", "karaoke", "8d audio", "slowed", "sped up", "nightcore"} {
		if strings.Contains(t, kw) {
			score -= 10
		}
	}
	if strings.Contains(t, "cover") && artist != "" && !strings.Contains(ch, artist) {
		score -= 6
	}
	if strings.Contains(t, "live") {
		score -= 4
	}
	if strings.Contains(t, "backing track") || strings.Contains(t, "instrumental") {
		score += 4
	}

	// Prefer studio-length recordings over long lessons.
	if durationSec > 0 {
		if durationSec > 900 {
			score -= 8
		} else if durationSec >= 120 && durationSec <= 420 {
			score += 4
		}
	}

	return score
}
