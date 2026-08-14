package player

import (
	"errors"

	"fretboard/internal/model"
)

// Events generates a chronological list of MIDI note events for a tab.
// BPM drives the tempo. Note spacing density and optional rhythm markers
// determine each step's duration; when neither is present every column is
// treated as an equal 16th-note step. Bars are visited in repeat-aware
// performance order (RepeatOrder), so "|:" ":" sections and 1./2. endings
// play the way a human reads them, matching BuildSchedule.
func Events(tab *model.Tab, bpm int) ([]Event, error) {
	if tab == nil {
		return nil, errors.New("nil tab")
	}
	if len(tab.Tuning) == 0 {
		return nil, errors.New("tab has no tuning")
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
		if len(noteCols) == 0 {
			continue
		}
		for i, col := range noteCols {
			notes, _ := collectNotesAt(tab.Tuning, bar.Strings, col)
			advance := columnTicks(bar, col, cols, noteCols, i)
			sustain := sustainForNote(bar, col, advance)
			if len(notes) > 0 {
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
			}
			currentTick += int64(advance)
		}
	}
	return events, nil
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
