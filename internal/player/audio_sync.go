package player

import (
	"os/exec"
	"sort"
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

// DeriveBPMFromAudio estimates tempo that maps the tab schedule to an audio
// file. audioOffset excludes any calibrated intro from the timing math: the
// musical content of a recording with an intro occupies audioDur - offset.
func DeriveBPMFromAudio(schedule []PlaybackStep, audioDur, audioOffset time.Duration) int {
	ticks := ScheduleTotalTicks(schedule)
	if ticks <= 0 || audioDur <= 0 {
		return DefaultBPM
	}
	musicDur := audioDur - audioOffset
	if musicDur <= 0 {
		musicDur = audioDur
	}
	quarters := float64(ticks) / float64(ticksPerQuarter)
	minutes := musicDur.Minutes()
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

// SyncPoint anchors a bar to an audio time (seconds since playback start),
// Guitar Pro sync-point style.
type SyncPoint struct {
	Bar     int     `json:"bar"`
	Seconds float64 `json:"seconds"`
}

// stepIndexAtBar returns the first schedule step belonging to bar, clamped
// into range.
func stepIndexAtBar(schedule []PlaybackStep, bar int) int {
	for i, s := range schedule {
		if s.Bar >= bar {
			return i
		}
	}
	return len(schedule) - 1
}

// StepIndexAtSyncPoints maps an audio position to the active schedule step
// using per-bar sync anchors. Between anchors the timeline is linearly scaled;
// before the first anchor the cursor sits at step 0; past the last anchor the
// final segment's step rate is extended (so outros keep the cursor moving).
func StepIndexAtSyncPoints(schedule []PlaybackStep, points []SyncPoint, audioSeconds float64, bpm int) int {
	if len(schedule) == 0 {
		return 0
	}
	if len(points) == 0 {
		return StepIndexAtScheduleTime(schedule, time.Duration(audioSeconds*float64(time.Second)), bpm)
	}
	// Sort by seconds (stable, keeps bar order for equal times).
	sorted := append([]SyncPoint(nil), points...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Seconds < sorted[j].Seconds })
	first := sorted[0]
	if audioSeconds <= first.Seconds {
		return 0
	}
	// Ensure bar anchors are ascending too; if not, clamp by time ordering.
	for i := 0; i < len(sorted)-1; i++ {
		cur, next := sorted[i], sorted[i+1]
		if audioSeconds < next.Seconds {
			return segmentStep(schedule, cur, next, audioSeconds, bpm)
		}
	}
	// Past the last anchor: extend the final segment's rate.
	last := sorted[len(sorted)-1]
	if len(sorted) >= 2 {
		prev := sorted[len(sorted)-2]
		return extendLastSegment(schedule, prev, last, audioSeconds, bpm)
	}
	// Single anchor: plain schedule accumulation past the anchor.
	return StepIndexAtScheduleTime(schedule, time.Duration((audioSeconds-last.Seconds)*float64(time.Second)), bpm)
}

func segmentStep(schedule []PlaybackStep, a, b SyncPoint, audioSeconds float64, bpm int) int {
	startStep := stepIndexAtBar(schedule, a.Bar)
	endStep := stepIndexAtBar(schedule, b.Bar)
	if endStep <= startStep {
		endStep = startStep + 1
	}
	span := b.Seconds - a.Seconds
	if span <= 0 {
		return startStep
	}
	f := (audioSeconds - a.Seconds) / span
	if f >= 1 {
		return endStep - 1
	}
	step := startStep + int(f*float64(endStep-startStep))
	if step >= endStep {
		step = endStep - 1
	}
	if step >= len(schedule) {
		step = len(schedule) - 1
	}
	return step
}

func extendLastSegment(schedule []PlaybackStep, a, b SyncPoint, audioSeconds float64, bpm int) int {
	startStep := stepIndexAtBar(schedule, a.Bar)
	endStep := stepIndexAtBar(schedule, b.Bar)
	if endStep <= startStep {
		endStep = startStep + 1
	}
	span := b.Seconds - a.Seconds
	if span <= 0 {
		return endStep
	}
	rate := float64(endStep-startStep) / span
	step := endStep + int((audioSeconds-b.Seconds)*rate)
	if step >= len(schedule) {
		step = len(schedule) - 1
	}
	return step
}

// ScheduleTimeAtBar returns the schedule time (at the given BPM) at which the
// first step of bar starts; past the end it clamps to the total duration.
func ScheduleTimeAtBar(schedule []PlaybackStep, bar int, bpm int) time.Duration {
	total := time.Duration(0)
	for _, s := range schedule {
		if s.Bar >= bar {
			return total
		}
		total += time.Duration(StepDuration(s.Ticks, bpm)) * time.Millisecond
	}
	return total
}
