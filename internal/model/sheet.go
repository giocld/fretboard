package model

// SheetLine is one line of a chord sheet: lyrics, a section label, or any
// other text, with the chords written above it.
type SheetLine struct {
	Text   string
	Chords []SheetChord
}

// SheetChord is a chord name written over column Col of its line. Bar is the
// tab bar (index into Tab.Bars) that plays it.
type SheetChord struct {
	Name string
	Col  int
	Bar  int
}
