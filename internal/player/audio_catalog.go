package player

import (
	"fmt"
	"path/filepath"
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

// StrictCompatible reports whether a category is acceptable under strict
// audio selection: the same performance as the tab. Live shows, covers, and
// lessons change tempo, arrangement, or key — they are exactly what causes
// "the tab doesn't match the audio".
func StrictCompatible(c AudioCategory) bool {
	return c == CatOfficial || c == CatBacking || c == CatLocal
}

// AudioSource is one selectable playback option for a tab.
type AudioSource struct {
	ID         string
	Kind       AudioSourceKind
	Label      string
	Path       string
	VideoID    string
	Duration   time.Duration
	Score      int
	Detail     string
	Category   AudioCategory
	StrictOK   bool   // acceptable under strict studio-lock selection
	PickReason string // human-readable why this source won the ranking ("" when unranked)
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
	best := 0
	bestScore := 0
	found := false
	for i, s := range c.Sources {
		if s.Kind == SourceMIDI {
			continue
		}
		if !found || s.Score > bestScore {
			found = true
			bestScore = s.Score
			best = i
		}
	}
	if !found {
		return 0
	}
	return best
}

// HasStrictRejected reports whether the catalog contains online candidates
// that fail strict studio-lock selection (live/cover/lesson), i.e. cases
// where strict auto-pick deliberately skipped audio.
func (c AudioCatalog) HasStrictRejected() bool {
	for _, s := range c.Sources {
		if s.Kind == SourceOnline && !s.StrictOK {
			return true
		}
	}
	return false
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
		localCat := ClassifyLocalFilename(filepath.Base(path))
		cat.Sources = append(cat.Sources, AudioSource{
			ID:       "local:" + path,
			Kind:     SourceLocal,
			Label:    filepath.Base(path),
			Path:     path,
			Duration: dur,
			Score:    100,
			Detail:   formatDuration(dur) + " · local file",
			Category: localCat,
			StrictOK: StrictCompatible(localCat),
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
