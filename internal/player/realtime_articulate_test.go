package player

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"fretboard/internal/model"
)

// nopWriteCloser adapts a bytes.Buffer for use as the realtime stdin.
type nopWriteCloser struct {
	buf *bytes.Buffer
}

func (n *nopWriteCloser) Write(p []byte) (int, error) { return n.buf.Write(p) }
func (n *nopWriteCloser) Close() error                { return nil }

func TestStepDurationExactAndNeverZero(t *testing.T) {
	// A 1-tick sustain at 126 BPM is 0.99 ms: it must still be positive
	// (a 0 ms sustain skipped the noteoff, so the note rang forever).
	if got := StepDuration(1, 126); got <= 0 || got >= time.Millisecond {
		t.Fatalf("StepDuration(1, 126) = %v, want just under 1ms", got)
	}
	// 480 ticks at 120 BPM = 500 ms exactly.
	if got := StepDuration(480, 120); got != 500*time.Millisecond {
		t.Fatalf("StepDuration(480, 120) = %v, want 500ms", got)
	}
	// A 16th at 137 BPM is 109.489 ms; whole-ms rounding drifted ~1s per
	// 4-minute song.
	if got := StepDuration(120, 137); got < 109488*time.Microsecond || got > 109490*time.Microsecond {
		t.Fatalf("StepDuration(120, 137) = %v, want 109.489ms", got)
	}
	// Invalid inputs fall back to a sixteenth note at 120 BPM = 125 ms.
	if got := StepDuration(0, 0); got != 125*time.Millisecond {
		t.Fatalf("StepDuration(0, 0) = %v, want 125ms", got)
	}
}

// TestPlayStepRearticulatesRepeatedPitch guards the slurring bug: two
// consecutive steps on the same pitch must emit noteoff before noteon so the
// note is re-articulated instead of blending into one tone.
func TestPlayStepRearticulatesRepeatedPitch(t *testing.T) {
	s := NewSynth()
	var buf bytes.Buffer
	s.realtime = true
	s.stdin = &nopWriteCloser{buf: &buf}
	s.mu.Lock()
	s.playGen++
	s.noteGen = make(map[int]int)
	s.mu.Unlock()

	tab := &model.Tab{Tuning: model.Standard, Bars: []model.Bar{{
		Strings: []model.StringLine{{
			Segments: []model.Segment{
				{Char: '0', Value: 0, Position: 0, Width: 1},
				{Char: '0', Value: 0, Position: 4, Width: 1},
			},
		}},
	}}}

	if err := s.PlayStep(tab, PlaybackStep{Bar: 0, Col: 0, Ticks: 480, Sustain: 480}, 120); err != nil {
		t.Fatalf("first PlayStep: %v", err)
	}
	if !s.noteActive(40) {
		t.Fatal("pitch E2 should be active after the first step")
	}
	buf.Reset()
	if err := s.PlayStep(tab, PlaybackStep{Bar: 0, Col: 4, Ticks: 480, Sustain: 480}, 120); err != nil {
		t.Fatalf("second PlayStep: %v", err)
	}
	got := buf.String()
	offIdx := strings.Index(got, "noteoff 0 40")
	onIdx := strings.Index(got, "noteon 0 40 100")
	if offIdx < 0 || onIdx < 0 {
		t.Fatalf("expected noteoff+noteon for the repeated pitch, got %q", got)
	}
	if offIdx > onIdx {
		t.Fatalf("noteoff must precede noteon for re-articulation, got %q", got)
	}
}
