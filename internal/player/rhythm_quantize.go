package player

import (
	"math"
	"sort"

	"fretboard/internal/model"
)

// ponytail: every spacing-only bar is 4/4; read a time signature once the
// parser extracts one.
const (
	ticksPerMeasure = 4 * ticksPerQuarter
	ticksPerSlot    = ticksPerQuarter / 4 // quantize to 16th notes
)

// BarSteps returns each bar's playback steps once, before repeats are
// expanded. Bars with real durations (a rhythm row, Guitar Pro ticks) use
// them as-is; every other bar is timed from its spacing (see
// quantizedSteps). A full staff of dashes is a measure of rest; a note-less
// bar with fewer lines than the tab's other bars is a divider the parser
// picked up, and gets no steps.
func BarSteps(tab *model.Tab) [][]PlaybackStep {
	if tab == nil {
		return nil
	}
	grid := tabGrid(tab)
	fullStaff := 0
	for _, bar := range tab.Bars {
		fullStaff = max(fullStaff, len(bar.Strings))
	}
	perBar := make([][]PlaybackStep, len(tab.Bars))
	for b, bar := range tab.Bars {
		cols := maxColumns(bar.Strings)
		if cols == 0 {
			continue
		}
		noteCols := NoteColumns(bar)
		switch {
		case len(noteCols) == 0 && len(bar.Strings) >= fullStaff:
			perBar[b] = []PlaybackStep{{Bar: b, Ticks: restTicks(bar, cols, grid), Rest: true}}
		case len(noteCols) == 0:
			// divider line: not music
		case hasRhythmData(bar):
			onset := 0
			for i, col := range noteCols {
				ticks := columnTicks(bar, col, cols, noteCols, i)
				perBar[b] = append(perBar[b], PlaybackStep{
					Bar: b, Col: col, ColWidth: stepWidth(bar.Strings, col),
					Ticks: ticks, Sustain: sustainForNote(bar, col, ticks), Onset: onset,
				})
				onset += ticks
			}
		default:
			perBar[b] = quantizedSteps(b, bar, cols, noteCols, grid)
		}
	}
	return perBar
}

func hasRhythmData(bar model.Bar) bool {
	return len(bar.Rhythm) > 0 || len(bar.ColumnTicks) > 0
}

// restTicks is a rest bar's length: its rhythm row's total when it has one,
// else whole measures at the tab's column rate.
func restTicks(bar model.Bar, cols int, grid colGrid) int {
	total := 0
	for _, r := range bar.Rhythm {
		total += r.Ticks
	}
	if total > 0 {
		return total
	}
	if grid.perMeasure <= 0 {
		return ticksPerMeasure
	}
	return max(1, int(math.Round(float64(cols)/float64(grid.perMeasure)))) * ticksPerMeasure
}

// colGrid is how a tab's author spaced notes: unit columns per grid step
// (the most common distance between notes) and perMeasure columns per 4/4
// measure.
type colGrid struct{ unit, perMeasure int }

// ColumnGrid reports how BarSteps reads a tab's spacing: unit columns per
// note step and perMeasure columns per 4/4 measure (0, 0 when no bar is
// timed by spacing).
func ColumnGrid(tab *model.Tab) (unit, perMeasure int) {
	g := tabGrid(tab)
	return g.unit, g.perMeasure
}

