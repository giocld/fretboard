package parser

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"fretboard/internal/model"
)

// gpExtensions lists Guitar Pro file extensions supported via gp-parser.
var gpExtensions = []string{".gp3", ".gp4", ".gp5", ".gpx", ".gp"}

// IsGpFile reports whether path looks like a Guitar Pro file.
func IsGpFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	for _, e := range gpExtensions {
		if ext == e {
			return true
		}
	}
	return false
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
	tab := &model.Tab{
		Title:    raw.Title,
		Artist:   raw.Artist,
		Tuning:   model.Tuning(raw.Tuning),
		Metadata: raw.Metadata,
	}
	if tab.Metadata == nil {
		tab.Metadata = map[string]string{}
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
