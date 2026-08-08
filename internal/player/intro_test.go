package player

import (
	"path/filepath"
	"testing"
	"time"
)

const silenceLog = `[silencedetect @ 0x0] silence_start: 0
[silencedetect @ 0x0] silence_end: 3.2 | silence_duration: 3.2
[silencedetect @ 0x0] silence_start: 95.4
`

const noSilenceLog = `[silencedetect @ 0x0] silence_start: 1.2 | silence_duration: 0.2
`

// TestLeadingSilenceDetectsIntro guards US-14's intro assist: the first
// silence_end marks the end of the leading silence and becomes the offset.
func TestLeadingSilenceDetectsIntro(t *testing.T) {
	writeFakeFFmpeg(t, silenceLog)
	got, err := LeadingSilence(filepath.Join(t.TempDir(), "song.mp3"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 3200*time.Millisecond {
		t.Fatalf("LeadingSilence = %v, want 3.2s", got)
	}
}

// TestLeadingSilenceNoIntro guards the no-intro case: no silence_end before
// the music means no offset.
func TestLeadingSilenceNoIntro(t *testing.T) {
	writeFakeFFmpeg(t, noSilenceLog)
	got, err := LeadingSilence(filepath.Join(t.TempDir(), "song.mp3"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 0 {
		t.Fatalf("no leading silence should yield 0, got %v", got)
	}
}

// TestLeadingSilenceAbsentFFmpeg guards graceful degradation: without ffmpeg
// the assist is silently skipped.
func TestLeadingSilenceAbsentFFmpeg(t *testing.T) {
	got, err := LeadingSilence(filepath.Join(t.TempDir(), "song.mp3"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 0 {
		t.Fatalf("no ffmpeg should yield 0, got %v", got)
	}
}

// TestLeadingSilenceCountIn guards the count-in-aware detection: silence →
// click bursts → music anchors at the music, not at the first silence end.
func TestLeadingSilenceCountIn(t *testing.T) {
	log := "[silence_start: 0\n" +
		"[silence_end: 2.0\n" + // end of true intro silence
		"[silence_start: 2.3\n" + // silence between click groups
		"[silence_end: 2.6\n" +
		"[silence_start: 2.9\n" +
		"[silence_end: 3.1\n" + // last silence before the music
		"[silence_start: 12.0\n" + // a musical pause much later
		"[silence_end: 12.4\n"
	got, err := introOffsetFromSilenceLog(log)
	if err != nil {
		t.Fatal(err)
	}
	if got.Seconds() != 3.1 {
		t.Fatalf("count-in intro should anchor at 3.1s (music start), got %v", got)
	}
}

// TestLeadingSilencePauseNotMistakenForIntro guards the first-sustained
// rule: a pause after the music started must not shift the offset.
func TestLeadingSilencePauseNotMistakenForIntro(t *testing.T) {
	log := "[silence_start: 0\n" +
		"[silence_end: 1.5\n" + // intro silence, music from 1.5
		"[silence_start: 20.0\n" + // pause at 20s
		"[silence_end: 21.0\n"
	got, err := introOffsetFromSilenceLog(log)
	if err != nil {
		t.Fatal(err)
	}
	if got.Seconds() != 1.5 {
		t.Fatalf("offset should stay at the intro end (1.5s), got %v", got)
	}
}
