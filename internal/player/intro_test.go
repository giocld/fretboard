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
