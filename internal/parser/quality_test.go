package parser

import (
	"fmt"
	"strings"
	"testing"

	"fretboard/internal/model"
)

// wellAlignedTab is a clean 6-string tab: three header lines plus six rows
// with a uniform bar grid (identical pipe columns) and dash runs of at
// least two columns. Expected: RowRatio 6/9, dash and align 1.0, solid.
func wellAlignedTab() []string {
	return []string{
		"Song Title",
		"Artist",
		"Tuning: EADGBE",
		"e|----0----0----|",
		"B|----1----1----|",
		"G|----2----2----|",
		"D|----2----2----|",
		"A|----0----0----|",
		"E|----3----3----|",
	}
}

// compactTab is a real but hand-tight tab: pipes align across rows, but
// notes are separated by single dashes, which the dash metric treats as
// misaligned spacing. Expected: row and align 1.0, dash 0, approximate.
func compactTab() []string {
	return []string{
		"e|-0-2-3-0-2-|",
		"B|-0-2-3-0-2-|",
		"G|-0-2-3-0-2-|",
		"D|-0-2-3-0-2-|",
		"A|-0-2-3-0-2-|",
		"E|-0-2-3-0-2-|",
	}
}

// prose is a non-tab text body (lyrics): nothing matches a string row, no
// pipes, no in-bar dashes. Expected: all metrics 0, sloppy.
func prose() []string {
	return []string{
		"We don't need no education",
		"We don't need no thought control",
		"No dark sarcasm in the classroom",
		"Teachers leave them kids alone",
	}
}

func TestScoreTabSolid(t *testing.T) {
	q := ScoreTab(wellAlignedTab())
	if q.TimingWord != TimingSolid {
		t.Fatalf("TimingWord = %q, want %q (row=%.3f dash=%.3f align=%.3f)",
			q.TimingWord, TimingSolid, q.RowRatio, q.DashConsistency, q.BarAlign)
	}
	if q.RowRatio != 2.0/3.0 {
		t.Errorf("RowRatio = %.3f, want 0.667 (6 rows of 9 non-blank lines)", q.RowRatio)
	}
	if q.DashConsistency != 1.0 {
		t.Errorf("DashConsistency = %.3f, want 1.000", q.DashConsistency)
	}
	if q.BarAlign != 1.0 {
		t.Errorf("BarAlign = %.3f, want 1.000", q.BarAlign)
	}
	if c := compositeScore(q); c < solidThreshold {
		t.Errorf("composite %.3f below solid threshold %.2f", c, solidThreshold)
	}
}

func TestScoreTabSloppy(t *testing.T) {
	q := ScoreTab(prose())
	if q.TimingWord != TimingSloppy {
		t.Fatalf("TimingWord = %q, want %q", q.TimingWord, TimingSloppy)
	}
	if q.RowRatio != 0 || q.DashConsistency != 0 || q.BarAlign != 0 {
		t.Errorf("row=%.3f dash=%.3f align=%.3f, want all 0 for prose",
			q.RowRatio, q.DashConsistency, q.BarAlign)
	}
	if c := compositeScore(q); c > sloppyThreshold {
		t.Errorf("composite %.3f above sloppy threshold %.2f", c, sloppyThreshold)
	}
}

func TestScoreTabApproximate(t *testing.T) {
	q := ScoreTab(compactTab())
	if q.TimingWord != TimingApproximate {
		t.Fatalf("TimingWord = %q, want %q (row=%.3f dash=%.3f align=%.3f)",
			q.TimingWord, TimingApproximate, q.RowRatio, q.DashConsistency, q.BarAlign)
	}
	if q.RowRatio != 1.0 {
		t.Errorf("RowRatio = %.3f, want 1.000", q.RowRatio)
	}
	if q.DashConsistency != 0 {
		t.Errorf("DashConsistency = %.3f, want 0 (single-dash runs)", q.DashConsistency)
	}
	if q.BarAlign != 1.0 {
		t.Errorf("BarAlign = %.3f, want 1.000", q.BarAlign)
	}
	if c := compositeScore(q); c <= sloppyThreshold || c >= solidThreshold {
		t.Errorf("composite %.3f outside approximate band (%.2f, %.2f)",
			c, sloppyThreshold, solidThreshold)
	}
}

func TestScoreTabEmpty(t *testing.T) {
	for name, lines := range map[string][]string{
		"nil":   nil,
		"empty": {},
		"blank": {"", "  ", "\t"},
	} {
		q := ScoreTab(lines)
		if q.TimingWord != TimingSloppy {
			t.Errorf("%s: TimingWord = %q, want sloppy", name, q.TimingWord)
		}
		if c := compositeScore(q); c != 0 {
			t.Errorf("%s: composite = %.3f, want 0", name, c)
		}
	}
}

func TestScoreTabIgnoresBlankLines(t *testing.T) {
	lines := append(append([]string{}, wellAlignedTab()...), "", "   ")
	got, want := ScoreTab(lines), ScoreTab(wellAlignedTab())
	if got != want {
		t.Errorf("blank lines changed the score: %+v vs %+v", got, want)
	}
}

