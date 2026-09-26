package player

import (
	"fretboard/internal/model"
	"time"
)

// BuildSchedule returns playback steps with rhythm-aware tick durations,
// bars visited in repeat-aware performance order (RepeatOrder). See
// BarSteps for how each bar is timed.
func BuildSchedule(tab *model.Tab) []PlaybackStep {
	if tab == nil || len(tab.Bars) == 0 {
		return nil
	}
	perBar := BarSteps(tab)
	var steps []PlaybackStep
	for _, b := range RepeatOrder(tab) {
		steps = append(steps, perBar[b]...)
	}
	return steps
}

// RepeatOrder returns the bar indices in performance order, expanding "|:"
// ":|" repeat sections once and resolving 1./2. endings. Sections without
// endings simply play twice; a section with endings plays ending-1 bars on
// the first pass and ending-2 bars on the second. Malformed markers (an
// unpaired ":|" or "|:") fall back to playing the bar once. Count marks
// ("(x12)", Bar.Times) replay their block that many times in total.
func RepeatOrder(tab *model.Tab) []int {
	if tab == nil {
		return nil
	}
	bars := tab.Bars
	endToSection := map[int][2]int{}
	stack := -1
	for i, b := range bars {
		if b.RepeatStart {
			stack = i
		}
		if b.RepeatEnd {
			if stack >= 0 {
				endToSection[i] = [2]int{stack, i}
				stack = -1
			}
		}
	}
	inSection := func(i int) bool {
		for _, s := range endToSection {
			if i >= s[0] && i <= s[1] {
				return true
			}
		}
		return false
	}

	var order []int
	counted := 0
	i := 0
	for i < len(bars) {
		if sec, ok := endToSection[i]; ok {
			// Bar i closes a repeat section that started at sec[0]. The walk
			// already emitted sec[0]..i-1 on the first pass; emit bar i too
			// (unless it is a second ending, which only plays on pass 2),
			// then replay the whole section, skipping first endings.
			if bars[i].Ending != 2 {
				order = append(order, i)
			}
			for j := sec[0]; j <= sec[1]; j++ {
				if bars[j].Ending == 1 {
					continue
				}
				order = append(order, j)
			}
			i++
			continue
		}
		if inSection(i) && bars[i].Ending == 2 {
			i++ // second-ending bar: skip on the first pass
			continue
		}
		order = append(order, i)
		if b := bars[i]; b.Times > 1 && b.TimesFrom >= 0 && b.TimesFrom <= i {
			for n := 1; n < b.Times; n++ {
				for j := b.TimesFrom; j <= i; j++ {
					order = append(order, j)
				}
			}
			counted += (b.Times - 1) * (i - b.TimesFrom + 1)
		}
		i++
	}
	// Safety net against pathological "|:" chains: never expand beyond a
	// sane multiple of the tab size, plus what explicit counts ask for.
	order = order[:min(len(order), len(bars)*3+counted)]
	return order
}

// StepIndexAtPosition returns the first schedule index at or after bar/col.
func StepIndexAtPosition(schedule []PlaybackStep, bar, col int) int {
	if len(schedule) == 0 {
		return 0
	}
	for i, step := range schedule {
		if step.Bar > bar || (step.Bar == bar && step.Col >= col) {
			return i
		}
	}
	return len(schedule) - 1
}

// StepDuration converts MIDI ticks to wall-clock time at the given BPM,
// exactly. Whole milliseconds are not good enough for a clock: rounding each
// step (up or down) by ~0.5 ms drifts about a second over a 4-minute song
// at 137 BPM. Any positive tick count gives a positive duration, so a short
// sustain still schedules its noteoff.
func StepDuration(ticks, bpm int) time.Duration {
	if bpm <= 0 {
		bpm = 120
	}
	if ticks <= 0 {
		ticks = ticksPerQuarter / 4
	}
	return time.Duration(ticks) * time.Minute / time.Duration(bpm*ticksPerQuarter)
}

// ScheduleDurationSeconds returns the schedule's total wall-clock length at
// the given BPM — the expected duration of the song as written.
func ScheduleDurationSeconds(tab *model.Tab, bpm int) float64 {
	if tab == nil {
		return 0
	}
	if bpm <= 0 {
		bpm = 120
	}
	var total int64
	for _, s := range BuildSchedule(tab) {
		total += int64(s.Ticks)
	}
	if total <= 0 {
		return 0
	}
	return float64(total) * 60.0 / float64(bpm) / float64(ticksPerQuarter)
}
