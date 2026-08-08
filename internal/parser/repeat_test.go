package parser

import (
	"strings"
	"testing"
)

// TestParseRepeatMarkers verifies "|:" ":|" and 1./2. ending markers are
// captured per bar from a realistic repeated section.
func TestParseRepeatMarkers(t *testing.T) {
	src := `Title: Repeats
Tuning: E Standard

e|:--0--|--3--|1.--5--|2.--7--:||--9--|
B|------|------|-------|--------|-----|
G|------|------|-------|--------|-----|
D|------|------|-------|--------|-----|
A|------|------|-------|--------|-----|
E|------|------|-------|--------|-----|
`
	tab, err := Parse(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(tab.Bars) != 5 {
		t.Fatalf("expected 5 bars, got %d", len(tab.Bars))
	}
	b0, b2, b3 := tab.Bars[0], tab.Bars[2], tab.Bars[3]
	if !b0.RepeatStart {
		t.Fatal("bar 1 should open the repeat (|:)")
	}
	if b0.RepeatEnd || b0.Ending != 0 {
		t.Fatalf("bar 1 markers wrong: %+v", b0)
	}
	if b2.Ending != 1 {
		t.Fatalf("bar 3 should be a first ending (1.), got Ending=%d", b2.Ending)
	}
	if !b3.RepeatEnd || b3.Ending != 2 {
		t.Fatalf("bar 4 should close the repeat (:|) with a second ending, got %+v", b3)
	}
	if tab.Bars[4].RepeatStart || tab.Bars[4].RepeatEnd || tab.Bars[4].Ending != 0 {
		t.Fatalf("bar 5 should have no markers, got %+v", tab.Bars[4])
	}
}

// TestParseRepeatMarkersSingleBarRepeat covers "|:--:|" (one-bar repeat) and
// a repeat marker written on the top string line only.
func TestParseRepeatMarkersSingleBarRepeat(t *testing.T) {
	src := `Tuning: E Standard

e|:--0--:|
B|------|
G|------|
D|------|
A|------|
E|------|
`
	tab, err := Parse(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(tab.Bars) != 1 {
		t.Fatalf("expected 1 bar, got %d", len(tab.Bars))
	}
	if !tab.Bars[0].RepeatStart || !tab.Bars[0].RepeatEnd {
		t.Fatalf("single-bar repeat should open and close, got %+v", tab.Bars[0])
	}
}

// TestParseRepeatMarkersUntouched verifies tabs without repeats stay clean.
func TestParseRepeatMarkersUntouched(t *testing.T) {
	src := `Tuning: E Standard

e|--0--|--3--|
B|-----|-----|
G|-----|-----|
D|-----|-----|
A|-----|-----|
E|-----|-----|
`
	tab, err := Parse(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	for i, b := range tab.Bars {
		if b.RepeatStart || b.RepeatEnd || b.Ending != 0 {
			t.Fatalf("bar %d should have no markers, got %+v", i, b)
		}
	}
}
