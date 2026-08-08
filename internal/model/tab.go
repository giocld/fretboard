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

	// Repeat structure ("|:" ":|" and 1./2. endings): playback visits the
	// section twice, skipping first-ending bars on the second pass and
	// second-ending bars on the first pass.
	RepeatStart bool
	RepeatEnd   bool
	Ending      int // 1 or 2 for first/second endings; 0 = no ending

	// Section names the song part this bar belongs to ("Verse 1",
	// "Chorus", ...), from headers like "[Verse]" or "Chorus:" in the tab.
	Section string
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
