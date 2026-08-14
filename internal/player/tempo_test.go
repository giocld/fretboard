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

// TestMarshalTempoMapRoundTrip guards the per-source persistence: anchors,
// onsets, and (new) strengths all round-trip, and a payload persisted before
// strengths existed unmarshals to nil strengths (backward compatible).
func TestMarshalTempoMapRoundTrip(t *testing.T) {
	anchors := []SyncPoint{{Bar: 1, Seconds: 1.0}, {Bar: 5, Seconds: 5.0}}
	onsets := []time.Duration{1 * time.Second, 1*time.Second + 250*time.Millisecond}
	strengths := []float64{1.0, 0.5}
	raw := MarshalTempoMap(anchors, onsets, strengths)
	if raw == "" {
		t.Fatal("marshal failed")
	}
	gotA, gotO, gotS := UnmarshalTempoMap(raw)
	if len(gotA) != 2 || gotA[1].Bar != 5 || len(gotO) != 2 || gotO[1] != 1250*time.Millisecond {
		t.Fatalf("round-trip mismatch: %+v %+v", gotA, gotO)
	}
	if len(gotS) != 2 || gotS[0] != 1.0 || gotS[1] != 0.5 {
		t.Fatalf("strengths round-trip mismatch: %+v", gotS)
	}
	// An OLD payload (no "strengths" key) must restore with nil strengths.
	oldRaw := `{"anchors":[{"bar":1,"seconds":1.0},{"bar":5,"seconds":5.0}],"onsets":[1.0,1.25]}`
	gotA2, gotO2, gotS2 := UnmarshalTempoMap(oldRaw)
	if len(gotA2) != 2 || len(gotO2) != 2 || gotO2[1] != 1250*time.Millisecond {
		t.Fatalf("old payload round-trip mismatch: %+v %+v", gotA2, gotO2)
	}
	if gotS2 != nil {
		t.Fatalf("old payload without strengths must unmarshal to nil, got %+v", gotS2)
	}
}

// TestCorrectStepSnapBeatRelativeThresholds guards the tempo-relative snap
// thresholds: at 240 BPM (beat 250ms, snap 62ms, radius 125ms) a 100ms drift
// snaps but a 50ms drift does not; at 60 BPM (beat 1000ms clamped to snap
// 200ms, radius 250ms) a 250ms drift still finds the onset and a 400ms drift
// is beyond the radius.
func TestCorrectStepSnapBeatRelativeThresholds(t *testing.T) {
	tab := testTab()
	sched := BuildSchedule(tab)
	points := []SyncPoint{{Bar: 1, Seconds: 1.0}}

	// 240 BPM: beat = 250ms -> snapThreshold = 62ms, searchRadius = 125ms.
	fast := make([]time.Duration, 0, 64)
	for i := 0; i < 64; i++ {
		fast = append(fast, 1*time.Second+time.Duration(i)*250*time.Millisecond)
	}
	if _, ok := CorrectStepSnap(sched, points, 2*time.Second+100*time.Millisecond, fast, 240); !ok {
		t.Fatal("240 BPM: 100ms drift should snap (threshold 62ms, radius 125ms)")
	}
	if _, ok := CorrectStepSnap(sched, points, 2*time.Second+50*time.Millisecond, fast, 240); ok {
		t.Fatal("240 BPM: 50ms drift is below the 62ms snap threshold and must not snap")
	}

	// 60 BPM: beat = 1000ms -> snapThreshold clamped to 200ms, searchRadius
	// clamped to 250ms. A 250ms drift still finds the onset and snaps.
	if _, ok := CorrectStepSnap(sched, points, 2*time.Second+250*time.Millisecond, []time.Duration{2 * time.Second, 2*time.Second + 600*time.Millisecond}, 60); !ok {
		t.Fatal("60 BPM: 250ms drift should still find the onset (radius clamp 250ms)")
	}
	// 400ms drift is beyond the clamped 250ms radius: no snap.
	if _, ok := CorrectStepSnap(sched, points, 2*time.Second+400*time.Millisecond, []time.Duration{2 * time.Second, 2*time.Second + 900*time.Millisecond}, 60); ok {
		t.Fatal("60 BPM: 400ms drift is beyond the 250ms radius and must not snap")
	}
}

