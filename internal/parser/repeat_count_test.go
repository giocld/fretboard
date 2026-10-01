package parser

import (
	"strings"
	"testing"

	"fretboard/internal/model"
	"fretboard/internal/player"
)

func TestRepeatCountMarks(t *testing.T) {
	for in, want := range map[string]int{
		" (x12)": 12, "x4": 4, "X 3": 3, "4x": 4, "(2x)": 2, "x100": 100, "(x120)": 120,
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

// TestAbsurdRepeatCountIsClamped guards the schedule against remote content:
// a silly count mark must not explode the playback schedule (the parser
// clamps, and RepeatOrder re-checks the bound for hand-built tabs).
func TestAbsurdRepeatCountIsClamped(t *testing.T) {
	tab, err := Parse(strings.NewReader("E|-0-|\n(x999999)\n"))
	if err != nil {
		t.Fatal(err)
	}
	if b := tab.Bars[0]; b.Times > maxRepeatCount {
		t.Fatalf("count = %d, want ≤ %d", b.Times, maxRepeatCount)
	}
	// RepeatOrder's own bound holds even for hand-built tabs.
	hand := &model.Tab{
		Tuning: model.Standard,
		Bars: []model.Bar{{Strings: []model.StringLine{{Segments: []model.Segment{
			{Char: '0', Value: 0, Position: 0, Width: 1},
		}}}}},
	}
	hand.Bars[0].Times = 1 << 30
	hand.Bars[0].TimesFrom = 0
	order := player.RepeatOrder(hand)
	if len(order) != 1 {
		t.Fatalf("uncapped count expanded to %d bars, want 1", len(order))
	}
}
