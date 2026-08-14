package viewer

import (
	"testing"
	"time"

	"fretboard/internal/player"
)

// syncScheduleN returns bars of uniform quarter-note steps (4 steps of 480
// ticks per bar = 4 quarters per bar = 2s per bar at 120 BPM).
func syncScheduleN(bars int) []player.PlaybackStep {
	var steps []player.PlaybackStep
	for bar := 0; bar < bars; bar++ {
		for i := 0; i < 4; i++ {
			steps = append(steps, player.PlaybackStep{Bar: bar, Col: i, Ticks: 480})
		}
	}
	return steps
}

// TestSyncElapsedSnapsToStrongestOnset guards T8's reflex-latency fix: a tap
// lands ~200-400ms after the beat the user heard, so the tap must snap to
// the strongest onset in the 400ms window BEFORE it. With strengths present
// the strongest onset in the window wins even when a weaker one is nearer;
// with nil strengths the nearest onset in the window wins.
func TestSyncElapsedSnapsToStrongestOnset(t *testing.T) {
	onsets := []time.Duration{2*time.Second + 300*time.Millisecond, 2*time.Second + 500*time.Millisecond}
	strengths := []float64{0.9, 0.1}
	tap := 2*time.Second + 600*time.Millisecond // window [2.2s, 2.6s]
	// The task's reference scenario: 2.0s and 2.5s onsets, tap at 2.6s ->
	// strongest in the window is 2.5s.
	if got := syncElapsed(tap, []time.Duration{2 * time.Second, 2*time.Second + 500*time.Millisecond}, []float64{0.3, 0.9}); got != 2*time.Second+500*time.Millisecond {
		t.Fatalf("reference scenario should snap to the strongest in-window onset, got %v", got)
	}
	// Strongest beats nearest: 2.3s (strength 0.9) vs 2.5s (strength 0.1).
	if got := syncElapsed(tap, onsets, strengths); got != onsets[0] {
		t.Fatalf("syncElapsed with strengths = %v, want strongest onset %v", got, onsets[0])
	}
	// Nil strengths: nearest onset in the window wins (2.5s, 0.1s away).
	if got := syncElapsed(tap, onsets, nil); got != onsets[1] {
		t.Fatalf("syncElapsed without strengths = %v, want nearest onset %v", got, onsets[1])
	}
	// Window bounds are inclusive: an onset exactly at the tap and one
	// exactly at window start are both candidates.
	if got := syncElapsed(tap, []time.Duration{tap - 400*time.Millisecond, tap}, []float64{0.1, 0.9}); got != tap {
		t.Fatalf("onset exactly at the tap must be a candidate, got %v", got)
	}
	if got := syncElapsed(tap, []time.Duration{tap - 400*time.Millisecond, tap - 200*time.Millisecond}, nil); got != tap-200*time.Millisecond {
		t.Fatalf("nearest in-window onset must win, got %v", got)
	}
}

// TestSyncElapsedFallsBackToRawElapsed guards T8's no-change rule: when no
// onset sits in the window before the tap (or there are no onsets at all),
// the raw tap moment is kept.
func TestSyncElapsedFallsBackToRawElapsed(t *testing.T) {
	tap := 3 * time.Second
	onsets := []time.Duration{tap - 500*time.Millisecond, tap + 500*time.Millisecond}
	if got := syncElapsed(tap, onsets, nil); got != tap {
		t.Fatalf("no onset in the window must keep the raw tap, got %v", got)
	}
	if got := syncElapsed(tap, nil, nil); got != tap {
		t.Fatalf("no onsets must keep the raw tap, got %v", got)
	}
}

// TestRefineSyncPointsMADRejection guards T8's robust fit: with at least 4
// anchors, an anchor whose adjacent segment tempo deviates more than 2*MAD
// from the median is dropped and the survivors are re-fitted; the consistent
// anchors survive byte-for-byte. Fewer than 4 anchors stay unchanged, and
// non-derivable (0 BPM) segments carry no information - never an outlier.
func TestRefineSyncPointsMADRejection(t *testing.T) {
	schedule := syncScheduleN(6) // 4 quarters per bar, 2s/bar at 120 BPM
	// Bars 1-4 sit exactly on the 120 BPM grid; bar 5 is a wild mistap
	// (28s instead of 8s), so its segment tempo is an extreme outlier.
	points := []player.SyncPoint{
		{Bar: 1, Seconds: 0}, {Bar: 2, Seconds: 2}, {Bar: 3, Seconds: 4},
		{Bar: 4, Seconds: 6}, {Bar: 5, Seconds: 28},
	}
	got := refineSyncPoints(schedule, points)
	if len(got) != 4 {
		t.Fatalf("refined anchors = %d, want 4 (outlier dropped): %+v", len(got), got)
	}
	for i, want := range points[:4] {
		if got[i] != want {
			t.Fatalf("kept anchor %d = %+v, want %+v", i, got[i], want)
		}
	}
}

