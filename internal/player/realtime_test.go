package player_test

import (
	"strings"
	"testing"
	"time"

	"github.com/YOUR_USERNAME/fretboard/internal/parser"
	"github.com/YOUR_USERNAME/fretboard/internal/player"
)

func TestNotesAtStep(t *testing.T) {
	tab, err := parser.Parse(strings.NewReader("Tuning: E Standard\n\ne|0-3-5|\n"))
	if err != nil {
		t.Fatal(err)
	}
	steps := player.BuildSchedule(tab)
	if len(steps) == 0 {
		t.Fatal("no steps")
	}
	notes, err := player.NotesAtStep(tab, steps[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) == 0 {
		t.Fatal("expected notes at first step")
	}
}

func TestRealtimeSynthPlaysStep(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	sf := player.ResolveSoundfont()
	if sf == "" {
		t.Skip("no soundfont")
	}
	tab, err := parser.Parse(strings.NewReader("Tuning: E Standard\n\ne|0-3-5|\n"))
	if err != nil {
		t.Fatal(err)
	}
	s := player.NewSynth()
	s.Soundfont = sf
	if err := s.StartRealtime(); err != nil {
		t.Fatalf("StartRealtime: %v", err)
	}
	steps := player.BuildSchedule(tab)
	if err := s.PlayStep(tab, steps[0], 120); err != nil {
		t.Fatalf("PlayStep: %v", err)
	}
	time.Sleep(300 * time.Millisecond)
	if !s.Running() {
		t.Fatal("realtime synth died")
	}
	_ = s.Stop()
}
