// Package model holds the core domain types for fretboard: tabs, bars,
// string lines, segments, and tunings.
package model

// Tab is a complete parsed guitar tab.
type Tab struct {
	Title    string
	Artist   string
	Tuning   Tuning
	Bars     []Bar
	Metadata map[string]string
}

// RhythmMark is a parsed rhythm symbol above a tab column (e.g. q, e, h).
type RhythmMark struct {
	Position int
	Ticks    int
}

// Bar is one measure: one StringLine per string in the tuning.
type Bar struct {
	Number      int
	Strings     []StringLine
	Capo        int
	Rhythm      []RhythmMark // optional rhythm row above the strings
	ColumnTicks []int        // optional per-column MIDI tick durations (GP import)
}

// StringLine represents one string in one bar.
type StringLine struct {
	Segments []Segment
}

// Segment is one displayed character in the tab. For multi-digit frets
// (e.g. "12"), Char is the first digit, Value is the full fret number,
// Width is how many columns the segment takes.
type Segment struct {
	Char     rune
	Value    int
	Position int
	Width    int
}