// TestRefineSyncPointsTooFewOrNoInfo guards T8's no-change rules: < 4
// anchors are too few for robust statistics, a zero-span (0 BPM) segment is
// no information rather than an outlier, and a nil schedule derives nothing.
func TestRefineSyncPointsTooFewOrNoInfo(t *testing.T) {
	schedule := syncScheduleN(6)
	three := []player.SyncPoint{{Bar: 1, Seconds: 0}, {Bar: 2, Seconds: 2}, {Bar: 3, Seconds: 4}}
	if got := refineSyncPoints(schedule, three); len(got) != 3 {
		t.Fatalf("< 4 anchors must stay unchanged, got %+v", got)
	}
	// Two consecutive anchors with identical times: zero span -> SegmentBPM 0.
	zeroSpan := []player.SyncPoint{
		{Bar: 1, Seconds: 0}, {Bar: 2, Seconds: 2}, {Bar: 3, Seconds: 4},
		{Bar: 4, Seconds: 6}, {Bar: 5, Seconds: 6},
	}
	if got := refineSyncPoints(schedule, zeroSpan); len(got) != 5 {
		t.Fatalf("a zero-span segment must not count as an outlier, got %+v", got)
	}
	five := []player.SyncPoint{
		{Bar: 1, Seconds: 0}, {Bar: 2, Seconds: 2}, {Bar: 3, Seconds: 4},
		{Bar: 4, Seconds: 6}, {Bar: 5, Seconds: 8},
	}
	if got := refineSyncPoints(nil, five); len(got) != 5 {
		t.Fatalf("a nil schedule must leave anchors unchanged, got %+v", got)
	}
}

// TestSetSyncPointKeepsRawElapsedWhenWindowEmpty guards the setSyncPoint
// wiring through the real model: with a loaded tab and auto onsets but no
// engine playback (Elapsed() == 0) no onset can sit in the 400ms window
// before the tap, so the anchor must land on the raw elapsed - the pre-T8
// behavior - proving the snap path degrades cleanly.
func TestSetSyncPointKeepsRawElapsedWhenWindowEmpty(t *testing.T) {
	m := NewViewerModel()
	m.LoadTab(sampleTab(), "", 0)
	m.autoOnsets = []time.Duration{2 * time.Second, 2*time.Second + 500*time.Millisecond}
	m.autoStrengths = []float64{0.3, 0.9}
	m.cursorBar = 3
	m, _ = m.setSyncPoint()
	if len(m.syncPoints) != 1 || m.syncPoints[0].Bar != 4 {
		t.Fatalf("sync point should anchor bar 4, got %+v", m.syncPoints)
	}
	if m.syncPoints[0].Seconds != 0 {
		t.Fatalf("an empty tap window must keep the raw elapsed 0, got %v", m.syncPoints[0].Seconds)
	}
}

// TestSetSyncPointBarOneOffsetUsesSnappedElapsed guards the bar-1 path of
// the wiring: the audio offset and the appended anchor must both carry the
// same snapped elapsed.
func TestSetSyncPointBarOneOffsetUsesSnappedElapsed(t *testing.T) {
	m := NewViewerModel()
	m.LoadTab(sampleTab(), "", 0)
	m.cursorBar = 0 // user bar 1
	m, _ = m.setSyncPoint()
	if m.audioOffset != 0 {
		t.Fatalf("bar-1 offset should be the snapped elapsed (0), got %v", m.audioOffset)
	}
	if len(m.syncPoints) != 1 || m.syncPoints[0].Bar != 1 {
		t.Fatalf("bar-1 anchor expected, got %+v", m.syncPoints)
	}
	if m.syncPoints[0].Seconds != m.audioOffset {
		t.Fatalf("bar-1 anchor seconds %v must equal the offset %v", m.syncPoints[0].Seconds, m.audioOffset)
	}
}
