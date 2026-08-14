package player

import (
	"math"

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
	Rest     bool // rest bar: no notes, but the clock and metronome continue
}

// NoteColumns returns sorted column indices in a bar that contain at least one
// fretted note.
func NoteColumns(bar model.Bar) []int {
	cols := maxColumns(bar.Strings)
	out := make([]int, 0, cols)
	for col := 0; col < cols; col++ {
		if len(notesAtColumn(bar.Strings, col)) > 0 {
			out = append(out, col)
		}
	}
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
	if rt := rhythmTicksForNote(bar, col); rt > 0 {
		sustain = rt
	}
	return min(sustain, advance)
}

// columnTicks returns the MIDI tick duration for a note starting at col.
func columnTicks(bar model.Bar, col, cols int, noteCols []int, idx int) int {
	step := ticksPerQuarter / 4
	if ticks := rhythmTicksAt(bar, col); ticks > 0 {
		return ticks
	}
	if len(bar.ColumnTicks) > col && bar.ColumnTicks[col] > 0 {
		return bar.ColumnTicks[col]
	}
	return columnSpacing(bar, col, cols, noteCols, idx) * step
}
