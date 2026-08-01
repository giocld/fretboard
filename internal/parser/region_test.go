package parser

import (
	"strings"
	"testing"
)

func TestFindTabRegionIncludesRhythm(t *testing.T) {
	lines := []string{
		"Rhythm Tab",
		"Tuning: E Standard",
		"",
		"| q  e  e  q  |",
		"E|----0-----3-----5---|",
	}
	start, end := findTabRegion(lines, 0)
	t.Logf("region=%d:%d lines=%q", start, end, lines[start:end])
	if start < 0 || end <= start {
		t.Fatal("no region")
	}
	if !looksLikeRhythmLine(lines[start]) {
		t.Fatalf("expected rhythm at start, got %q", lines[start])
	}
	_ = strings.TrimSpace
}
