package parser

import (
	"strings"
	"testing"
)

// TestParseVariableLengthLinesDoesNotPanic reproduces the UG-fetch panic:
// a bar whose later string lines are shorter than the pipe positions taken
// from the top line must parse cleanly (repeatMarkers used to slice
// line[start:end] out of range).
func TestParseVariableLengthLinesDoesNotPanic(t *testing.T) {
	src := `Tuning: E Standard

e|:--0--|--3--|
B|------|--|
G|------|------|
D|------|------|
A|------|------|
E|------|------|
`
	tab, err := Parse(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(tab.Bars) < 2 {
		t.Fatalf("expected at least 2 bars, got %d", len(tab.Bars))
	}
	if !tab.Bars[0].RepeatStart {
		t.Fatalf("bar 1 should open the repeat: %+v", tab.Bars[0])
	}
}