// quantizedSteps times a bar whose only rhythm clue is spacing. ASCII spacing
// is typographic ("12h14" is wider than "0h2") and blocks are padded to a
// common width, so counting each column as a 16th gives every bar its own
// tempo and puts the padding after a riff's last note into every repeat.
// Instead the bar lasts as many whole measures as its notes reach into, and
// each note is snapped to the nearest 16th. A bar written out to its end is
// its own timeline; a padded bar uses the tab's column rate.
func quantizedSteps(b int, bar model.Bar, cols int, noteCols []int, grid colGrid) []PlaybackStep {
	lead, end := barExtent(bar, cols, noteCols)
	span := max(end-lead, 1)
	if grid.perMeasure <= 0 {
		grid = colGrid{unit: 1, perMeasure: span}
	}
	// Notes running up to two grid steps past the bar line are a spill (a
	// turnaround typed a little wide), not another measure.
	measures := max(1, int(math.Ceil(float64(span-2*grid.unit)/float64(grid.perMeasure))))
	slotTicks := ticksPerSlot
	slots := measures * ticksPerMeasure / slotTicks
	perCol := float64(slots) / float64(measures*grid.perMeasure)
	if cols-end <= 2*grid.unit {
		// Filled to the closing bar line: the bar runs from its downbeat to
		// the pipe. Authors disagree by a column on whether the last dash
		// belongs to the final note; measuring to the midpoint lands both.
		perCol = float64(slots) / (float64(cols-lead) + 0.5)
	}
	onsets := make([]int, len(noteCols))
	for {
		prev := -1
		for i, col := range noteCols {
			// Notes closer together than the grid would share a slot.
			onsets[i] = max(int(math.Round(float64(col-lead)*perCol)), prev+1)
			prev = onsets[i]
		}
		// Pack a spilled tail into the bar's last slots instead of
		// stretching the whole bar, so the notes before it stay on grid.
		for i := len(onsets) - 1; i >= 0; i-- {
			onsets[i] = min(onsets[i], slots-len(onsets)+i)
		}
		if onsets[0] >= 0 || slotTicks == 1 {
			break
		}
		slots *= 2
		slotTicks /= 2
		perCol *= 2
	}
	var steps []PlaybackStep
	if onsets[0] > 0 {
		steps = append(steps, PlaybackStep{Bar: b, Col: lead, ColWidth: 1, Ticks: onsets[0] * slotTicks, Rest: true})
	}
	for i, col := range noteCols {
		next := slots
		if i+1 < len(onsets) {
			next = onsets[i+1]
		}
		ticks := (next - onsets[i]) * slotTicks
		steps = append(steps, PlaybackStep{
			Bar: b, Col: col, ColWidth: stepWidth(bar.Strings, col),
			Ticks: ticks, Sustain: ticks, Onset: onsets[i] * slotTicks,
		})
	}
	return steps
}

// barExtent returns where a spacing-only bar's music starts (the downbeat)
// and ends (just past its last note). Dashes after the last note are
// padding.
func barExtent(bar model.Bar, cols int, noteCols []int) (lead, end int) {
	last := noteCols[len(noteCols)-1]
	return downbeatCol(bar, cols, noteCols), last + stepWidth(bar.Strings, last)
}

// downbeatCol returns the column heard as beat 1. Bars open with a dash or
// three of padding, so a first note that near the start is on the downbeat;
// a first note further in follows a rest, and the bar starts just after the
// padding's first dash.
func downbeatCol(bar model.Bar, cols int, noteCols []int) int {
	first := cols
	for _, s := range bar.Strings {
		if len(s.Segments) > 0 && s.Segments[0].Position < first {
			first = s.Segments[0].Position
		}
	}
	if noteCols[0]-first <= cols/8 {
		return noteCols[0]
	}
	return first + 1
}

// tabGrid finds the tab's column rate. Authors space notes on a fixed grid,
// so the most common gap between notes -- counted from the end of one note,
// so "12-12" and "0-0" are the same gap -- is one grid step, an 8th or a
// 16th note. Whichever makes a measure closest to the typical bar's musical
// length (downbeat to last note, padding excluded) wins. Deciding once per
// tab keeps every bar on the same scale.
func tabGrid(tab *model.Tab) colGrid {
	var spans []int
	gaps := map[int]int{}
	for _, bar := range tab.Bars {
		noteCols := NoteColumns(bar)
		if hasRhythmData(bar) || len(noteCols) == 0 {
			continue
		}
		lead, end := barExtent(bar, maxColumns(bar.Strings), noteCols)
		spans = append(spans, end-lead)
		for i := 1; i < len(noteCols); i++ {
			gaps[noteCols[i]-noteCols[i-1]-stepWidth(bar.Strings, noteCols[i-1])+1]++
		}
	}
	if len(spans) == 0 {
		return colGrid{}
	}
	unit, best := 2, 0
	for g, n := range gaps {
		if g > 0 && (n > best || (n == best && g < unit)) {
			unit, best = g, n
		}
	}
	sort.Ints(spans)
	typical := spans[len(spans)/2]
	if eighths := unit * 8; abs(typical-eighths) < abs(typical-unit*16) {
		return colGrid{unit: unit, perMeasure: eighths}
	}
	return colGrid{unit: unit, perMeasure: unit * 16}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
