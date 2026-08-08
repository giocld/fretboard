package player

import (
	"testing"
)

// denseSparseSchedule builds a schedule where bar 0 is dense (8 sixteenth
// steps of 240 ticks = 1920) and bar 1 is sparse (2 steps of 60 ticks =
// 120). The dense bar holds 94% of the segment's ticks but only 80% of its
// steps — the property that makes tick-aware mapping differ from
// step-count mapping.
func denseSparseSchedule() []PlaybackStep {
	var steps []PlaybackStep
	for i := 0; i < 8; i++ {
		steps = append(steps, PlaybackStep{Bar: 0, Col: i * 2, Ticks: 240})
	}
	for i := 0; i < 2; i++ {
		steps = append(steps, PlaybackStep{Bar: 1, Col: i * 4, Ticks: 60})
	}
	return steps
}

// TestSegmentStepUsesTickDensity guards US-13: mid-segment mapping must be
// proportional to MIDI ticks, not to raw step count. At 90% of the segment
// time the accumulated ticks are still inside the dense bar (step 7); a
// step-count mapping would already be in the sparse bar (step 9).
func TestSegmentStepUsesTickDensity(t *testing.T) {
	schedule := denseSparseSchedule()
	a := SyncPoint{Bar: 0, Seconds: 0}
	b := SyncPoint{Bar: 2, Seconds: 10}

	if got := segmentStep(schedule, a, b, 0); got != 0 {
		t.Fatalf("at anchor A cursor should be step 0, got %d", got)
	}
	// 50% of 2040 ticks = 1020 -> step 4 (step-count mapping would say 5).
	if got := segmentStep(schedule, a, b, 5.0); got != 4 {
		t.Fatalf("50%% segment time should land at step 4, got %d", got)
	}
	// 90% of 2040 ticks = 1836 -> still bar 0 (step 7); the sparse bar only
	// starts at 94% of the audio time.
	if got := segmentStep(schedule, a, b, 9.0); got != 7 {
		t.Fatalf("90%% segment time should stay in the dense bar (step 7), got %d", got)
	}
	if got := segmentStep(schedule, a, b, 10); got != 9 {
		t.Fatalf("at anchor B cursor should be the last step (9), got %d", got)
	}
}

func TestTicksBetweenBars(t *testing.T) {
	schedule := denseSparseSchedule()
	if got := TicksBetweenBars(schedule, 0, 1); got != 1920 {
		t.Fatalf("bar 0 ticks = %d, want 1920", got)
	}
	if got := TicksBetweenBars(schedule, 0, 2); got != 2040 {
		t.Fatalf("total ticks = %d, want 2040", got)
	}
	if got := TicksBetweenBars(schedule, 1, 2); got != 120 {
		t.Fatalf("bar 1 ticks = %d, want 120", got)
	}
}

// TestSegmentBPMDerivesTempo guards the anchor-derived tempo map: 8 quarters
// spanning 5 seconds is 96 BPM.
func TestSegmentBPMDerivesTempo(t *testing.T) {
	schedule := []PlaybackStep{
		{Bar: 0, Ticks: 480}, {Bar: 0, Ticks: 480}, {Bar: 0, Ticks: 480}, {Bar: 0, Ticks: 480},
		{Bar: 1, Ticks: 480}, {Bar: 1, Ticks: 480}, {Bar: 1, Ticks: 480}, {Bar: 1, Ticks: 480},
	}
	got := SegmentBPM(schedule, SyncPoint{Bar: 0, Seconds: 0}, SyncPoint{Bar: 2, Seconds: 5})
	if got != 96 {
		t.Fatalf("SegmentBPM = %d, want 96", got)
	}
	if got := SegmentBPM(schedule, SyncPoint{Bar: 0, Seconds: 0}, SyncPoint{Bar: 0, Seconds: 0}); got != 0 {
		t.Fatalf("zero-length segment should not derive a tempo, got %d", got)
	}
}

func TestTicksToSeconds(t *testing.T) {
	if got := TicksToSeconds(480, 120); got != 0.5 {
		t.Fatalf("TicksToSeconds(480, 120) = %v, want 0.5", got)
	}
	if got := TicksToSeconds(960, 120); got != 1.0 {
		t.Fatalf("TicksToSeconds(960, 120) = %v, want 1.0", got)
	}
}

// TestStepIndexAtSyncPointsFollowsAnchors guards the anchor path end to end:
// with two anchors, audio time between them maps through the tick-aware
// segment mapping, and time before the first anchor holds at step 0.
func TestStepIndexAtSyncPointsFollowsAnchors(t *testing.T) {
	schedule := denseSparseSchedule()
	points := []SyncPoint{{Bar: 0, Seconds: 0}, {Bar: 2, Seconds: 10}}
	// 30% of 2040 ticks = 612 -> step 2 (tick-proportional, not step-count 3).
	if got := StepIndexAtSyncPoints(schedule, points, 3, 120); got != 2 {
		t.Fatalf("30%% in should land at step 2, got %d", got)
	}
	if got := StepIndexAtSyncPoints(schedule, points, -1, 120); got != 0 {
		t.Fatalf("before the first anchor should hold step 0, got %d", got)
	}
	if got := StepIndexAtSyncPoints(schedule, points, 15, 120); got != 9 {
		t.Fatalf("past the last anchor should clamp at the final step, got %d", got)
	}
}
