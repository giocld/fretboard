package player

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"fretboard/internal/parser"
)

type recordingSynthIn struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (w *recordingSynthIn) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *recordingSynthIn) Close() error { return nil }

func (w *recordingSynthIn) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

func containsPitch(pitches []int, p int) bool {
	return slices.Contains(pitches, p)
}

func TestPlayStepSustainsNotesUntilStop(t *testing.T) {
	tab, err := parser.Parse(strings.NewReader("Tuning: E Standard\n\ne|0-3-5|\n"))
	if err != nil {
		t.Fatal(err)
	}
	steps := BuildSchedule(tab)
	if len(steps) < 2 {
		t.Fatal("expected at least 2 steps")
	}
	in := &recordingSynthIn{}
	s := NewSynth()
	s.realtime = true
	s.stdin = in

	first, err := NotesAtStep(tab, steps[0])
	if err != nil || len(first) == 0 {
		t.Fatalf("notes at first step: %v", err)
	}
	if err := s.PlayStep(tab, steps[0], 120); err != nil {
		t.Fatalf("PlayStep(0): %v", err)
	}
	for _, n := range first {
		if !containsPitch(s.activeNotes, n.Note) {
			t.Fatalf("note %d not active after its own step", n.Note)
		}
	}
	if err := s.PlayStep(tab, steps[1], 120); err != nil {
		t.Fatalf("PlayStep(1): %v", err)
	}
	for _, n := range first {
		if !containsPitch(s.activeNotes, n.Note) {
			t.Fatalf("note %d was cut at the next step boundary; activeNotes=%v", n.Note, s.activeNotes)
		}
	}
	if err := s.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if len(s.activeNotes) != 0 {
		t.Fatalf("Stop should release all notes, activeNotes=%v", s.activeNotes)
	}
	wantOff := fmt.Sprintf("noteoff 0 %d", first[0].Note)
	if !strings.Contains(in.String(), wantOff) {
		t.Fatalf("Stop did not send noteoff for released note %s; wrote: %q", wantOff, in.String())
	}
}
