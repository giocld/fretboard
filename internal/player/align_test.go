package player

import (
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"fretboard/internal/model"
)

// testTab builds a long, regular tab: 240 two-quarter-note bars (~240 s at
// 120 BPM). The spacing heuristic gives each note a full quarter ("0---3---").
func testTab() *model.Tab {
	bar := model.Bar{Strings: []model.StringLine{{Segments: []model.Segment{
		{Char: '0', Value: 0, Position: 0, Width: 1},
		{Char: '-', Position: 1}, {Char: '-', Position: 2}, {Char: '-', Position: 3},
		{Char: '3', Value: 3, Position: 4, Width: 1},
		{Char: '-', Position: 5}, {Char: '-', Position: 6}, {Char: '-', Position: 7},
	}}}}
	bars := make([]model.Bar, 240)
	for i := range bars {
		bars[i] = bar
	}
	return &model.Tab{Title: "Test", Artist: "Synthetic", Bars: bars}
}

// TestAlignAudioRecoversTempoAndIntro guards the automatic alignment on a
// realistic recording: 118 BPM, a 3.2 s non-silent intro, eighth-note
// clicks with accented quarters. No manual anchors involved.
func TestAlignAudioRecoversTempoAndIntro(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	path := filepath.Join(t.TempDir(), "song.wav")
	bpm := 118
	beat := time.Duration(60000/bpm/2) * time.Millisecond // eighths: 254 ms
	intro := 3200 * time.Millisecond
	var clicks []time.Duration
	for i := 0; i < 480; i++ {
		clicks = append(clicks, intro+time.Duration(i)*beat)
	}
	// Accented quarters: every second eighth is strong.
	if err := writeSyntheticWAVAlt(path, 8000, clicks, 30*time.Millisecond, 2); err != nil {
		t.Fatal(err)
	}

	a := AlignAudio(testTab(), path, intro)
	if a.BPM == 0 {
		t.Fatal("alignment returned nothing")
	}
	if a.BPM < 117 || a.BPM > 119 {
		t.Fatalf("BPM = %d, want ~118", a.BPM)
	}
	if d := absDur(a.Offset - intro); d > 150*time.Millisecond {
		t.Fatalf("offset = %v, want ~3.2s", a.Offset)
	}
	if a.Confidence < 0.6 {
		t.Fatalf("confidence = %.2f, want >= 0.6", a.Confidence)
	}
}

// TestAlignAudioNoHintStillFindsOffset guards the strength cue alone: even
// without the silence hint the accented quarter grid pins the offset.
func TestAlignAudioNoHintStillFindsOffset(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	path := filepath.Join(t.TempDir(), "song.wav")
	bpm := 118
	beat := time.Duration(60000/bpm/2) * time.Millisecond
	intro := 3200 * time.Millisecond
	var clicks []time.Duration
	for i := 0; i < 480; i++ {
		clicks = append(clicks, intro+time.Duration(i)*beat)
	}
	if err := writeSyntheticWAVAlt(path, 8000, clicks, 30*time.Millisecond, 2); err != nil {
		t.Fatal(err)
	}
	a := AlignAudio(testTab(), path, 0)
	if a.BPM == 0 {
		t.Fatal("alignment returned nothing")
	}
	if a.BPM < 117 || a.BPM > 119 {
		t.Fatalf("BPM = %d, want ~118", a.BPM)
	}
	// Without a prior, the offset is recoverable only up to one beat (the
	// accent grid is periodic); congruence within a beat is the guarantee.
	// The drift meter (S3) resolves the residual during playback.
	period := time.Duration(60000/a.BPM) * time.Millisecond
	resid := absDur(a.Offset - intro) % period
	if resid > period/2 {
		resid = period - resid
	}
	if resid > 200*time.Millisecond {
		t.Fatalf("offset = %v, want congruent to ~3.2s mod %v (strength cue only)", a.Offset, period)
	}
}

// TestExpectedOnsets guards the expected-onset derivation: regular beats at
// the tab BPM, bar starts flagged.
func TestExpectedOnsets(t *testing.T) {
	tab := testTab()
	onsets := ExpectedOnsets(tab, 120)
	if len(onsets) < 100 {
		t.Fatalf("expected many onsets, got %d", len(onsets))
	}
	// Second note = 500 ms at 120 BPM (two quarter notes per bar).
	if d := absDur(onsets[1].Time - 500*time.Millisecond); d > 2*time.Millisecond {
		t.Fatalf("onset[1] = %v, want ~500ms", onsets[1].Time)
	}
	if !onsets[0].BarStart || onsets[1].BarStart || !onsets[2].BarStart {
		t.Fatalf("bar-start flags wrong: %+v %+v %+v", onsets[0], onsets[1], onsets[2])
	}
}

// TestNearestOnset guards the helper the drift meter will use.
func TestNearestOnset(t *testing.T) {
	onsets := []time.Duration{1000 * time.Millisecond, 1500 * time.Millisecond}
	if n, ok := NearestOnset(onsets, 1450*time.Millisecond, 300*time.Millisecond); !ok || n != 1500*time.Millisecond {
		t.Fatalf("nearest = %v %v", n, ok)
	}
	if _, ok := NearestOnset(onsets, 2500*time.Millisecond, 300*time.Millisecond); ok {
		t.Fatal("out-of-gap must not match")
	}
}
