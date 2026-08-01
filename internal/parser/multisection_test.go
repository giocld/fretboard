package parser

import (
	"strings"
	"testing"
)

func TestFindTabRegionMultiSection(t *testing.T) {
	lines := []string{
		"Layla", "Eric Clapton", "Tuning: EADGBE", "",
		"e|----0-----|", "B|----------|", "G|----------|", "D|----------|", "A|----------|", "E|----------|", "",
		"e|----3-----|", "B|----------|", "G|----------|", "D|----------|", "A|----------|", "E|----------|",
	}
	start, end := findTabRegion(lines, 3)
	if start < 0 || end <= start {
		t.Fatalf("no region: %d %d", start, end)
	}
	if len(lines[start:end]) < 12 {
		t.Fatalf("expected both tab sections in region, got %d lines", len(lines[start:end]))
	}
}

func TestParseMultiSectionUGTab(t *testing.T) {
	content := "Layla\nEric Clapton\nTuning: EADGBE\n\ne|----0-----|\nB|----------|\nG|----------|\nD|----------|\nA|----------|\nE|----------|\n\ne|----3-----|\nB|----------|\nG|----------|\nD|----------|\nA|----------|\nE|----------|\n\ne|----5-----|\nB|----------|\nG|----------|\nD|----------|\nA|----------|\nE|----------|\n"
	tab, err := Parse(strings.NewReader(content))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(tab.Bars) < 3 {
		t.Fatalf("expected at least 3 bars, got %d", len(tab.Bars))
	}
}
