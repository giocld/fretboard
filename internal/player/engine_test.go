package player

import (
	"testing"
	"time"
)

// TestElapsedAccountsForAudioBaseAndRate guards against Elapsed() reporting
// raw wall-clock time since playback start. Playback rate and prior seeks
// (audioBase) must be folded in, or rate changes seek to the wrong position
// and the playhead cursor runs at the wrong speed.
func TestElapsedAccountsForAudioBaseAndRate(t *testing.T) {
	e := NewEngine()
	e.mode = "audio"
	e.playbackStart = time.Now().Add(-10 * time.Second)

	cases := []struct {
		name     string
		base     time.Duration
		rate     float64
		min, max time.Duration
	}{
		{"rate 1 keeps base", 5 * time.Second, 1, 14 * time.Second, 16 * time.Second},
		{"rate 2 doubles wall time", 5 * time.Second, 2, 24 * time.Second, 26 * time.Second},
		{"rate 0.5 halves wall time", 0, 0.5, 4 * time.Second, 6 * time.Second},
		{"zero base at rate 1", 0, 1, 9 * time.Second, 11 * time.Second},
	}
	for _, tc := range cases {
		e.audioBase = tc.base
		e.rate = tc.rate
		got := e.Elapsed()
		if got < tc.min || got > tc.max {
			t.Fatalf("%s: Elapsed() = %v, want ~%v", tc.name, got, tc.base+time.Duration(float64(10*time.Second)*tc.rate))
		}
	}

	// Non-audio modes and unstarted playback report zero.
	e.mode = "midi"
	if got := e.Elapsed(); got != 0 {
		t.Fatalf("midi mode: Elapsed() = %v, want 0", got)
	}
	e.mode = "audio"
	e.playbackStart = time.Time{}
	if got := e.Elapsed(); got != 0 {
		t.Fatalf("unstarted: Elapsed() = %v, want 0", got)
	}
}
