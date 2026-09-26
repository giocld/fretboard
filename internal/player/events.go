package player

import (
	"errors"

	"fretboard/internal/model"
)

// Events generates a chronological list of MIDI note events for a tab.
// Pitched tabs follow BuildSchedule step for step, so a MIDI file plays
// exactly what the cursor shows. Drum tabs (x/o hits, which the schedule
// does not see) keep the column walk below, one 16th per column. Bars are
// visited in repeat-aware performance order (RepeatOrder).
func Events(tab *model.Tab, bpm int) ([]Event, error) {
	if tab == nil {
		return nil, errors.New("nil tab")
	}
	if len(tab.Tuning) == 0 {
		return nil, errors.New("tab has no tuning")
	}

	drum := DetectDrumTab(tab)
	if !drum {
		return scheduleEvents(tab), nil
	}
	var events []Event
	currentTick := int64(0)

	for _, b := range RepeatOrder(tab) {
		bar := tab.Bars[b]
		if len(bar.Strings) == 0 {
			continue
		}
		cols := maxColumns(bar.Strings)
		noteCols := NoteColumns(bar)
		if drum {
			// A drum tab's hits are x/o chars, which NoteColumns ignores
			// (it only counts fretted digits); without this the Events
			// loop would visit no columns and the tab would play silence.
			noteCols = drumNoteColumns(bar)
		}
		if len(noteCols) == 0 {
			continue
		}
		for i, col := range noteCols {
			notes, _ := collectNotesAt(tab.SoundingTuning(), bar.Strings, col)
			var drumHits []int
			if drum {
				drumHits = drumHitsAt(bar.Strings, col)
			}
			advance := columnTicks(bar, col, cols, noteCols, i)
			sustain := sustainForNote(bar, col, advance)
			if len(notes) > 0 || len(drumHits) > 0 {
				for _, n := range notes {
					events = append(events, Event{
						Type:   NoteOn,
						Tick:   currentTick,
						String: n.String,
						Fret:   n.Fret,
						Note:   n.Note,
						Vel:    100,
					})
				}
				for _, s := range drumHits {
					// String index carries the drum sound: WriteTabSMF
					// maps it to a GM percussion pitch (channel 9).
					events = append(events, Event{
						Type:   NoteOn,
						Tick:   currentTick,
						String: s,
						Fret:   0,
						Note:   0,
						Vel:    100,
					})
				}
				offTick := currentTick + int64(sustain)
				for _, n := range notes {
					events = append(events, Event{
						Type:   NoteOff,
						Tick:   offTick,
						String: n.String,
						Fret:   n.Fret,
						Note:   n.Note,
						Vel:    0,
					})
				}
				for _, s := range drumHits {
					events = append(events, Event{
						Type:   NoteOff,
						Tick:   offTick,
						String: s,
						Fret:   0,
						Note:   0,
						Vel:    0,
					})
				}
			}
			currentTick += int64(advance)
		}
	}
	return events, nil
}

// drumNoteColumns returns every column holding a fretted note or an x/o
// drum hit, so a drum tab's hits become timing steps. Non-drum tabs keep
// using NoteColumns (x columns are not steps for fretted guitar).
func drumNoteColumns(bar model.Bar) []int {
	cols := maxColumns(bar.Strings)
	var out []int
	for col := range cols {
		if len(notesAtColumn(bar.Strings, col)) > 0 || len(drumHitsAt(bar.Strings, col)) > 0 {
			out = append(out, col)
		}
	}
	return out
}

// drumHitsAt returns the string indices with an x/o hit segment at col.
// The parser keeps 'x' as a segment; 'o' is dropped at parse time, so open
// hi-hat hits do not surface here (they are absent from the parsed bar).
func drumHitsAt(strings []model.StringLine, col int) []int {
	var out []int
	for s, str := range strings {
		for _, seg := range str.Segments {
			if seg.Position == col && (seg.Char == 'x' || seg.Char == 'o') {
				out = append(out, s)
				break
			}
		}
	}
	return out
}

// scheduleEvents turns BuildSchedule into note on/off events.
func scheduleEvents(tab *model.Tab) []Event {
	var events []Event
	tick := int64(0)
	tuning := tab.SoundingTuning()
	for _, step := range BuildSchedule(tab) {
		if !step.Rest {
			notes, _ := collectNotesAt(tuning, tab.Bars[step.Bar].Strings, step.Col)
			for _, n := range notes {
				events = append(events, Event{Type: NoteOn, Tick: tick, String: n.String, Fret: n.Fret, Note: n.Note, Vel: 100})
			}
			off := tick + int64(step.Sustain)
			for _, n := range notes {
				events = append(events, Event{Type: NoteOff, Tick: off, String: n.String, Fret: n.Fret, Note: n.Note, Vel: 0})
			}
		}
		tick += int64(step.Ticks)
	}
	return events
}

type note struct {
	String int
	Fret   int
	Note   int
}

// collectNotesAt returns the unique notes that start at the given column.
// If a fret segment spans more than one column (e.g. fret "12"), it returns
// width > 1 so the caller can advance past it.
func collectNotesAt(tuning model.Tuning, strings []model.StringLine, col int) ([]note, int) {
	if col < 0 || len(strings) == 0 {
		return nil, 0
	}
	var notes []note
	width := 1
	for s, str := range strings {
		for _, seg := range str.Segments {
			if seg.Position == col && seg.Char >= '0' && seg.Char <= '9' {
				midi := tuning.Semitone(s, seg.Value)
				if midi > 0 {
					notes = append(notes, note{String: s, Fret: seg.Value, Note: midi})
				}
				width = max(width, seg.Width)
			}
		}
	}
	seen := make(map[int]bool)
	var out []note
	for _, n := range notes {
		if !seen[n.Note] {
			seen[n.Note] = true
			out = append(out, n)
		}
	}
	return out, width
}

// stepWidth returns the width of any note starting at col, so the playhead
// advances past multi-digit frets.
func stepWidth(strings []model.StringLine, col int) int {
	w := 1
	for _, str := range strings {
		for _, seg := range str.Segments {
			if seg.Position == col {
				w = max(w, seg.Width)
			}
		}
	}
	return w
}

func maxColumns(strings []model.StringLine) int {
	m := 0
	for _, str := range strings {
		for _, seg := range str.Segments {
			m = max(m, seg.Position)
		}
	}
	return m + 1
}
