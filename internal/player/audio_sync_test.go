package player

import (
	"testing"
	"time"

	"fretboard/internal/model"
)

func TestStepIndexAtElapsed(t *testing.T) {
	schedule := []PlaybackStep{
		{Bar: 0, Col: 0, Ticks: 480},
		{Bar: 0, Col: 4, Ticks: 480},
		{Bar: 1, Col: 0, Ticks: 480},
	}
	audio := 3 * time.Second
	if got := StepIndexAtElapsed(schedule, 0, audio); got != 0 {
		t.Fatalf("start: got %d want 0", got)
	}
	if got := StepIndexAtElapsed(schedule, 1500*time.Millisecond, audio); got != 1 {
		t.Fatalf("mid: got %d want 1", got)
	}
	if got := StepIndexAtElapsed(schedule, audio, audio); got != 2 {
		t.Fatalf("end: got %d want 2", got)
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
