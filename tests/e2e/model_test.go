package e2e_test

import (
	"testing"

	"github.com/YOUR_USERNAME/fretboard/internal/model"
)

func TestTuningE2E(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		openMIDI []int
	}{
		{
			name:     "E Standard",
			input:    "EADGBE",
			openMIDI: []int{40, 45, 50, 55, 59, 64},
		},
		{
			name:     "Drop D",
			input:    "DADGBE",
			openMIDI: []int{38, 45, 50, 55, 59, 64},
		},
		{
			name:     "DADGAD",
			input:    "DADGAD",
			openMIDI: []int{38, 45, 50, 55, 57, 62},
		},
		{
			name:     "7-string standard",
			input:    "BEADGBE",
			openMIDI: []int{35, 40, 45, 50, 55, 59, 64},
		},
		{
			name:     "Open G",
			input:    "DGDGBD",
			openMIDI: []int{38, 43, 50, 55, 59, 62},
		},
		{
			name:     "Eb Standard",
			input:    "EbAbDbGbBbEb",
			openMIDI: []int{39, 44, 49, 54, 58, 63},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tn := model.ParseTuning(tc.input)
			if len(tn) != len(tc.openMIDI) {
				t.Fatalf("expected %d strings, got %d (%v)", len(tc.openMIDI), len(tn), tn)
			}
			for i, want := range tc.openMIDI {
				if tn[i] != want {
					t.Errorf("string %d: got MIDI %d, want %d", i, tn[i], want)
				}
			}
		})
	}
}

func TestSemitoneMath(t *testing.T) {
	tn := model.Standard

	if got := tn.Semitone(0, 0); got != 40 {
		t.Errorf("open low E should be MIDI 40, got %d", got)
	}
	if got := tn.Semitone(0, 12); got != 52 {
		t.Errorf("12th fret low E should be E3 (MIDI 52), got %d", got)
	}
	if got := tn.Semitone(1, 7); got != 52 {
		t.Errorf("7th fret A should also be E3 (MIDI 52), got %d", got)
	}
	if got := tn.Semitone(5, 0); got != 64 {
		t.Errorf("open high e should be MIDI 64, got %d", got)
	}
	if got := tn.Semitone(5, 12); got != 76 {
		t.Errorf("12th fret high e should be E5 (MIDI 76), got %d", got)
	}
}

func TestNoteNameRendering(t *testing.T) {
	tn := model.Standard
	want := []string{"E2", "A2", "D3", "G3", "B3", "E4"}
	for i, w := range want {
		if got := tn.NoteName(i); got != w {
			t.Errorf("string %d: got %q, want %q", i, got, w)
		}
	}
	if got := tn.Label(); got != "EADGBE" {
		t.Errorf("Label() = %q, want %q", got, "EADGBE")
	}
}

func TestTabStructure(t *testing.T) {
	tab := &model.Tab{
		Title:  "Test",
		Artist: "Fretboard Test",
		Tuning: model.Standard,
		Bars: []model.Bar{
			{
				Number: 1,
				Strings: []model.StringLine{
					{Segments: []model.Segment{{Char: '0', Value: 0, Position: 0}}},
					{Segments: []model.Segment{{Char: '2', Value: 2, Position: 0}}},
					{Segments: []model.Segment{{Char: '2', Value: 2, Position: 0}}},
					{Segments: []model.Segment{{Char: '0', Value: 0, Position: 0}}},
					{Segments: []model.Segment{{Char: '0', Value: 0, Position: 0}}},
					{Segments: []model.Segment{{Char: '0', Value: 0, Position: 0}}},
				},
			},
		},
	}

	if tab.Title != "Test" {
		t.Errorf("title lost: %q", tab.Title)
	}
	if len(tab.Bars) != 1 {
		t.Fatalf("expected 1 bar, got %d", len(tab.Bars))
	}
	if got := tab.Tuning.Strings(); got != 6 {
		t.Fatalf("expected 6 strings, got %d", got)
	}
	if got := tab.Tuning.Semitone(2, 2); got != 52 {
		t.Errorf("D string fret 2 should be MIDI 52, got %d", got)
	}
}
