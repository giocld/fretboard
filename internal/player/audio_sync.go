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

// StepIndexAtElapsed maps elapsed audio time to the active schedule step.
func StepIndexAtElapsed(schedule []PlaybackStep, elapsed, audioDur time.Duration) int {
	if len(schedule) == 0 {
		return 0
	}
	if audioDur <= 0 {
		return 0
	}
	if elapsed >= audioDur {
		return len(schedule) - 1
	}
	totalTicks := ScheduleTotalTicks(schedule)
	if totalTicks <= 0 {
		return 0
	}
	progress := float64(elapsed) / float64(audioDur)
	target := int64(float64(totalTicks) * progress)
	var cum int64
	for i, step := range schedule {
		t := int64(step.Ticks)
		if t <= 0 {
			t = ticksPerQuarter / 4
		}
		cum += t
		if cum > target {
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
