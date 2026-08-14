package player

import (
	"os/exec"
	"path/filepath"
	"strconv"
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

// testTabAt builds the standard test tab with an explicit tempo so a test
// can force the primary (tab-derived) BPM window into a regime the old
// 60-BPM lower clamp could not reach.
func testTabAt(bpm int) *model.Tab {
	tab := testTab()
	tab.Metadata = map[string]string{model.MetaKeyBPM: strconv.Itoa(bpm)}
	return tab
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
	resid := absDur(a.Offset-intro) % period
	if resid > period/2 {
		resid = period - resid
	}
	if resid > 200*time.Millisecond {
		t.Fatalf("offset = %v, want congruent to ~3.2s mod %v (strength cue only)", a.Offset, period)
	}
}

// TestAlignAudioSecondBPMWindow guards the ratio-gated second BPM window: a
// tab whose tempo is off (45 BPM) from a recording that is slower still
// (33 BPM) would invert the old primary window to empty (clamped to
// [60,56]) and return nothing. The window derived from the audio's own
// median inter-onset interval must rescue the tempo and the offset. The
// clicks are accented every two beats so the bar-start strength cue is
// present; AlignAudio sorts the two-pass onset list into time order so the
// median interval is the single-beat period.
func TestAlignAudioSecondBPMWindow(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	path := filepath.Join(t.TempDir(), "slow.wav")
	audioBPM := 33
	beat := time.Duration(60000/audioBPM) * time.Millisecond // quarters: 1818 ms
	intro := 1000 * time.Millisecond
	var clicks []time.Duration
	for i := 0; i < 48; i++ {
		clicks = append(clicks, intro+time.Duration(i)*beat)
	}
	if err := writeSyntheticWAVAlt(path, 8000, clicks, 30*time.Millisecond, 2); err != nil {
		t.Fatal(err)
	}
	a := AlignAudio(testTabAt(45), path, intro)
	if a.BPM == 0 {
		t.Fatal("alignment returned nothing")
	}
	if a.BPM < 31 || a.BPM > 35 {
		t.Fatalf("BPM = %d, want ~33 (audio-derived window)", a.BPM)
	}
	if d := absDur(a.Offset - intro); d > 250*time.Millisecond {
		t.Fatalf("offset = %v, want ~1s", a.Offset)
	}
	if a.Confidence < 0.5 {
		t.Fatalf("confidence = %.2f, want >= 0.5", a.Confidence)
	}
}

// TestAlignAudioOnsetSeededOffset guards the onset-seeded offset search: a
// recording whose opening silence lies beyond the old blind 0..15 s scan can
// only be aligned because each early detected onset seeds a candidate
// offset. Same fixture as the intro tests (118 BPM, accented eighth clicks)
// but a 16 s intro and no hint — the seeded search alone must recover both
// tempo and offset.
func TestAlignAudioOnsetSeededOffset(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	path := filepath.Join(t.TempDir(), "long_intro.wav")
	bpm := 118
	beat := time.Duration(60000/bpm/2) * time.Millisecond // eighths: 254 ms
	intro := 16 * time.Second
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
	if d := absDur(a.Offset - intro); d > 150*time.Millisecond {
		t.Fatalf("offset = %v, want ~16s (seeded search, no hint)", a.Offset)
	}
	if a.Confidence < 0.6 {
		t.Fatalf("confidence = %.2f, want >= 0.6", a.Confidence)
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

// TestAlignAudioErrPropagated guards F3: when the analysis itself fails (no
// decoder available), AlignAudio must surface the error in the alignment
// instead of returning a silent zero result.
func TestAlignAudioErrPropagated(t *testing.T) {
	orig := findDecoder
	findDecoder = func() string { return "" }
	defer func() { findDecoder = orig }()

	path := filepath.Join(t.TempDir(), "song.wav")
	if err := writeSyntheticWAV(path, 8000, []time.Duration{time.Second}, 30*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	a := AlignAudio(testTab(), path, 0)
	if a.Err == nil {
		t.Fatal("an analysis failure must propagate through AlignAudio")
	}
	if a.BPM != 0 || a.Confidence != 0 {
		t.Fatalf("a failed analysis must stay zero-confidence, got BPM=%d conf=%.2f", a.BPM, a.Confidence)
	}
}

// TestAlignAudioTooFewOnsetsNoErr guards F3's boundary: a usable-but-weak
// analysis (empty path decodes to no onsets) returns a zero-confidence
// alignment, not an error — only real analysis failures populate Err.
func TestAlignAudioTooFewOnsetsNoErr(t *testing.T) {
	a := AlignAudio(testTab(), "", 0)
	if a.Err != nil {
		t.Fatalf("too few onsets must not carry an error, got %v", a.Err)
	}
	if a.BPM != 0 || a.Confidence != 0 {
		t.Fatalf("too few onsets must stay zero-confidence, got BPM=%d conf=%.2f", a.BPM, a.Confidence)
	}
}
