package e2e_test

import (
	"os/exec"
	"strings"
	"testing"

	"fretboard/internal/parser"
	"fretboard/internal/player"
	"fretboard/tests/helpers"
)

func TestSynthPlayStop(t *testing.T) {
	// Skip if no external synthesizer is installed.
	hasSynth := false
	if _, err := exec.LookPath("fluidsynth"); err == nil {
		if player.ResolveSoundfont() != "" {
			hasSynth = true
		}
	}
	if !hasSynth {
		if _, err := exec.LookPath("timidity"); err != nil {
			t.Skip("no fluidsynth+soundfont or timidity installed")
		}
	}

	tab, err := parser.Parse(strings.NewReader(helpers.SmokeTab))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	synth := player.NewSynth()
	if err := synth.Play(tab, 120); err != nil {
		t.Fatalf("Play: %v", err)
	}
	if !synth.Running() {
		t.Fatalf("expected synth to be running")
	}
	if err := synth.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if synth.Running() {
		t.Fatalf("expected synth to be stopped")
	}
}

func TestSynthMissingDependency(t *testing.T) {
	// If neither synth is installed, Play should return a clear error.
	if _, err := exec.LookPath("fluidsynth"); err == nil {
		t.Skip("fluidsynth is installed")
	}
	if _, err := exec.LookPath("timidity"); err == nil {
		t.Skip("timidity is installed")
	}

	tab, err := parser.Parse(strings.NewReader(helpers.SmokeTab))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	synth := player.NewSynth()
	if err := synth.Play(tab, 120); err == nil {
		t.Fatalf("expected error when no synth is installed")
	}
}