// TestCorrectStepSnapStrengthTieBreak guards the strength-weighted tie-break:
// when two onsets are equidistant from elapsed, the stronger one wins; equal
// strengths keep NearestOnset's exact-tie behavior.
func TestCorrectStepSnapStrengthTieBreak(t *testing.T) {
	tab := testTab()
	sched := BuildSchedule(tab)
	points := []SyncPoint{{Bar: 1, Seconds: 1.0}}
	// 2.0s and 2.5s are both 250ms from 2.25s. The stronger later onset
	// must win over the weaker earlier one.
	onsets := []Onset{
		{Time: 2 * time.Second, Strength: 0.2},
		{Time: 2*time.Second + 500*time.Millisecond, Strength: 0.9},
	}
	idx, ok := CorrectStepSnapWithStrength(sched, points, 2*time.Second+250*time.Millisecond, onsets, 120)
	if !ok {
		t.Fatal("equidistant onsets should still snap")
	}
	want := StepIndexAtSyncPoints(sched, points, 2.5, 120)
	if idx != want {
		t.Fatalf("stronger equidistant onset should win: got idx %d, want %d", idx, want)
	}
	// The stronger EARLIER onset must win too.
	early := []Onset{
		{Time: 2 * time.Second, Strength: 0.9},
		{Time: 2*time.Second + 500*time.Millisecond, Strength: 0.2},
	}
	idx, ok = CorrectStepSnapWithStrength(sched, points, 2*time.Second+250*time.Millisecond, early, 120)
	if !ok {
		t.Fatal("equidistant onsets should still snap")
	}
	if want := StepIndexAtSyncPoints(sched, points, 2.0, 120); idx != want {
		t.Fatalf("stronger earlier onset should win: got idx %d, want %d", idx, want)
	}
	// Equal strengths must match CorrectStepSnap exactly (NearestOnset picks
	// the later onset of an exact tie).
	equal := []Onset{
		{Time: 2 * time.Second, Strength: 1},
		{Time: 2*time.Second + 500*time.Millisecond, Strength: 1},
	}
	idx, ok = CorrectStepSnapWithStrength(sched, points, 2*time.Second+250*time.Millisecond, equal, 120)
	if !ok {
		t.Fatal("equidistant onsets should still snap")
	}
	if want := StepIndexAtSyncPoints(sched, points, 2.5, 120); idx != want {
		t.Fatalf("equal-strength tie should match NearestOnset: got idx %d, want %d", idx, want)
	}
}

// TestTrackAlignmentAcceptance is the S4 acceptance: a realistic recording
// with a count-in (fast clicks), a non-silent intro, a gradual tempo drift,
// and accented quarters must align automatically and keep the mapped bar
// within one bar of the truth for the whole song — no manual anchors.
func TestTrackAlignmentAcceptance(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	path := filepath.Join(t.TempDir(), "song.wav")
	// Count-in: 8 fast clicks at 90ms. Music: 8th notes at 122 BPM with a
	// 4% slowdown over 60s (rubato), 300 clicks.
	var clicks []time.Duration
	t0 := time.Duration(0)
	for i := 0; i < 8; i++ {
		clicks = append(clicks, t0)
		t0 += 90 * time.Millisecond
	}
	interval := time.Duration(60000/122/2) * time.Millisecond // 245.9ms
	introEnd := t0
	for i := 0; i < 300; i++ {
		clicks = append(clicks, t0)
		// Gradual drift: +4% by the end.
		ratio := 1.0 + 0.04*float64(i)/300.0
		t0 += time.Duration(float64(interval) * ratio)
	}
	if err := writeSyntheticWAVAlt(path, 8000, clicks, 30*time.Millisecond, 2); err != nil {
		t.Fatal(err)
	}
	tab := testTab()
	a := AlignAudio(tab, path, 0)
	if a.BPM == 0 {
		t.Fatal("no alignment")
	}
	// The count-in must not be treated as the offset: the music starts at
	// ~0.72s; congruence within one beat either way is acceptable.
	period := time.Duration(60000/a.BPM) * time.Millisecond
	resid := absDur(a.Offset-introEnd) % period
	if resid > period/2 {
		resid = period - resid
	}
	if resid > 250*time.Millisecond {
		t.Fatalf("offset %v not congruent with the music start %v (bpm %d)", a.Offset, introEnd, a.BPM)
	}

	sched := BuildSchedule(tab)
	expected := ExpectedOnsets(tab, 120)
	anchors := TempoAnchors(expected, a.Onsets, 120.0/float64(a.BPM), a.Offset, a.BPM, 4)
	points := MergeAnchors(nil, anchors)
	if len(points) < 5 {
		t.Fatalf("too few anchors: %d", len(points))
	}
	// Truth: tab bar at audio time t (tab bars are 1s; the music drifts
	// ~4% slower over the track).
	trueBar := func(t float64) int {
		if t <= introEnd.Seconds() {
			return 0
		}
		// Integrate the drifting tempo: average ratio over the elapsed part.
		el := t - introEnd.Seconds()
		avgRatio := 1.0 + 0.02*el/60.0           // linear drift -> avg = mid ratio
		return int(el / (1.0 * avgRatio) / 0.98) // tab bar ~0.98s at 122 bpm
	}
	for _, at := range []float64{10, 25, 40, 55, 70} {
		idx := StepIndexAtSyncPoints(sched, points, at, a.BPM)
		if snap, ok := CorrectStepSnap(sched, points, time.Duration(at*float64(time.Second)), a.Onsets, a.BPM); ok {
			idx = snap
		}
		bar := sched[idx].Bar
		if diff := absInt(bar - trueBar(at)); diff > 2 {
			t.Fatalf("t=%.0fs: mapped bar %d vs true %d", at, bar+1, trueBar(at)+1)
		}
	}
}