func TestApplyQualityWritesMetadata(t *testing.T) {
	tab := &model.Tab{Metadata: map[string]string{}}
	q := QualityResult{TimingWord: TimingSolid, RowRatio: 0.75, DashConsistency: 1, BarAlign: 1}
	ApplyQuality(tab, q)

	want := map[string]string{
		"quality":           "0.92", // (0.75 + 1 + 1) / 3 = 0.9166...
		"quality_timing":    TimingSolid,
		"quality_row_ratio": "0.750",
		"quality_dash":      "1.000",
		"quality_bar_align": "1.000",
	}
	for k, v := range want {
		if got := tab.Metadata[k]; got != v {
			t.Errorf("Metadata[%q] = %q, want %q", k, got, v)
		}
	}
	if len(tab.Metadata) != len(want) {
		t.Errorf("Metadata has %d keys, want %d: %v", len(tab.Metadata), len(want), tab.Metadata)
	}
}

func TestApplyQualityNilSafe(t *testing.T) {
	// Nil tab must not panic; nil map must be initialized.
	ApplyQuality(nil, QualityResult{})
	tab := &model.Tab{}
	ApplyQuality(tab, QualityResult{TimingWord: TimingSloppy})
	if tab.Metadata["quality_timing"] != TimingSloppy {
		t.Errorf("quality_timing = %q, want %q", tab.Metadata["quality_timing"], TimingSloppy)
	}
	if _, ok := tab.Metadata["quality"]; !ok {
		t.Error("quality key missing after ApplyQuality on nil map")
	}
}

func TestDashConsistency(t *testing.T) {
	cases := []struct {
		name string
		rows []string
		want float64
	}{
		{"well padded", []string{"e|----0----|", "B|----------|"}, 1.0},
		{"single-dash gaps", []string{"e|-0-0-|"}, 0.0},
		{"mixed padding", []string{"e|--0-0-|"}, 1.0 / 3.0},
		{"runs outside bars ignored", []string{"e---|--0--|----"}, 1.0},
		{"no pipes", []string{"e------0----"}, 0.0},
		{"empty", nil, 0.0},
	}
	for _, tc := range cases {
		if got := dashConsistency(tc.rows); got != tc.want {
			t.Errorf("%s: dashConsistency = %.3f, want %.3f", tc.name, got, tc.want)
		}
	}
}

func TestBarAlign(t *testing.T) {
	cases := []struct {
		name string
		rows []string
		want float64
	}{
		{"aligned", []string{"e|----|", "B|----|", "G|----|"}, 1.0},
		// One row's closing pipe drifts one column: variance 0.25 over the
		// second boundary and 0 over the first, so the mean is 0.125.
		{"one-column drift", []string{"e|----|", "B|---|"}, 0.875},
		{"no pipes", []string{"e------", "B------"}, 0.0},
		{"single row", []string{"e|----|"}, 0.0},
		{"empty", nil, 0.0},
	}
	for _, tc := range cases {
		if got := barAlign(tc.rows); got != tc.want {
			t.Errorf("%s: barAlign = %.3f, want %.3f", tc.name, got, tc.want)
		}
	}
}

func TestScoreTabIgnoresRhythmAndHeaderLines(t *testing.T) {
	// Rhythm rows and non-tab content must not count as string rows nor
	// drag down dash/align: 2 tab rows + 3 other non-blank lines -> 0.4.
	lines := []string{
		"Title",
		"| q  e  | h  q  |", // rhythm row
		"e|--0--0--|",
		"B|--0--0--|",
		"Chorus:",
	}
	q := ScoreTab(lines)
	if q.RowRatio != 0.4 {
		t.Errorf("RowRatio = %.3f, want 0.400 (2 of 5 non-blank lines are rows)", q.RowRatio)
	}
	if q.DashConsistency != 1.0 || q.BarAlign != 1.0 {
		t.Errorf("rhythm/header lines leaked into metrics: dash=%.3f align=%.3f",
			q.DashConsistency, q.BarAlign)
	}
}

// TestScoreTabDeterministic guards against map-iteration nondeterminism in
// the scoring internals.
func TestScoreTabDeterministic(t *testing.T) {
	lines := compactTab()
	first := ScoreTab(lines)
	for i := 0; i < 20; i++ {
		if got := ScoreTab(lines); got != first {
			t.Fatalf("non-deterministic score on run %d: %+v vs %+v", i, got, first)
		}
	}
}

func TestParseAppliesQuality(t *testing.T) {
	// The Parse hook (parser.go) calls ScoreTab + ApplyQuality at parse
	// time, so a plain Parse of a tab must carry quality metadata.
	content := "Title\nArtist\n\ne|----0----0----|\nB|----1----1----|\nG|----2----2----|\nD|----2----2----|\nA|----0----0----|\nE|----3----3----|\n"
	tab, err := Parse(strings.NewReader(content))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if word := tab.Metadata["quality_timing"]; word != TimingSolid {
		t.Errorf("quality_timing = %q, want %q (well-aligned tab)", word, TimingSolid)
	}
	if _, ok := tab.Metadata["quality"]; !ok {
		t.Error("quality key missing after parse")
	}
	for _, k := range []string{"quality_row_ratio", "quality_dash", "quality_bar_align"} {
		if _, ok := tab.Metadata[k]; !ok {
			t.Errorf("%s missing after parse", k)
		}
	}
	// The stored composite must match a direct re-score of the lines.
	q := ScoreTab(strings.Split(content, "\n"))
	if got, want := tab.Metadata["quality"], fmt.Sprintf("%.2f", compositeScore(q)); got != want {
		t.Errorf("quality = %q, want %q", got, want)
	}
}
