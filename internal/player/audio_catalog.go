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

// AudioCategory classifies what kind of performance a candidate is, so the
// picker can tell a studio recording apart from live shows, covers, lessons,
// and backing tracks — the first step to staying synced with the tab.
type AudioCategory string

const (
	CatOfficial AudioCategory = "official" // studio/official recording of the song
	CatBacking  AudioCategory = "backing"  // instrumental backing track / karaoke
	CatLive     AudioCategory = "live"     // concert / TV / session performance
	CatCover    AudioCategory = "cover"    // performed by someone else (or reworked)
	CatLesson   AudioCategory = "lesson"   // tutorial / how-to-play
	CatOther    AudioCategory = "other"    // unclassifiable
	CatLocal    AudioCategory = "local"    // user-provided file, always acceptable
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
	ID       string
	Kind     AudioSourceKind
	Label    string
	Path     string
	VideoID  string
	Duration time.Duration
	Score    int
	Detail   string
	Category AudioCategory
	StrictOK bool // acceptable under strict studio-lock selection
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

// containsAny reports whether s contains any of the keywords.
func containsAny(s string, kws ...string) bool {
	for _, kw := range kws {
		if strings.Contains(s, kw) {
			return true
		}
	}
	return false
}

// ClassifyLocalFilename classifies a local audio file from its name, so the
// strict picker treats "Song (Live).mp3" the same way it treats a live
// YouTube result instead of blindly trusting any local file.
func ClassifyLocalFilename(name string) AudioCategory {
	n := strings.ToLower(strings.TrimSpace(name))
	// Lessons first, then covers (before live: "live cover"), then live.
	switch {
	case containsAny(n, "lesson", "tutorial", "how to play", "chord chart"):
		return CatLesson
	case containsAny(n, "karaoke", "backing track", "minus one", "without vocals", "no vocals"):
		return CatBacking
	case containsAny(n, "cover", "tribute", "reimagined", "reinterpretation"):
		return CatCover
	case containsAny(n, "live", "mtv unplugged", "session", "soundcheck"):
		return CatLive
	case containsAny(n, "official audio", "official video", "official music video", "(official)", "studio"):
		return CatOfficial
	default:
		return CatLocal
	}
}

func formatDuration(d time.Duration) string {
	if d <= 0 {
		return "?:??"
	}
	sec := int(d.Round(time.Second) / time.Second)
	return fmt.Sprintf("%d:%02d", sec/60, sec%60)
}

// ClassifyAudioCandidate detects the performance type of a YouTube result
// from its title, channel, and description. Classification is deliberately
// conservative: ambiguous results are CatOther, never guessed as official.
func ClassifyAudioCandidate(artist, song, title, channel, description string) AudioCategory {
	artist = strings.ToLower(strings.TrimSpace(artist))
	song = strings.ToLower(strings.TrimSpace(song))
	t := strings.ToLower(strings.TrimSpace(title))
	ch := strings.ToLower(strings.TrimSpace(channel))
	desc := strings.ToLower(strings.TrimSpace(description))

	hasWord := func(s, w string) bool {
		return strings.Contains(s, " "+w) || strings.HasPrefix(s, w+" ") || strings.HasSuffix(s, " "+w) || s == w ||
			strings.Contains(s, "("+w+")") || strings.Contains(s, "["+w+"]") || strings.Contains(s, " - "+w+" ") || strings.Contains(s, " "+w+" ")
	}

	// Lessons and tutorials first: they are never what we want to play along
	// with, even on an artist channel.
	if containsAny(t+" "+desc, "lesson", "tutorial", "how to", "guitar tab", "tablature", "chord chart", "reaction", "breakdown", "explained") {
		return CatLesson
	}
	// Karaoke and backing tracks are instrumentals with the same arrangement.
	if containsAny(t, "backing track", "instrumental", "karaoke", "minus one", "without vocals", "no vocals") {
		return CatBacking
	}
	// Cover: someone else's version (channel not the artist). An artist
	// channel covering their own song is still a cover performance.
	if containsAny(t, "cover", "tribute", "reimagined", "reinterpretation", "acoustic cover", "metal cover") {
		return CatCover
	}
	// Live performances: concerts, sessions, and TV appearances drift from
	// the studio tempo.
	if hasWord(t, "live") || containsAny(t, "live at", "live in", "mtv unplugged", "session", "soundcheck") || containsAny(ch, "live") {
		return CatLive
	}
	// Official markers are the strongest studio signal.
	if containsAny(t, "official audio", "official video", "official music video", "official visualizer", "official lyric", "official audio stream") {
		return CatOfficial
	}
	if strings.Contains(ch, "vevo") {
		return CatOfficial
	}
	// An artist channel posting the song is a studio or official recording.
	if artist != "" && strings.Contains(ch, artist) && song != "" && strings.Contains(t, song) {
		return CatOfficial
	}
	return CatOther
}

// ScoreYouTubeResult ranks a search hit for guitar-tab playback. The score
// rewards identity (artist + song in title/channel), provenance (official
// markers, VEVO, artist channel), and sane length, and hard-penalizes
// performances that fight the tab's tempo (live, cover, lesson, remixes).
func ScoreYouTubeResult(tab *model.Tab, title, channel, description string, durationSec int) int {
	if tab == nil {
		return 0
	}
	artist := strings.ToLower(strings.TrimSpace(tab.Artist))
	song := strings.ToLower(strings.TrimSpace(tab.Title))
	t := strings.ToLower(title)
	ch := strings.ToLower(channel)
	desc := strings.ToLower(description)

	score := 0

	// Identity: the song and the artist are named.
	if song != "" {
		if strings.Contains(t, song) {
			score += 18
		}
	}
	if artist != "" {
		if strings.Contains(ch, artist) || strings.Contains(t, artist) {
			score += 18
		}
	}
	if artist != "" && song != "" && strings.Contains(t, artist) && strings.Contains(t, song) {
		score += 12 // "Artist - Song" style full title
	}
	if song != "" && strings.HasPrefix(t, song) {
		score += 6
	}

	// Provenance.
	if artist != "" && strings.Contains(ch, artist) {
		score += 20
	}
	if strings.Contains(ch, "vevo") {
		score += 25
	}
	if containsAny(t, "official audio", "official video", "official music video", "official visualizer", "official lyric") {
		score += 30
	}

	// Generic guitar-friendly content.
	for _, kw := range []string{"guitar", "acoustic", "electric", "studio", "original", "remastered"} {
		if strings.Contains(t, kw) || strings.Contains(desc, kw) {
			score += 6
		}
	}

	// Length sanity: prefer typical song lengths over live marathons and
	// clip-length edits.
	if durationSec > 0 {
		if durationSec > 900 {
			score -= 8
		} else if durationSec >= 120 && durationSec <= 480 {
			score += 6
		}
	}
	// Duration proximity: the recording should be about as long as the tab
	// implies. A live jam twice the length and a 40-second clip both fail
	// this test even when the keywords miss them.
	if want := ScheduleDurationSeconds(tab, TabBPM(tab)); want > 30 && durationSec > 0 {
		ratio := float64(durationSec) / want
		switch {
		case ratio >= 0.95 && ratio <= 1.08:
			score += 40 // the right length: studio version territory
		case ratio >= 0.8 && ratio < 0.95:
			score += 12 // radio edit: shorter but same recording
		case ratio > 1.6:
			score -= 25 // live marathon / medley
		case ratio < 0.5:
			score -= 10 // clip / preview
		}
	}

	// Category penalties — the sync killers.
	switch ClassifyAudioCandidate(tab.Artist, tab.Title, title, channel, description) {
	case CatLesson:
		score -= 100
	case CatCover:
		score -= 60
	case CatLive:
		score -= 55
	case CatBacking:
		score += 10 // same arrangement, useful for practice
		if strings.Contains(t, "karaoke") {
			score -= 40
		}
	}
	if containsAny(t, "8d audio", "nightcore", "slowed", "sped up", "remix", "bass boost", "reverb") {
		score -= 70
	}
	if strings.Contains(t, "cover") && artist != "" && !strings.Contains(ch, artist) {
		score -= 6
	}

	return score
}
