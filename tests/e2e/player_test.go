package e2e_test

import (
	"strings"
	"testing"

	"github.com/YOUR_USERNAME/fretboard/internal/parser"
	"github.com/YOUR_USERNAME/fretboard/internal/player"
	"github.com/YOUR_USERNAME/fretboard/tests/helpers"
)

func TestEventsE2E(t *testing.T) {
	tab, err := parser.Parse(strings.NewReader(helpers.SmokeTab))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(tab.Bars) == 0 || len(tab.Bars[0].Strings) == 0 {
		t.Fatalf("expected at least one bar with strings")
	}
	if tab.Tuning[0] != 38 {
		t.Fatalf("expected Drop D low string MIDI 38, got %d", tab.Tuning[0])
	}

	evts, err := player.Events(tab, 120)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	if len(evts) == 0 {
		t.Fatalf("expected events, got none")
	}

	wantMIDI := map[int]bool{38: true, 45: true, 50: true}
	foundMIDI := make(map[int]bool)
	for _, e := range evts {
		if e.Type == player.NoteOn {
			foundMIDI[e.Note] = true
		}
	}
	for midi := range wantMIDI {
		if !foundMIDI[midi] {
			t.Errorf("expected MIDI note %d in events, got notes %v", midi, foundMIDI)
		}
	}

	for i := 1; i < len(evts); i++ {
		if evts[i].Tick < evts[i-1].Tick {
			t.Fatalf("events out of order at index %d: %d < %d", i, evts[i].Tick, evts[i-1].Tick)
		}
	}

	foundFret3 := false
	for _, e := range evts {
		if e.Type == player.NoteOn && e.Note == 41 && e.String == 0 {
			foundFret3 = true
			break
		}
	}
	if !foundFret3 {
		t.Errorf("expected a fret-3 note on the low D string to be MIDI 41, got notes %v", foundMIDI)
	}
}

func TestEventsRejectsNil(t *testing.T) {
	_, err := player.Events(nil, 120)
	if err == nil {
		t.Fatalf("expected error for nil tab")
	}
}

func TestEventsMonophonicLine(t *testing.T) {
	tab, err := parser.Parse(strings.NewReader(helpers.MonoTab))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	evts, err := player.Events(tab, 120)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	var notes []int
	for _, e := range evts {
		if e.Type == player.NoteOn {
			notes = append(notes, e.Note)
		}
	}
	want := []int{40, 43, 45}
	if len(notes) != len(want) {
		t.Fatalf("expected %v, got %v", want, notes)
	}
	for i, w := range want {
		if notes[i] != w {
			t.Errorf("note %d: got %d, want %d", i, notes[i], w)
		}
	}
}

func TestWriteSMF(t *testing.T) {
	tab, err := parser.Parse(strings.NewReader(helpers.SmokeTab))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	evts, err := player.Events(tab, 120)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	data, err := player.WriteSMF(evts, 120)
	if err != nil {
		t.Fatalf("WriteSMF: %v", err)
	}
	if len(data) < 14 {
		t.Fatalf("SMF too short: %d bytes", len(data))
	}
	if string(data[:4]) != "MThd" {
		t.Fatalf("expected SMF header MThd, got %q", string(data[:4]))
	}
	if string(data[14:18]) != "MTrk" {
		t.Fatalf("expected track header MTrk, got %q", string(data[14:18]))
	}
}

func TestCursorAdvances(t *testing.T) {
	c := &player.Cursor{Bar: 0, Col: 0, Playing: true}
	c.Advance(4)
	if c.Bar != 0 || c.Col != 1 {
		t.Fatalf("advance 1 col in a 4-col bar: got Bar=%d Col=%d", c.Bar, c.Col)
	}
	c.Advance(4)
	c.Advance(4)
	c.Advance(4)
	if c.Bar != 1 || c.Col != 0 {
		t.Fatalf("wrapping bar after 4 advances: got Bar=%d Col=%d", c.Bar, c.Col)
	}
	c.Advance(4)
	c.Advance(4)
	c.Advance(4)
	c.Advance(4)
	if c.Bar != 2 || c.Col != 0 {
		t.Fatalf("wrapping bar after 8 advances: got Bar=%d Col=%d", c.Bar, c.Col)
	}
	c.Reset()
	if c.Bar != 0 || c.Col != 0 {
		t.Fatalf("reset failed: got Bar=%d Col=%d", c.Bar, c.Col)
	}
}
