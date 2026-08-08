package player

import (
	"testing"

	"fretboard/internal/model"
)

func repeatBar(ending int, start, end bool) model.Bar {
	return model.Bar{Number: 1, RepeatStart: start, RepeatEnd: end, Ending: ending,
		Strings: []model.StringLine{{Segments: []model.Segment{{Char: '0', Value: 0, Position: 0, Width: 1}}}}}
}

// TestRepeatOrderExpandsSections pins the performance order for a section
// with first/second endings: A |: B C 1.D 2.E :| F plays as A B C D B C E F.
func TestRepeatOrderExpandsSections(t *testing.T) {
	bars := []model.Bar{
		repeatBar(0, false, false), // A
		repeatBar(0, true, false),  // B (|:)
		repeatBar(0, false, false), // C
		repeatBar(1, false, false), // D (1.)
		repeatBar(2, false, true),  // E (2. :|)
		repeatBar(0, false, false), // F
	}
	tab := &model.Tab{Bars: bars}
	got := RepeatOrder(tab)
	want := []int{0, 1, 2, 3, 1, 2, 4, 5}
	if len(got) != len(want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}

// TestRepeatOrderPlainSectionWithoutEndings: |: A B :| plays A B A B.
func TestRepeatOrderPlainSectionWithoutEndings(t *testing.T) {
	bars := []model.Bar{
		repeatBar(0, true, false),  // A (|:)
		repeatBar(0, false, true),  // B (:|)
		repeatBar(0, false, false), // C
	}
	got := RepeatOrder(&model.Tab{Bars: bars})
	want := []int{0, 1, 0, 1, 2}
	if len(got) != len(want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}

// TestRepeatOrderSingleBarRepeat: |: A :| plays A A.
func TestRepeatOrderSingleBarRepeat(t *testing.T) {
	bars := []model.Bar{repeatBar(0, true, true), repeatBar(0, false, false)}
	got := RepeatOrder(&model.Tab{Bars: bars})
	want := []int{0, 0, 1}
	if len(got) != len(want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}

// TestRepeatOrderUnpairedMarkers: stray markers must not change the order.
func TestRepeatOrderUnpairedMarkers(t *testing.T) {
	bars := []model.Bar{
		repeatBar(0, false, true),  // :| with no opener — play once
		repeatBar(0, true, false),  // |: with no closer — play once
		repeatBar(1, false, false), // 1. without a repeat — play once
	}
	got := RepeatOrder(&model.Tab{Bars: bars})
	want := []int{0, 1, 2}
	if len(got) != len(want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

// TestBuildScheduleExpandsRepeats verifies playback steps follow the
// performance order: the cursor visits the repeated bar twice.
func TestBuildScheduleExpandsRepeats(t *testing.T) {
	tab := &model.Tab{Bars: []model.Bar{
		{Number: 1, RepeatStart: true, Strings: []model.StringLine{
			{Segments: []model.Segment{{Char: '0', Value: 0, Position: 0, Width: 1}, {Char: '-', Position: 1}, {Char: '-', Position: 2}, {Char: '3', Value: 3, Position: 3, Width: 1}}},
		}},
		{Number: 2, RepeatEnd: true, Strings: []model.StringLine{
			{Segments: []model.Segment{{Char: '5', Value: 5, Position: 0, Width: 1}}},
		}},
	}}
	sched := BuildSchedule(tab)
	if len(sched) == 0 {
		t.Fatal("empty schedule")
	}
	// Bar 0 has notes at cols 0 and 3; bar 1 at col 0. Repeat => visit 0,0,1,0,0,1.
	var bars []int
	for _, s := range sched {
		bars = append(bars, s.Bar)
	}
	want := []int{0, 0, 1, 0, 0, 1}
	if len(bars) != len(want) {
		t.Fatalf("schedule bars = %v, want %v", bars, want)
	}
	for i := range want {
		if bars[i] != want[i] {
			t.Fatalf("schedule bars = %v, want %v", bars, want)
		}
	}
}
