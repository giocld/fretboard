package player

import (
	"math"
	"sort"

	"fretboard/internal/model"
)

const ticksPerQuarter = 480

// PlaybackStep is one cursor position during playback with its MIDI tick length.
type PlaybackStep struct {
	Bar      int
	Col      int
	ColWidth int
	Ticks    int
	Sustain  int
}

// NoteColumns returns sorted column indices in a bar that contain at least one
// fretted note.
func NoteColumns(bar model.Bar) []int {
	cols := maxColumns(bar.Strings)
	seen := make(map[int]bool)
	for col := 0; col < cols; col++ {
		if len(notesAtColumn(bar.Strings, col)) > 0 {
			seen[col] = true
		}
	}
	out := make([]int, 0, len(seen))
	for col := range seen {
		out = append(out, col)
	}
	sort.Ints(out)
	return out
}

func notesAtColumn(strings []model.StringLine, col int) []note {
	notes, _ := collectNotesAt(model.Standard, strings, col)
	return notes
}

// columnSpacing returns how many display columns elapse before the next note
// column. When rhythm markers are present on the bar they take precedence.
func columnSpacing(bar model.Bar, col, cols int, noteCols []int, idx int) int {
	if len(bar.Rhythm) > 0 {
		if ticks := rhythmTicksAt(bar, col); ticks > 0 {
			step := ticksPerQuarter / 4
			if step <= 0 {
				step = 1
			}
			return int(math.Max(1, math.Round(float64(ticks)/float64(step))))
		}
	}
	if idx+1 < len(noteCols) {
		gap := noteCols[idx+1] - col
		if gap < 1 {
			gap = 1
		}
		return gap
	}
	tail := cols - col
	if tail < 1 {
		tail = 1
	}
	return tail
}

func rhythmTicksAt(bar model.Bar, col int) int {
	for _, r := range bar.Rhythm {
		if r.Position == col && r.Ticks > 0 {
			return r.Ticks
		}
	}
	return 0
}

// rhythmTicksForNote returns the sustain duration for a note column using the
// nearest preceding rhythm mark on the bar. Rhythm rows often sit above the
// string lines, so mark positions rarely match fret columns exactly.
func rhythmTicksForNote(bar model.Bar, col int) int {
	if len(bar.Rhythm) == 0 {
		return 0
	}
	bestPos := -1
	best := 0
	for _, r := range bar.Rhythm {
		if r.Position <= col && r.Position > bestPos && r.Ticks > 0 {
			bestPos = r.Position
			best = r.Ticks
		}
	}
	return best
}

// sustainForNote returns how many ticks a note starting at col should ring:
// the rhythm-derived sustain when a rhythm row is present, clamped to the
// step's advance so notes never bleed past their column.
func sustainForNote(bar model.Bar, col, advance int) int {
	sustain := advance
	if len(bar.Rhythm) > 0 {
		if rt := rhythmTicksForNote(bar, col); rt > 0 {
			sustain = rt
		}
	}
	if sustain > advance {
		sustain = advance
	}
	return sustain
}

// columnTicks returns the MIDI tick duration for a note starting at col.
func columnTicks(bar model.Bar, col, cols int, noteCols []int, idx int) int {
	step := ticksPerQuarter / 4
	if step <= 0 {
		step = 1
	}
	if ticks := rhythmTicksAt(bar, col); ticks > 0 {
		return ticks
	}
	if len(bar.ColumnTicks) > col && bar.ColumnTicks[col] > 0 {
		return bar.ColumnTicks[col]
	}
	return columnSpacing(bar, col, cols, noteCols, idx) * step
}

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
			continue
		}
		for i, col := range noteCols {
			width := stepWidth(bar.Strings, col)
			ticks := columnTicks(bar, col, cols, noteCols, i)
			if ticks < 1 {
				ticks = ticksPerQuarter / 4
			}
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
	if len(order) > len(bars)*3 {
		order = order[:len(bars)*3]
	}
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
