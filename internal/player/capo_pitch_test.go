package player

import (
	"strings"
	"testing"

	"fretboard/internal/parser"
)

// Tabs write frets relative to the capo: fret 0 with a capo on 3 sounds
// three semitones above the open string. Both playback paths must agree.
func TestCapoRaisesPitch(t *testing.T) {
	tab, err := parser.Parse(strings.NewReader("Capo: 3\n\ne|-0--|\nB|----|\nG|----|\nD|----|\nA|----|\nE|----|\n"))
	if err != nil {
		t.Fatal(err)
	}
	notes, _ := NotesAtStep(tab, BuildSchedule(tab)[0])
	if len(notes) != 1 || notes[0].Note != 67 {
		t.Fatalf("open high e with capo 3 = %+v, want MIDI 67 (G4)", notes)
	}
	evts, err := Events(tab, 120)
	if err != nil || len(evts) == 0 || evts[0].Note != 67 {
		t.Fatalf("MIDI file path: first event %+v (err %v), want MIDI 67", evts, err)
	}
}
