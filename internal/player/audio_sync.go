package player

import (
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ScheduleTotalTicks sums MIDI tick lengths across a playback schedule.
func ScheduleTotalTicks(schedule []PlaybackStep) int64 {
	var total int64
	for _, step := range schedule {
		if step.Ticks > 0 {
			total += int64(step.Ticks)
		}
	}
	return total
}

// ScheduleMIDIDuration estimates wall-clock length at the given BPM.
func ScheduleMIDIDuration(schedule []PlaybackStep, bpm int) time.Duration {
	if bpm <= 0 {
		bpm = DefaultBPM
	}
	ms := ScheduleTotalTicks(schedule) * int64(60_000) / int64(bpm) / int64(ticksPerQuarter)
	if ms < 0 {
		ms = 0
	}
	return time.Duration(ms) * time.Millisecond
}

// DeriveBPMFromAudio estimates tempo that maps the tab schedule to an audio file.
func DeriveBPMFromAudio(schedule []PlaybackStep, audioDur time.Duration) int {
	ticks := ScheduleTotalTicks(schedule)
	if ticks <= 0 || audioDur <= 0 {
		return DefaultBPM
	}
	quarters := float64(ticks) / float64(ticksPerQuarter)
	minutes := audioDur.Minutes()
	if minutes <= 0 {
		return DefaultBPM
	}
	return ClampBPM(int(quarters/minutes*60 + 0.5))
}

// StepIndexAtScheduleTime maps music time (audio elapsed minus the calibrated
// audio offset) to the active schedule step, using the tab's own rhythm
// durations at the given BPM. Unlike a linear mapping across the whole audio
// file, this follows the tab's internal timing, so tempo changes and varied
// note densities keep the cursor on the right note.
func StepIndexAtScheduleTime(schedule []PlaybackStep, musicTime time.Duration, bpm int) int {
	if len(schedule) == 0 || musicTime <= 0 {
		return 0
	}
	var cum time.Duration
	for i, step := range schedule {
		cum += time.Duration(StepDuration(step.Ticks, bpm)) * time.Millisecond
		if cum > musicTime {
			return i
		}
	}
	return len(schedule) - 1
}

// ProbeDuration returns the length of an audio file using ffprobe.
func ProbeDuration(path string) (time.Duration, error) {
	if path == "" {
		return 0, nil
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		return 0, err
	}
	out, err := exec.Command("ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "default=noprint_wrappers=1:nokey=1", path).Output()
	if err != nil {
		return 0, err
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return 0, nil
	}
	sec, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	if sec <= 0 {
		return 0, nil
	}
	return time.Duration(sec * float64(time.Second)), nil
}
