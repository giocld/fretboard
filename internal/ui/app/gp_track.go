package app

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"fretboard/internal/model"
	"fretboard/internal/parser"
)

// gpTrackTab returns a fully decoded Tab for the chosen track of a Guitar
// Pro file. parser.ParseGuitarProTracks decodes only the first track into a
// Tab (the rest are metadata-only), so a non-first pick re-decodes the
// chosen track from the same gp-parser --all envelope the parser package
// pins in its tests (internal/parser/gp_tracks_test.go). The envelope is
// stable: per-track bars arrive for every track, and the decode below
// mirrors the parser's own tabFromGPJSON shape.
func gpTrackTab(path string, tracks []parser.GPTrack, idx int) (*model.Tab, error) {
	if idx >= 0 && idx < len(tracks) && tracks[idx].Tab != nil {
		return tracks[idx].Tab, nil
	}
	bin, err := gpParserBin()
	if err != nil {
		return nil, err
	}
	out, err := exec.Command(bin, path, "--all").Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("gp-parser: %s", strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, fmt.Errorf("gp-parser: %w", err)
	}
	return decodeGPTrackJSON(out, idx)
}

// gpParserBin locates the gp-parser helper exactly like the parser package
// does (FRETBOARD_GP_PARSER env override, PATH, repo-relative release
// build). Mirrored here so the picker's re-decode works without importing
// parser internals.
func gpParserBin() (string, error) {
	if env := os.Getenv("FRETBOARD_GP_PARSER"); env != "" {
		if _, err := os.Stat(env); err == nil {
			return env, nil
		}
	}
	if p, err := exec.LookPath("gp-parser"); err == nil {
		return p, nil
	}
	for _, c := range []string{
		"tools/gp-parser/target/release/gp-parser",
		"../tools/gp-parser/target/release/gp-parser",
	} {
		if _, err := os.Stat(c); err == nil {
			abs, _ := filepath.Abs(c)
			return abs, nil
		}
	}
	return "", fmt.Errorf("gp-parser not found: build with `cd tools/gp-parser && cargo build --release` or set FRETBOARD_GP_PARSER")
}

// The --all envelope, decoded identically to the parser package's shape.
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

// decodeGPTrackJSON builds a Tab from the chosen track of a --all envelope.
func decodeGPTrackJSON(data []byte, idx int) (*model.Tab, error) {
	var raw gpAllJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("decode gp json: %w", err)
	}
	if idx < 0 || idx >= len(raw.Tracks) {
		return nil, fmt.Errorf("decode gp json: track %d out of range (%d tracks)", idx, len(raw.Tracks))
	}
	t := raw.Tracks[idx]
	tab := &model.Tab{
		Title:    raw.Title,
		Artist:   raw.Artist,
		Tuning:   model.Tuning(t.Tuning),
		Metadata: map[string]string{"source": "guitar-pro", "track": t.Name},
	}
	for _, b := range t.Bars {
		bar := model.Bar{Number: b.Number, ColumnTicks: b.ColumnTicks}
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
	return tab, nil
}

// gpTrackMeta is one entry of the tab's metadata["tracks"] payload: the
// per-track info the viewer's in-viewer track switcher renders (Wave-2
// cross-agent contract, serialized with encoding/json).
type gpTrackMeta struct {
	Name       string `json:"name"`
	Instrument string `json:"instrument"`
	Strings    int    `json:"strings"`
	Tuning     string `json:"tuning"`
}

// trackMetas renders the full track list of a GP file (including the
// imported track, so the switcher can switch back).
func trackMetas(tracks []parser.GPTrack) []gpTrackMeta {
	out := make([]gpTrackMeta, 0, len(tracks))
	for _, t := range tracks {
		out = append(out, gpTrackMeta{Name: t.Name, Instrument: t.Instrument, Strings: t.Strings, Tuning: t.Tuning})
	}
	return out
}
