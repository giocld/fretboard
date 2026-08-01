package parser

import (
	"strings"
	"testing"
)

func TestLooksLikeRhythmLine(t *testing.T) {
	if !looksLikeRhythmLine("| q  e  e  q  |") {
		t.Fatal("expected rhythm line")
	}
	if looksLikeRhythmLine("e|----0-----3-----|") {
		t.Fatal("string line should not match rhythm")
	}
}

func TestParseRhythmLine(t *testing.T) {
	marks := parseRhythmLine("| q  e  |")
	if len(marks) != 2 {
		t.Fatalf("expected 2 marks, got %d", len(marks))
	}
	if marks[0].Ticks != 480 || marks[1].Ticks != 240 {
		t.Fatalf("unexpected ticks: %+v", marks)
	}
}

func TestExtractBarsWithRhythmRow(t *testing.T) {
	tab, err := Parse(strings.NewReader(`Rhythm Tab
Tuning: E Standard

| q  e  e  q  |
E|----0-----3-----5---|
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(tab.Bars) == 0 {
		t.Fatal("no bars")
	}
	if len(tab.Bars[0].Rhythm) == 0 {
		t.Fatalf("expected rhythm marks, bar=%+v", tab.Bars[0])
	}
}
