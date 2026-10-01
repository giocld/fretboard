// Package player turns a parsed model.Tab into MIDI events and a Standard
// MIDI File (SMF). It deliberately keeps the audio output external: either
// write a .mid file to /tmp and shell out to fluidsynth, or produce events
// for real-time playback.
package player

// EventType is a MIDI event category.
type EventType int

const (
	NoteOn EventType = iota
	NoteOff
	// ProgramChange carries the GM program number in Note (0-127).
	ProgramChange
)

// Event is a single MIDI note event with an absolute tick time.
// String is the original string index (0 = lowest), Fret is the fret number
// that produced the note. Ch overrides the output MIDI channel (0 = default:
// channel 0, or the GM percussion channel for drum tabs).
type Event struct {
	Type   EventType
	Tick   int64
	String int
	Fret   int
	Note   int
	Vel    int
	Ch     int
}

// CursorPlayMsg is a tick message for the TUI.
type CursorPlayMsg struct {
	Bar int
	Col int
}
