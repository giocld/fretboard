package parser

import (
	"strings"
	"testing"
)

func TestRepeatCountMarks(t *testing.T) {
	for in, want := range map[string]int{
		" (x12)": 12, "x4": 4, "X 3": 3, "4x": 4, "(2x)": 2,
		"": 0, "3x5---": 0, "then x4 more": 0, "PM---|": 0,
	} {
		if got := repeatCount(in); got != want {
			t.Errorf("repeatCount(%q) = %d, want %d", in, got, want)
		}
	}
}

// "(x12)" after a block's closing bar line, or "x2" on the line below it,
// plays the whole block that many times. A multi-bar line repeats as a
// line, not bar by bar.
func TestRepeatCountBlocks(t *testing.T) {
	tab, err := Parse(strings.NewReader(`e|-----------|
B|-----------|
G|-----------|
D|---5-------|
A|--7--------|
E|-0-----6-5-| (x12)

e|-----------|
B|-----------|
G|-----------|
D|-----------|
A|-2---------|
E|-0-0-0-0---|

e|-----|-----|
B|-----|-----|
G|-----|-----|
D|-----|-----|
A|-----|-----|
E|-0---|-3---|
x2
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(tab.Bars) != 4 {
		t.Fatalf("want 4 bars, got %d", len(tab.Bars))
	}
	if b := tab.Bars[0]; b.Times != 12 || b.TimesFrom != 0 {
		t.Fatalf("bar 1 should play 12 times from itself, got %d from %d", b.Times, b.TimesFrom)
	}
	if tab.Bars[1].Times != 0 || tab.Bars[2].Times != 0 {
		t.Fatalf("bars 2-3 carry no count: %d %d", tab.Bars[1].Times, tab.Bars[2].Times)
	}
	if b := tab.Bars[3]; b.Times != 2 || b.TimesFrom != 2 {
		t.Fatalf("x2 under a two-bar line repeats bars 3-4: got %d from %d", b.Times, b.TimesFrom)
	}
}
