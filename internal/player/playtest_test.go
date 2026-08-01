package player_test

import (
	"strings"
	"testing"
	"time"

	"fretboard/internal/parser"
	"fretboard/internal/player"
)

func TestResolveSoundfontInstalled(t *testing.T) {
	sf := player.ResolveSoundfont()
	t.Logf("soundfont=%q", sf)
	if sf == "" {
		t.Fatal("expected soundfont on this system")
	}
}

func TestPlayWithInstalledSoundfont(t *testing.T) {
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
	if err := s.Play(tab, 120); err != nil {
		t.Fatalf("Play: %v", err)
	}
	if !s.Running() {
		t.Fatal("synth not running after Play")
	}
	time.Sleep(500 * time.Millisecond)
	if !s.Running() {
		t.Fatal("synth died within 500ms - audio backend likely failed")
	}
	_ = s.Stop()
}
