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
