package parser

import (
	"strings"
	"testing"
)

// Palm-mute rows under a block ("   PM-----|") are mostly dashes, but they
// are annotations, not strings. Read as a string they add a phantom 7th
// string, and blocks without the row then map every note one string low.
func TestPalmMuteRowsAreNotStrings(t *testing.T) {
	tab, err := Parse(strings.NewReader(`e|-----------|
B|-----------|
G|-----------|
D|---5-------|
A|--7--------|
E|-0-----6-5-|

e|-----------|
B|-----------|
G|-----------|
D|-----------|
A|-2---------|
E|-0-0-0-0---|
   PM--------|
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(tab.Tuning) != 6 {
		t.Fatalf("palm-mute rows must not add a string: tuning %v", tab.Tuning)
	}
	for i, b := range tab.Bars {
		if len(b.Strings) != 6 {
			t.Fatalf("bar %d has %d strings, want 6", i+1, len(b.Strings))
		}
	}
	// String 0 is the low E: open, it must be E2 (MIDI 40), not B1.
	if got := tab.Tuning.Semitone(0, 0); got != 40 {
		t.Fatalf("low string open = MIDI %d, want 40", got)
	}
	for _, line := range []string{"e|--0--|", "Eb|--0--|", "|--0--|", "--0--3--", "b|-3-|", "E-----0--", "e |--0--|", "HH|--x---x-|", "SD|x-------|"} {
		if !looksLikeStringLine(line) {
			t.Errorf("%q is a string line", line)
		}
	}
	for _, line := range []string{"   PM--------|", "let ring-------|", "P.M.---|", "   PM-|     PM---|", "PM----"} {
		if looksLikeStringLine(line) {
			t.Errorf("%q is an annotation, not a string", line)
		}
	}
}
