package parser

import (
	"strings"
	"testing"

	"fretboard/internal/player"
)

func TestLooksLikeRhythmLine(t *testing.T) {
	if !looksLikeRhythmLine("| q  e  e  q  |") {
		t.Fatal("expected rhythm line")
	}
	if !looksLikeRhythmLine("| h |") {
		t.Fatal("single-mark rhythm row must be recognized")
	}
	if looksLikeRhythmLine("e|----0-----3-----|") {
		t.Fatal("string line should not match rhythm")
	}
	if looksLikeRhythmLine("e|--|") {
		t.Fatal("short string line with label must not match rhythm")
	}
}

// TestSingleMarkRhythmRowParses guards against slow-ballad rows like "| h |"
// being dropped entirely (they used to fail the >= 2 letters check and the
// row was even absorbed as a string line).
func TestSingleMarkRhythmRowParses(t *testing.T) {
	tab, err := Parse(strings.NewReader(`Slow
Tuning: E Standard

| h |
E|--0-|
B|----|
G|----|
D|----|
A|----|
E|----|
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(tab.Bars) != 1 {
		t.Fatalf("expected 1 bar, got %d", len(tab.Bars))
	}
	if len(tab.Bars[0].Rhythm) != 1 || tab.Bars[0].Rhythm[0].Ticks != 960 {
		t.Fatalf("expected a half-note rhythm mark, got %+v", tab.Bars[0].Rhythm)
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

// TestMultiBarRhythmRowRebasesMarksPerBar guards against rhythm rows that span
// several bars in one chunk: each bar must receive only the marks inside its
// own pipe-delimited range, rebased to bar-relative positions. Attaching the
// chunk-wide positions verbatim to every bar times bars after the first with
// the first bar's rhythm.
func TestMultiBarRhythmRowRebasesMarksPerBar(t *testing.T) {
	tab, err := Parse(strings.NewReader(`Two Bars
Tuning: E Standard

|   q  e |  h  q |
E|--0--3-|--0--2-|
B|-------|-------|
G|-------|-------|
D|-------|-------|
A|-------|-------|
E|-------|-------|
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(tab.Bars) != 2 {
		t.Fatalf("expected 2 bars, got %d", len(tab.Bars))
	}
	bar1 := tab.Bars[0]
	if len(bar1.Rhythm) != 2 {
		t.Fatalf("bar 1: expected 2 marks, got %+v", bar1.Rhythm)
	}
	if bar1.Rhythm[0].Position != 2 || bar1.Rhythm[0].Ticks != 480 ||
		bar1.Rhythm[1].Position != 5 || bar1.Rhythm[1].Ticks != 240 {
		t.Fatalf("bar 1 rhythm wrong: %+v", bar1.Rhythm)
	}
	bar2 := tab.Bars[1]
	if len(bar2.Rhythm) != 2 {
		t.Fatalf("bar 2: expected 2 marks, got %+v", bar2.Rhythm)
	}
	if bar2.Rhythm[0].Position != 2 || bar2.Rhythm[0].Ticks != 960 ||
		bar2.Rhythm[1].Position != 5 || bar2.Rhythm[1].Ticks != 480 {
		t.Fatalf("bar 2 rhythm wrong (must be rebased to bar 2's own marks): %+v", bar2.Rhythm)
	}
}

// TestMultiBarRhythmRowTimesBar2ByItsOwnMarks proves the fix end-to-end: the
// second bar's notes must take the h/q durations (960/480), not the first
// bar's q/e (480/240) that used to be attached to every bar.
func TestMultiBarRhythmRowTimesBar2ByItsOwnMarks(t *testing.T) {
	tab, err := Parse(strings.NewReader(`Two Bars
Tuning: E Standard

|   q  e |  h  q |
E|--0--3-|--0--2-|
B|-------|-------|
G|-------|-------|
D|-------|-------|
A|-------|-------|
E|-------|-------|
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	steps := player.BuildSchedule(tab)
	var bar1Ticks, bar2Ticks []int
	for _, s := range steps {
		if s.Bar == 0 {
			bar1Ticks = append(bar1Ticks, s.Ticks)
		}
		if s.Bar == 1 {
			bar2Ticks = append(bar2Ticks, s.Ticks)
		}
	}
	if len(bar1Ticks) != 2 || bar1Ticks[0] != 480 || bar1Ticks[1] != 240 {
		t.Fatalf("bar 1 ticks wrong: %v", bar1Ticks)
	}
	if len(bar2Ticks) != 2 || bar2Ticks[0] != 960 || bar2Ticks[1] != 480 {
		t.Fatalf("bar 2 ticks wrong (should be h=960, q=480): %v", bar2Ticks)
	}
}
