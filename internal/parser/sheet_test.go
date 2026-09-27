package parser

import (
	"strings"
	"testing"

	"fretboard/internal/model"
)

// wonderwallSheet is a small chord sheet with an intro line, a repeat mark,
// a slash chord and a section label.
const wonderwallSheet = `Title: Wonderwall
Artist: Oasis
Capo 2nd fret

[Intro]
Em7  G  Dsus4  A7sus4   x2

[Verse 1]
Em7          G
Today is gonna be the day
      Dsus4                A7sus4
That they're gonna throw it back to you
C/G
N.C.
`

// sheetChordNames flattens a tab's chords in playing order.
func sheetChordNames(tab *model.Tab) []string {
	var names []string
	for _, sl := range tab.Sheet {
		for _, c := range sl.Chords {
			names = append(names, c.Name)
		}
	}
	return names
}

// TestChordSheetLayout: chord lines become Sheet lines with the lyric line
// under them, chords carry their source column, and every chord is a bar.
func TestChordSheetLayout(t *testing.T) {
	tab, err := Parse(strings.NewReader(wonderwallSheet))
	if err != nil {
		t.Fatal(err)
	}
	if tab.Title != "Wonderwall" || tab.Artist != "Oasis" || tab.Metadata[model.MetaKeyCapo] != "2" {
		t.Fatalf("metadata: title %q artist %q capo %q", tab.Title, tab.Artist, tab.Metadata[model.MetaKeyCapo])
	}
	want := []string{"Em7", "G", "Dsus4", "A7sus4", "Em7", "G", "Dsus4", "A7sus4", "C/G"}
	if got := sheetChordNames(tab); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("chords = %v, want %v", got, want)
	}
	if len(tab.Bars) != len(want) {
		t.Fatalf("%d chords must make %d bars, got %d", len(want), len(want), len(tab.Bars))
	}
	// The lyric line under a chord line is attached, not a text line of its
	// own; the section label stays text.
	if got := tab.Sheet[0].Text; got != "[Intro]" {
		t.Fatalf("first sheet line = %q, want [Intro]", got)
	}
	lyric := -1
	for i, sl := range tab.Sheet {
		if sl.Text == "Today is gonna be the day" {
			lyric = i
		}
	}
	if lyric < 0 {
		t.Fatal("lyrics not attached to their chord line")
	}
	// Em7 sits at column 0 of its line; G at column 13.
	if tab.Sheet[lyric].Chords[0].Col != 0 || tab.Sheet[lyric].Chords[1].Col != 13 {
		t.Fatalf("chord columns = %d,%d, want 0,13", tab.Sheet[lyric].Chords[0].Col, tab.Sheet[lyric].Chords[1].Col)
	}
	// Every chord's bar carries its shape; the first Em7 is the open shape.
	bar := tab.Bars[0]
	if got := segmentFrets(bar); got != "0 2 0 0 0 0" {
		t.Fatalf("Em7 bar = %q, want 0 2 0 0 0 0", got)
	}
}

// segmentFrets renders a bar's one-column shape as fret numbers, x for muted.
func segmentFrets(bar model.Bar) string {
	var parts []string
	for _, sl := range bar.Strings {
		if len(sl.Segments) == 0 {
			parts = append(parts, "x")
			continue
		}
		seg := sl.Segments[0]
		if seg.Char < '0' || seg.Char > '9' {
			parts = append(parts, "x")
			continue
		}
		parts = append(parts, string(seg.Char))
	}
	return strings.Join(parts, " ")
}

// TestChordSheetBarShapes: a barre chord's bar is its barre shape, and a
// chord quality with no shape plays as a rest instead of disappearing.
func TestChordSheetBarShapes(t *testing.T) {
	tab, err := Parse(strings.NewReader("C\nCdim\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(tab.Bars) != 2 {
		t.Fatalf("want 2 bars, got %d", len(tab.Bars))
	}
	if got := segmentFrets(tab.Bars[0]); got != "x 3 5 5 5 3" {
		t.Fatalf("C bar = %q, want x 3 5 5 5 3", got)
	}
	if got := segmentFrets(tab.Bars[1]); got != "x x x x x x" {
		t.Fatalf("Cdim (no shape) bar = %q, want a rest", got)
	}
}

// TestTransposedSheet re-fingers the chords instead of shifting fret numbers,
// so an open string stays open where the new chord wants it.
func TestTransposedSheet(t *testing.T) {
	tab, err := Parse(strings.NewReader("G\nAm7\n"))
	if err != nil {
		t.Fatal(err)
	}
	up := TransposedSheet(tab, 2)
	if got := sheetChordNames(up); strings.Join(got, " ") != "A Bm7" {
		t.Fatalf("transposed chords = %v, want [A Bm7]", got)
	}
	if got := segmentFrets(up.Bars[0]); got != "x 0 2 2 2 0" {
		t.Fatalf("A bar = %q, want x 0 2 2 2 0", got)
	}
	// The original tab is untouched.
	if got := sheetChordNames(tab); strings.Join(got, " ") != "G Am7" {
		t.Fatalf("source chords changed: %v", got)
	}
	if got := segmentFrets(tab.Bars[0]); got != "3 5 5 4 3 3" {
		t.Fatalf("source G bar changed: %q", got)
	}
	// A whole-octave transpose keeps the same shapes (TransposeChord's rule).
	oct := TransposedSheet(tab, 12)
	if got := segmentFrets(oct.Bars[0]); got != "3 5 5 4 3 3" {
		t.Fatalf("octave transposed G bar = %q, want unchanged", got)
	}
}

// TestChordSheetFillerLines: a line of bar lines or repeat marks is text, not
// a chord line, and header lines never leak into the sheet body.
func TestChordSheetFillerLines(t *testing.T) {
	tab, err := Parse(strings.NewReader("Title: T\n\nAm\n| x2 |\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := sheetChordNames(tab); strings.Join(got, " ") != "Am" {
		t.Fatalf("chords = %v, want [Am]", got)
	}
	for _, sl := range tab.Sheet {
		if strings.Contains(sl.Text, "Title:") {
			t.Fatalf("header line leaked into the sheet: %q", sl.Text)
		}
	}
}
