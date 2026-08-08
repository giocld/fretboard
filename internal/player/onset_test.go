package player

import (
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestDetectOnsetsSynthetic guards the analysis pipeline end to end: a WAV
// with clicks at 120 BPM behind a 3.2 s intro must yield onsets at the
// click times. Skipped when ffmpeg is unavailable.
func TestDetectOnsetsSynthetic(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	path := filepath.Join(t.TempDir(), "clicks.wav")
	bpm := 120
	beat := time.Duration(60000/bpm) * time.Millisecond
	offset := 3200 * time.Millisecond
	var clicks []time.Duration
	for i := 0; i < 60; i++ {
		clicks = append(clicks, offset+time.Duration(i)*beat)
	}
	if err := writeSyntheticWAV(path, 8000, clicks, 30*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	onsets, err := DetectOnsets(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(onsets) < 40 {
		t.Fatalf("expected most clicks detected, got %d: %v", len(onsets), onsets)
	}
	// The first onset must be at the first click ± 40 ms.
	if d := time.Duration(mathAbs(int64(onsets[0] - offset))); d > 40*time.Millisecond {
		t.Fatalf("first onset %v, want ~%v", onsets[0], offset)
	}
	// Median inter-onset interval must be the beat ± 20 ms.
	var gaps []int64
	for i := 1; i < len(onsets); i++ {
		gaps = append(gaps, int64(onsets[i]-onsets[i-1]))
	}
	med := medianInt64(gaps)
	if d := time.Duration(mathAbs(med - int64(beat))); d > 20*time.Millisecond {
		t.Fatalf("median inter-onset %v, want ~%v", time.Duration(med), beat)
	}
}

func mathAbs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

func medianInt64(v []int64) int64 {
	if len(v) == 0 {
		return 0
	}
	// insertion sort (small n)
	for i := 1; i < len(v); i++ {
		for j := i; j > 0 && v[j] < v[j-1]; j-- {
			v[j], v[j-1] = v[j-1], v[j]
		}
	}
	return v[len(v)/2]
}
