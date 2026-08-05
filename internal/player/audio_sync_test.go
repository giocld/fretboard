package player

import (
	"testing"
	"time"

	"fretboard/internal/model"
)

func TestStepIndexAtScheduleTime(t *testing.T) {
	schedule := []PlaybackStep{
		{Bar: 0, Col: 0, Ticks: 480},
		{Bar: 0, Col: 4, Ticks: 480},
		{Bar: 1, Col: 0, Ticks: 960},
	}
	// 480 ticks = 1 quarter note; at 120 BPM one quarter = 500ms.
	if got := StepIndexAtScheduleTime(schedule, 0, 120); got != 0 {
		t.Fatalf("start: got %d want 0", got)
	}
	if got := StepIndexAtScheduleTime(schedule, -1*time.Second, 120); got != 0 {
		t.Fatalf("before start (negative music time): got %d want 0", got)
	}
	if got := StepIndexAtScheduleTime(schedule, 600*time.Millisecond, 120); got != 1 {
		t.Fatalf("second quarter: got %d want 1", got)
	}
	// A slower bar (960 ticks = 1s) must hold the cursor longer than a
	// linear mapping across the audio would.
	if got := StepIndexAtScheduleTime(schedule, 1100*time.Millisecond, 120); got != 2 {
		t.Fatalf("slow bar boundary: got %d want 2", got)
	}
	if got := StepIndexAtScheduleTime(schedule, 10*time.Second, 120); got != 2 {
		t.Fatalf("past end: got %d want 2", got)
	}
	// Different BPM shifts all boundaries.
	if got := StepIndexAtScheduleTime(schedule, 300*time.Millisecond, 240); got != 1 {
		t.Fatalf("240 BPM: got %d want 1", got)
	}
	if got := StepIndexAtScheduleTime(nil, 1*time.Second, 120); got != 0 {
		t.Fatalf("empty schedule: got %d want 0", got)
	}
}

func TestScoreYouTubeResultPrefersGuitarOverLesson(t *testing.T) {
	tab := &model.Tab{Title: "Layla", Artist: "Eric Clapton"}
	lesson := ScoreYouTubeResult(tab, "Eric Clapton Layla Electric Guitar Lesson + Tutorial", "Marty Music", "", 1049)
	official := ScoreYouTubeResult(tab, "Eric Clapton - Layla (Official Audio)", "Eric Clapton", "studio recording", 271)
	if official <= lesson {
		t.Fatalf("official score %d should beat lesson %d", official, lesson)
	}
}

func TestDeriveBPMFromAudioExcludesOffset(t *testing.T) {
	schedule := []PlaybackStep{{Ticks: ticksPerQuarter * 4}, {Ticks: ticksPerQuarter * 4}}
	audioDur := 2 * time.Minute
	base := DeriveBPMFromAudio(schedule, audioDur, 0)
	intro := DeriveBPMFromAudio(schedule, audioDur, 20*time.Second)
	if intro <= base {
		t.Fatalf("with a 20s intro the derived BPM (%d) must be higher than the no-intro BPM (%d)", intro, base)
	}
	if got := DeriveBPMFromAudio(schedule, audioDur, 10*time.Minute); got != base {
		t.Fatalf("offset beyond audio length should fall back to the full-duration BPM, got %d", got)
	}
}

func TestStepIndexAtSyncPoints(t *testing.T) {
	// Schedule steps carry 0-based tab-bar indices (BuildSchedule: bar = range
	// index), so sync-point bars must be 0-based too.
	schedule := []PlaybackStep{
		{Bar: 0, Ticks: 4 * ticksPerQuarter}, {Bar: 0, Ticks: 4 * ticksPerQuarter},
		{Bar: 1, Ticks: 4 * ticksPerQuarter}, {Bar: 1, Ticks: 4 * ticksPerQuarter},
		{Bar: 2, Ticks: 4 * ticksPerQuarter}, {Bar: 2, Ticks: 4 * ticksPerQuarter},
	}
	// Bar1 anchored at 10s, bar2 at 20s, bar3 at 25s: bar2 is 4x faster.
	points := []SyncPoint{{Bar: 0, Seconds: 10}, {Bar: 1, Seconds: 20}, {Bar: 2, Seconds: 25}}
	if got := StepIndexAtSyncPoints(schedule, points, 9.9, 120); got != 0 {
		t.Fatalf("before the first anchor should sit at step 0, got %d", got)
	}
	if got := StepIndexAtSyncPoints(schedule, points, 10, 120); got != 0 {
		t.Fatalf("at the first anchor should sit at step 0, got %d", got)
	}
	if got := StepIndexAtSyncPoints(schedule, points, 15, 120); got != 1 {
		t.Fatalf("mid bar1..bar2 segment should be step 1, got %d", got)
	}
	if got := StepIndexAtSyncPoints(schedule, points, 25, 120); got != 4 {
		t.Fatalf("at the bar3 anchor should be step 4, got %d", got)
	}
	if got := StepIndexAtSyncPoints(schedule, points, 35, 120); got != 5 {
		t.Fatalf("past the last anchor should clamp to the last step, got %d", got)
	}
	if got := StepIndexAtSyncPoints(schedule, nil, 30, 120); got != 5 {
		t.Fatalf("no anchors should use plain schedule accumulation, got %d", got)
	}
	if got := StepIndexAtSyncPoints(nil, points, 30, 120); got != 0 {
		t.Fatalf("empty schedule should be step 0, got %d", got)
	}
}

func TestScheduleTimeAtBar(t *testing.T) {
	schedule := []PlaybackStep{
		{Bar: 0, Ticks: 4 * ticksPerQuarter}, {Bar: 0, Ticks: 4 * ticksPerQuarter},
		{Bar: 1, Ticks: 4 * ticksPerQuarter}, {Bar: 1, Ticks: 4 * ticksPerQuarter},
	}
	if got := ScheduleTimeAtBar(schedule, 0, 120); got != 0 {
		t.Fatalf("bar 1 should start at 0, got %v", got)
	}
	want := 4 * time.Second
	if got := ScheduleTimeAtBar(schedule, 1, 120); got != want {
		t.Fatalf("bar 2 should start at %v, got %v", want, got)
	}
	if got := ScheduleTimeAtBar(schedule, 5, 120); got != 8*time.Second {
		t.Fatalf("past the end should clamp to the total duration, got %v", got)
	}
}
