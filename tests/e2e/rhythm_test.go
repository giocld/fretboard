package e2e_test

import (
	"strings"
	"testing"

	"github.com/YOUR_USERNAME/fretboard/internal/model"
	"github.com/YOUR_USERNAME/fretboard/internal/parser"
	"github.com/YOUR_USERNAME/fretboard/internal/player"
)

func TestRhythmAwareEvents(t *testing.T) {
	tab, err := parser.Parse(strings.NewReader(`Rhythm Tab
Tuning: E Standard

| q  e  e  q  |
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

	var firstOn, firstOff, secondOn int64 = -1, -1, -1
	for _, e := range evts {
		if e.Type == player.NoteOn && firstOn < 0 {
			firstOn = e.Tick
		}
		if e.Type == player.NoteOff && firstOff < 0 && e.Tick > firstOn {
			firstOff = e.Tick
		}
		if e.Type == player.NoteOn && firstOff >= 0 && secondOn < 0 && e.Tick > firstOn {
			secondOn = e.Tick
			break
		}
	}
	if firstOff-firstOn <= 0 || secondOn-firstOff <= 0 {
		t.Fatalf("could not measure durations: on=%d off=%d next=%d", firstOn, firstOff, secondOn)
	}
	if firstOff-firstOn <= secondOn-firstOff {
		t.Fatalf("quarter should last longer than eighth: q=%d e=%d", firstOff-firstOn, secondOn-firstOff)
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
