package player

import (
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// clickTrain builds click times: eighth notes at bpm1 for n1 clicks, then at
// bpm2 for n2 clicks (a real tempo change), after an optional intro.
func clickTrain(bpm1, n1, bpm2, n2 int, intro time.Duration) []time.Duration {
	var clicks []time.Duration
	t := intro
	interval := time.Duration(60000/bpm1/2) * time.Millisecond
	for i := 0; i < n1; i++ {
		clicks = append(clicks, t)
		t += interval
	}
	interval = time.Duration(60000/bpm2/2) * time.Millisecond
	for i := 0; i < n2; i++ {
		clicks = append(clicks, t)
		t += interval
	}
	return clicks
}

// TestTempoAnchorsMeasuredSpacing guards the auto tempo map: at a constant
// tempo the measured anchors land every 4 bars with the correct spacing.
func TestTempoAnchorsMeasuredSpacing(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	path := filepath.Join(t.TempDir(), "song.wav")
	clicks := clickTrain(120, 400, 120, 0, 1000*time.Millisecond)
	if err := writeSyntheticWAVAlt(path, 8000, clicks, 30*time.Millisecond, 2); err != nil {
		t.Fatal(err)
	}
	a := AlignAudio(testTab(), path, 1000*time.Millisecond)
	if a.Confidence < 0.6 {
		t.Fatalf("confidence %.2f too low", a.Confidence)
	}
	expected := ExpectedOnsets(testTab(), 120)
	scale := 120.0 / float64(a.BPM)
	anchors := TempoAnchors(expected, a.Onsets, scale, a.Offset, a.BPM, 4)
	if len(anchors) < 10 {
		t.Fatalf("expected many anchors, got %d", len(anchors))
	}
	// Spacing between consecutive anchors = 4 tab bars at ~1.0s each = ~4s.
	for i := 1; i < len(anchors) && i < 6; i++ {
		gap := anchors[i].Seconds - anchors[i-1].Seconds
		if gap < 3.5 || gap > 4.5 {
			t.Fatalf("anchor gap %v (bars %d->%d), want ~8s", gap, anchors[i-1].Bar, anchors[i].Bar)
		}
	}
}

// TestCorrectStepSnapFixesDrift guards the live self-correction: a mapping
// drifted off the onsets snaps to the onset-aligned step; on-target mappings
// are left alone; silence gaps never cause jumps.
func TestCorrectStepSnapFixesDrift(t *testing.T) {
	tab := testTab()
	sched := BuildSchedule(tab)
	onsets := make([]time.Duration, 0, 240)
	for i := 0; i < 240; i++ {
		onsets = append(onsets, 1000*time.Millisecond+time.Duration(i)*500*time.Millisecond)
	}
	points := []SyncPoint{{Bar: 1, Seconds: 1.0}}
	// On target: 60.0s is exactly an onset; no snap needed.
	idx, ok := CorrectStepSnap(sched, points, 60*time.Second, onsets, 120)
	if ok {
		t.Fatal("on-target mapping must not snap")
	}
	_ = idx
	// Drifted 300ms late: 60.3s is 200ms from the nearest onset (60.5); the
	// mapping must snap to the onset-aligned step.
	idx, ok = CorrectStepSnap(sched, points, 60*time.Second+300*time.Millisecond, onsets, 120)
	if !ok {
		t.Fatal("drifted mapping should snap")
	}
	want := StepIndexAtSyncPoints(sched, points, 60.5, 120)
	if idx != want {
		t.Fatalf("snap idx %d, want %d", idx, want)
	}
	// Far from any onset (a silence gap in a sparse grid): no snap.
	sparse := []time.Duration{60 * time.Second, 61 * time.Second}
	if _, ok := CorrectStepSnap(sched, points, 60*time.Second+700*time.Millisecond, sparse, 120); ok {
		t.Fatal("silence gap must not snap")
	}
}

// TestAutoMapSurvivesTempoChange is the S3 acceptance: a recording that
// slows from 120 to 100 BPM mid-song. The auto anchors + drift correction
// keep the mapped bar within one bar of the truth at every sampled time,
// where the constant-BPM mapping drifts after the change.
func TestAutoMapSurvivesTempoChange(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	path := filepath.Join(t.TempDir(), "song.wav")
	// 120 BPM 8ths for 30s, then 100 BPM 8ths for 45s, after a 1s intro.
	clicks := clickTrain(120, 240, 100, 300, 1000*time.Millisecond)
	if err := writeSyntheticWAVAlt(path, 8000, clicks, 30*time.Millisecond, 2); err != nil {
		t.Fatal(err)
	}
	tab := testTab()
	a := AlignAudio(tab, path, 1000*time.Millisecond)
	if a.BPM == 0 {
		t.Fatal("no alignment")
	}
	sched := BuildSchedule(tab)
	expected := ExpectedOnsets(tab, 120)
	scale := 120.0 / float64(a.BPM)
	anchors := TempoAnchors(expected, a.Onsets, scale, a.Offset, a.BPM, 4)
	points := MergeAnchors(nil, anchors)
	if len(points) < 4 {
		t.Fatalf("too few anchors: %d", len(points))
	}

	// True bar index at audio time t: bar starts every 2s until 30s, then
	// every 2.4s (the recording's grid).
	trueBar := func(t float64) int {
		el := t - 0.975 // aligned offset
		if el < 0 {
			return 0
		}
		if el <= 60.0 {
			return int(el / 1.0) // 1s tab bars (120 BPM clicks)
		}
		return 60 + int((el-60.0)/1.2) // 1.2s tab bars at 100 BPM
	}
	for _, at := range []float64{20, 40, 55, 65, 72} {
		want := trueBar(at)
		// Constant-BPM mapping (what the old approach did).
		rawIdx := StepIndexAtSyncPoints(sched, points, at, a.BPM)
		rawBar := sched[rawIdx].Bar
		// With the drift correction.
		corrected := rawIdx
		if snapIdx, ok := CorrectStepSnap(sched, points, time.Duration(at*float64(time.Second)), a.Onsets, a.BPM); ok {
			corrected = snapIdx
		}
		corrBar := sched[corrected].Bar
		if diff := absInt(corrBar - want); diff > 1 {
			t.Fatalf("t=%.0fs: corrected bar %d vs true %d (raw %d) — still broken", at, corrBar, want, rawBar)
		}
	}
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
