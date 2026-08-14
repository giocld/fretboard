package parser

import (
	"testing"

	"fretboard/internal/model"
)

// TestExtractBarsZeroStringsPerColumn pins the behavior of the removed
// stringsPerColumn < 1 guard: a degenerate column count must yield no bars
// (nil), reached through the normal loop rather than an early return.
func TestExtractBarsZeroStringsPerColumn(t *testing.T) {
	region := []string{
		"e|--0--|--3--|",
		"B|------|-----|",
		"G|------|-----|",
		"D|------|-----|",
		"A|------|-----|",
		"E|------|-----|",
	}
	if got := extractBars(region, 0); got != nil {
		t.Fatalf("extractBars with stringsPerColumn=0 = %d bars, want nil", len(got))
	}
}

// TestBarsFromColumnMultiBarStringCount pins that every bar in a multi-bar
// column carries one StringLine per input line, matching the reverseAndParse
// output that replaced the old pre-sized make().
func TestBarsFromColumnMultiBarStringCount(t *testing.T) {
	col := []string{
		"e|--0--|--3--|",
		"B|------|-----|",
		"G|------|-----|",
		"D|------|-----|",
		"A|------|-----|",
		"E|------|-----|",
	}
	bars := barsFromColumn(col, 1, nil)
	if len(bars) != 2 {
		t.Fatalf("expected 2 bars, got %d", len(bars))
	}
	for i, b := range bars {
		if len(b.Strings) != len(col) {
			t.Fatalf("bar %d has %d strings, want %d", i, len(b.Strings), len(col))
		}
	}
}

// TestLeadingEndingIndexEmpty pins that an empty content slice means "no
// ending marker" (-1), which the trailing checks also produce directly.
func TestLeadingEndingIndexEmpty(t *testing.T) {
	if got := leadingEndingIndex(""); got != -1 {
		t.Fatalf("leadingEndingIndex(\"\") = %d, want -1", got)
	}
}

// TestRhythmForBarEmpty pins that an empty rhythm row produces a nil mark
// list, so bars without rhythm stay rhythm-free.
func TestRhythmForBarEmpty(t *testing.T) {
	if got := rhythmForBar(nil, 0, 10); got != nil {
		t.Fatalf("rhythmForBar(nil) = %+v, want nil", got)
	}
	if got := rhythmForBar([]model.RhythmMark{}, 0, 10); got != nil {
		t.Fatalf("rhythmForBar(empty) = %+v, want nil", got)
	}
}
