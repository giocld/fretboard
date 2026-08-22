package parser

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"fretboard/internal/model"
)

var gpExtensions = []string{".gp3", ".gp4", ".gp5", ".gpx", ".gp"}

// IsGpFile reports whether path looks like a Guitar Pro file.
func IsGpFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return slices.Contains(gpExtensions, ext)
}

// ParseGPFile converts a Guitar Pro file to a model.Tab using the gp-parser
// helper binary (Rust + slundi/guitarpro).
func ParseGPFile(path string) (*model.Tab, error) {
	bin, err := findGpParser()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(bin, path)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("gp-parser: %s", strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, fmt.Errorf("gp-parser: %w", err)
	}
	return decodeGPTabJSON(out)
}

// GPTrack is one track of a Guitar Pro file. The first track of the file
// carries a fully decoded Tab (same rendering as ParseGPFile); the remaining
// tracks carry metadata only (Tab is nil).
type GPTrack struct {
	Name       string
	Instrument string
	Strings    int
	Tuning     string
	Tab        *model.Tab
}

// ParseGuitarProTracks parses every track of a Guitar Pro file via
// `gp-parser <file> --all`. The first track is decoded into a full Tab and
// the rest are returned as metadata-only tracks. If the installed gp-parser
// predates the --all flag, falls back to the single-track invocation and
// returns that track alone.
func ParseGuitarProTracks(path string) ([]GPTrack, error) {
	bin, err := findGpParser()
	if err != nil {
		return nil, err
	}
	out, err := exec.Command(bin, path, "--all").Output()
	if err != nil {
		out, err = exec.Command(bin, path).Output()
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				return nil, fmt.Errorf("gp-parser: %s", strings.TrimSpace(string(ee.Stderr)))
			}
			return nil, fmt.Errorf("gp-parser: %w", err)
		}
		tab, err := decodeGPTabJSON(out)
		if err != nil {
			return nil, err
		}
		// "track" is the metadata key gp-parser emits for the track name.
		return []GPTrack{{Name: tab.Metadata["track"], Tab: tab}}, nil
	}
	return decodeGPTracksJSON(out)
}

func findGpParser() (string, error) {
	if env := os.Getenv("FRETBOARD_GP_PARSER"); env != "" {
		if _, err := os.Stat(env); err == nil {
			return env, nil
		}
	}
	if p, err := exec.LookPath("gp-parser"); err == nil {
		return p, nil
	}
	// Relative to repo: tools/gp-parser/target/release/gp-parser[.exe]
	candidates := []string{
		"tools/gp-parser/target/release/gp-parser" + exeSuffix,
		"../tools/gp-parser/target/release/gp-parser" + exeSuffix,
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			abs, _ := filepath.Abs(c)
			return abs, nil
		}
	}
	return "", fmt.Errorf("gp-parser not found: build with `cd tools/gp-parser && cargo build --release` or set FRETBOARD_GP_PARSER")
}

type gpTabJSON struct {
	Title    string            `json:"title"`
	Artist   string            `json:"artist"`
	Tuning   []int             `json:"tuning"`
	Bars     []gpBarJSON       `json:"bars"`
	Metadata map[string]string `json:"metadata"`
}

// gpAllJSON is the envelope emitted by `gp-parser --all`: song-level
// title/artist plus one entry per track of the file.
type gpAllJSON struct {
	Title  string        `json:"title"`
	Artist string        `json:"artist"`
	Tracks []gpTrackJSON `json:"tracks"`
}

type gpTrackJSON struct {
	Name       string      `json:"name"`
	Instrument string      `json:"instrument"`
	Strings    int         `json:"strings"`
	Tuning     []int       `json:"tuning"`
	Key        string      `json:"key"`
	Bars       []gpBarJSON `json:"bars"`
}

type gpBarJSON struct {
	Number      int            `json:"number"`
	Strings     []gpStringJSON `json:"strings"`
	ColumnTicks []int          `json:"column_ticks"`
}

type gpStringJSON struct {
	Segments []gpSegmentJSON `json:"segments"`
}

type gpSegmentJSON struct {
	Char     string `json:"char"`
	Value    int    `json:"value"`
	Position int    `json:"position"`
	Width    int    `json:"width"`
}

func decodeGPTabJSON(data []byte) (*model.Tab, error) {
	var raw gpTabJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("decode gp json: %w", err)
	}
	return tabFromGPJSON(raw)
}

// tabFromGPJSON builds a model.Tab from a decoded gp-parser track payload.
// Kept as a seam so multi-track decoding can reuse the single-track
// conversion without round-tripping through bytes.
func tabFromGPJSON(raw gpTabJSON) (*model.Tab, error) {
	tab := &model.Tab{
		Title:    raw.Title,
		Artist:   raw.Artist,
		Tuning:   model.Tuning(raw.Tuning),
		Metadata: raw.Metadata,
	}
	for _, b := range raw.Bars {
		bar := model.Bar{
			Number:      b.Number,
			ColumnTicks: b.ColumnTicks,
		}
		for _, s := range b.Strings {
			line := model.StringLine{}
			for _, seg := range s.Segments {
				ch := '-'
				if seg.Char != "" {
					ch = []rune(seg.Char)[0]
				}
				line.Segments = append(line.Segments, model.Segment{
					Char:     ch,
					Value:    seg.Value,
					Position: seg.Position,
					Width:    seg.Width,
				})
			}
			bar.Strings = append(bar.Strings, line)
		}
		tab.Bars = append(tab.Bars, bar)
	}
	if len(tab.Tuning) == 0 {
		tab.Tuning = model.Standard
	}
	normalizeTabBPM(tab)
	return tab, nil
}

// decodeGPTracksJSON decodes the --all envelope: the first track becomes a
// full Tab (title/artist from the song level), the rest stay metadata-only.
func decodeGPTracksJSON(data []byte) ([]GPTrack, error) {
	var raw gpAllJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("decode gp tracks json: %w", err)
	}
	if len(raw.Tracks) == 0 {
		return nil, fmt.Errorf("decode gp tracks json: no tracks in payload")
	}
	out := make([]GPTrack, 0, len(raw.Tracks))
	for i, t := range raw.Tracks {
		gt := GPTrack{
			Name:       t.Name,
			Instrument: t.Instrument,
			Strings:    t.Strings,
			Tuning:     model.Tuning(t.Tuning).Label(),
		}
		if i == 0 {
			tab, err := tabFromGPJSON(gpTabJSON{
				Title:  raw.Title,
				Artist: raw.Artist,
				Tuning: t.Tuning,
				Bars:   t.Bars,
			})
			if err != nil {
				return nil, err
			}
			gt.Tab = tab
		}
		out = append(out, gt)
	}
	return out, nil
}
