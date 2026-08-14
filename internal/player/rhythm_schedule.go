package player

import (
	"fretboard/internal/model"
)

// BuildSchedule returns playback steps with rhythm-aware tick durations.
// Bars are visited in repeat-aware performance order (RepeatOrder), so "|:"
// ":|" sections and 1./2. endings play the way a human reads them: the
// section is repeated once, first-ending bars are skipped on the second pass
// and second-ending bars are skipped on the first.
func BuildSchedule(tab *model.Tab) []PlaybackStep {
	if tab == nil || len(tab.Bars) == 0 {
		return nil
	}
	var steps []PlaybackStep
	for _, b := range RepeatOrder(tab) {
		bar := tab.Bars[b]
		if len(bar.Strings) == 0 {
			continue
		}
		cols := maxColumns(bar.Strings)
		if cols == 0 {
			continue
		}
		noteCols := NoteColumns(bar)
		if len(noteCols) == 0 {
			// Rest bar: emit one step with the bar's duration so the clock,
			// the metronome, and the audio-sync mapping all stay correct —
			// without it the cursor teleports over rests and the playhead
			// runs ahead of the music.
			steps = append(steps, PlaybackStep{
				Bar:   b,
				Col:   0,
				Ticks: restBarTicks(bar, cols),
				Rest:  true,
			})
			continue
		}
		for i, col := range noteCols {
			width := stepWidth(bar.Strings, col)
			ticks := columnTicks(bar, col, cols, noteCols, i)
			steps = append(steps, PlaybackStep{
				Bar:      b,
				Col:      col,
				ColWidth: width,
				Ticks:    ticks,
				Sustain:  sustainForNote(bar, col, ticks),
			})
		}
	}
	return steps
}

// RepeatOrder returns the bar indices in performance order, expanding "|:"
// ":|" repeat sections once and resolving 1./2. endings. Sections without
// endings simply play twice; a section with endings plays ending-1 bars on
// the first pass and ending-2 bars on the second. Malformed markers (an
// unpaired ":|" or "|:") fall back to playing the bar once.
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
		i++
	}
	// Safety net against pathological marker chains: never expand beyond a
	// sane multiple of the tab size.
	order = order[:min(len(order), len(bars)*3)]
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
// rounding up so a sub-millisecond step is never scheduled with 0 ms (which
// used to make notes at high BPM ring forever — the noteoff goroutine was
// skipped for sustainMs <= 0).
func StepDuration(ticks, bpm int) int64 {
	if bpm <= 0 {
		bpm = 120
	}
	if ticks <= 0 {
		ticks = ticksPerQuarter / 4
	}
	num := int64(ticks) * int64(60_000)
	den := int64(bpm) * int64(ticksPerQuarter)
	return (num + den - 1) / den
}

// BeatColumns returns the note columns of a bar that start a quarter-note
// beat, derived from the bar's own column tick durations. The first note of
// the bar is always a beat (accented by the metronome). Rest bars have no
// notes, so their beats come from the rhythm row or from the sixteenth-note
// column grid (every 8 columns = one quarter).
func BeatColumns(bar model.Bar) []int {
	cols := NoteColumns(bar)
	if len(cols) == 0 {
		// Rest bar: rhythm marks on quarter boundaries, else the column
		// grid (8 columns per quarter at the 16th-per-column heuristic).
		if len(bar.Rhythm) > 0 {
			var beats []int
			for _, r := range bar.Rhythm {
				if r.Ticks >= ticksPerQuarter {
					beats = append(beats, r.Position)
				}
			}
			if len(beats) > 0 {
				return beats
			}
		}
		width := maxColumns(bar.Strings)
		var beats []int
		for c := 0; c < width; c += 8 {
			beats = append(beats, c)
		}
		if len(beats) == 0 {
			beats = []int{0}
		}
		return beats
	}
	maxC := maxColumns(bar.Strings)
	var beats []int
	acc := 0
	for i, c := range cols {
		ticks := columnTicks(bar, c, maxC, cols, i)
		if acc%ticksPerQuarter == 0 {
			beats = append(beats, c)
		}
		acc += ticks
	}
	return beats
}

// restBarTicks returns the MIDI tick duration of a rest bar: the rhythm
// row's total when one is present, otherwise the bar's column span at the
// sixteenth-note-per-column heuristic (the same rule note columns use).
func restBarTicks(bar model.Bar, cols int) int {
	total := 0
	for _, r := range bar.Rhythm {
		total += r.Ticks
	}
	if total > 0 {
		return total
	}
	ticks := cols * (ticksPerQuarter / 4)
	if ticks < 1 {
		ticks = ticksPerQuarter
	}
	return ticks
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
