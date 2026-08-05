package e2e_test

import (
	"strings"
	"testing"

	"fretboard/internal/model"
	"fretboard/internal/parser"
	"fretboard/internal/player"
)

func TestRhythmAwareEvents(t *testing.T) {
	tab, err := parser.Parse(strings.NewReader(`Rhythm Tab
Tuning: E Standard

|    q      e      q  |
E|----0-----3-----5---|
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(tab.Bars) == 0 || len(tab.Bars[0].Rhythm) == 0 {
		t.Fatal("expected rhythm marks on bar")
	}

	evts, err := player.Events(tab, 120)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	if len(evts) < 4 {
		t.Fatalf("expected note events, got %d", len(evts))
	}

	// The q/e/q marks sit above the notes; the first note is a quarter (480
	// ticks), the second an eighth (240).
	var durations []int64
	var onTick int64 = -1
	for _, e := range evts {
		if e.Type == player.NoteOn {
			onTick = e.Tick
		}
		if e.Type == player.NoteOff && e.Tick > onTick {
			durations = append(durations, e.Tick-onTick)
			onTick = e.Tick
		}
	}
	if len(durations) < 2 {
		t.Fatalf("could not measure durations: %v", durations)
	}
	if durations[0] != 480 || durations[1] != 240 {
		t.Fatalf("unexpected durations: %v (want 480, 240)", durations)
	}
	if durations[0] <= durations[1] {
		t.Fatalf("quarter should last longer than eighth: q=%d e=%d", durations[0], durations[1])
	}
}

func TestBuildScheduleUsesColumnTicks(t *testing.T) {
	tab := &model.Tab{
		Tuning: model.Standard,
		Bars: []model.Bar{{
			Strings: []model.StringLine{{
				Segments: []model.Segment{
					{Char: '0', Value: 0, Position: 0, Width: 1},
					{Char: '3', Value: 3, Position: 4, Width: 1},
				},
			}},
			ColumnTicks: []int{960, 0, 0, 0, 240},
		}},
	}
	steps := player.BuildSchedule(tab)
	if len(steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(steps))
	}
	if steps[0].Ticks != 960 || steps[1].Ticks != 240 {
		t.Fatalf("unexpected ticks: %+v", steps)
	}
}
