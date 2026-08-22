package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fretboard/internal/model"
)

// chordFixtureDir resolves to tests/fixtures/chords relative to the package
// dir; Go tests run with the package directory as the working directory.
const chordFixtureDir = "../../tests/fixtures/chords"

// TestClassifyFixtures pins the 10 realistic chord sheets to SheetChord and
// the existing tab fixture to SheetTab — the acceptance guard that none of
// the pre-existing tab fixtures flip classification.
func TestClassifyFixtures(t *testing.T) {
	entries, err := os.ReadDir(chordFixtureDir)
	if err != nil {
		t.Fatalf("read chord fixtures: %v", err)
	}
	if len(entries) != 10 {
		t.Fatalf("expected 10 chord fixtures, got %d", len(entries))
	}
	for _, e := range entries {
		path := filepath.Join(chordFixtureDir, e.Name())
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		lines := strings.Split(string(b), "\n")
		if got := Classify(lines); got != SheetChord {
			t.Errorf("Classify(%s) = %v, want SheetChord", path, got)
		}
	}

	// The one pre-existing tab fixture must stay a tab sheet.
	b, err := os.ReadFile("../../tests/fixtures/sultans.txt")
	if err != nil {
		t.Fatalf("read sultans fixture: %v", err)
	}
	if got := Classify(strings.Split(string(b), "\n")); got != SheetTab {
		t.Errorf("Classify(sultans.txt) = %v, want SheetTab", got)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		name  string
		lines []string
		want  SheetKind
	}{
		{
			name: "full tab",
			lines: []string{
				"Title: T", "Tuning: E Standard",
				"e|--0--|", "B|-----|", "G|-----|", "D|-----|", "A|-----|", "E|-----|",
			},
			want: SheetTab, // 6 of 8 non-empty lines are tab rows
		},
		{
			name: "chords over lyrics",
			lines: []string{
				"Title: S",
				"Am  F  C  G",
				"some lyric line",
				"F  C  G  Am",
				"another lyric line",
			},
			want: SheetChord, // chord names and lyrics never match tab rows
		},
		{
			name:  "pure lyrics",
			lines: []string{"first line of a song", "second line", "third line"},
			want:  SheetChord,
		},
		{
			name: "exactly 30 percent tab rows",
			lines: []string{
				"e|--0--|", "B|-----|", "G|-----|",
				"a", "b", "c", "d", "e", "f", "g",
			},
			want: SheetTab, // 3 of 10: threshold is < 30% for chords
		},
		{
			name: "below 30 percent tab rows",
			lines: []string{
				"e|--0--|", "B|-----|",
				"a", "b", "c", "d", "e", "f", "g", "h",
			},
			want: SheetChord, // 2 of 10
		},
		{
			name:  "empty input",
			lines: nil,
			want:  SheetUnknown,
		},
		{
			name:  "blank lines only",
			lines: []string{"", "  ", ""},
			want:  SheetUnknown,
		},
		{
			// A power-chord line like "E5" matches the tab-row pattern by the
			// letter-then-digit rule, so a dense power-chord sheet reads as a
			// tab. Documented spec behavior, not a chord-sheet gap.
			name: "dense power chords read as tab",
			lines: []string{
				"Title: P",
				"E5  A5  B5",
				"E5  A5  B5",
				"E5  A5  B5",
			},
			want: SheetTab, // 3 of 4 non-empty lines
		},
	}
	for _, tc := range cases {
		if got := Classify(tc.lines); got != tc.want {
			t.Errorf("%s: Classify = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestSheetKindString(t *testing.T) {
	if SheetTab.String() != "tab" || SheetChord.String() != "chords" || SheetUnknown.String() != "unknown" {
		t.Fatalf("unexpected String() values: %q %q %q", SheetTab, SheetChord, SheetUnknown)
	}
}

func TestParseChord(t *testing.T) {
	cases := []struct {
		in   string
		want Chord
		ok   bool
	}{
		{"A", Chord{Root: "A"}, true},
		{"Am", Chord{Root: "A", Quality: "m"}, true},
		{"Am7", Chord{Root: "A", Quality: "m7"}, true},
		{"Cmaj7", Chord{Root: "C", Quality: "maj7"}, true},
		{"Cmaj", Chord{Root: "C", Quality: "maj"}, true},
		{"C7", Chord{Root: "C", Quality: "7"}, true},
		{"C6", Chord{Root: "C", Quality: "6"}, true},
		{"C9", Chord{Root: "C", Quality: "9"}, true},
		{"C5", Chord{Root: "C", Quality: "5"}, true},
		{"Csus2", Chord{Root: "C", Quality: "sus2"}, true},
		{"Csus4", Chord{Root: "C", Quality: "sus4"}, true},
		{"Cadd9", Chord{Root: "C", Quality: "add9"}, true},
		{"Caug", Chord{Root: "C", Quality: "aug"}, true},
		{"C+", Chord{Root: "C", Quality: "aug"}, true}, // "+" normalizes to aug
		{"Cdim", Chord{Root: "C", Quality: "dim"}, true},
		{"Cdim7", Chord{Root: "C", Quality: "dim7"}, true},
		{"F#", Chord{Root: "F#"}, true},
		{"Bb", Chord{Root: "Bb"}, true},
		{"F#m7", Chord{Root: "F#", Quality: "m7"}, true},
		{"C/G", Chord{Root: "C", Bass: "G"}, true},
		{"Am7/G", Chord{Root: "A", Quality: "m7", Bass: "G"}, true},
		{"F#m7/Bb", Chord{Root: "F#", Quality: "m7", Bass: "Bb"}, true},
		{"D/F#", Chord{Root: "D", Bass: "F#"}, true},
		{"b", Chord{Root: "B"}, true}, // bare root = major chord
		{"H", Chord{}, false},
		{"Cm7b5", Chord{}, false}, // not in the supported quality set
		{"Am7/G/B", Chord{}, false},
		{"N.C.", Chord{}, false},
		{"", Chord{}, false},
	}
	for _, tc := range cases {
		got, ok := ParseChord(tc.in)
		if ok != tc.ok {
			t.Errorf("ParseChord(%q) ok = %v, want %v", tc.in, ok, tc.ok)
			continue
		}
		if ok && got != tc.want {
			t.Errorf("ParseChord(%q) = %+v, want %+v", tc.in, got, tc.want)
		}
	}
}

func TestChordString(t *testing.T) {
	cases := []struct {
		c    Chord
		want string
	}{
		{Chord{Root: "A"}, "A"},
		{Chord{Root: "A", Quality: "m7"}, "Am7"},
		{Chord{Root: "F#", Quality: "m7"}, "F#m7"},
		{Chord{Root: "C", Bass: "G"}, "C/G"},
		{Chord{Root: "F#", Quality: "m7", Bass: "Bb"}, "F#m7/Bb"},
	}
	for _, tc := range cases {
		if got := tc.c.String(); got != tc.want {
			t.Errorf("(%+v).String() = %q, want %q", tc.c, got, tc.want)
		}
	}
}

func TestTransposeChord(t *testing.T) {
	cases := []struct {
		in   string
		semi int
		want string
	}{
		// Whole octaves and zero keep the input spelling verbatim.
		{"C", 0, "C"},
		{"C", 12, "C"},
		{"Eb", 12, "Eb"},
		// Basic chromatic shifts, sharps preferred.
		{"C", 1, "C#"},
		{"C", 2, "D"},
		{"C", 3, "D#"},
		{"E", 1, "F"},
		{"D", 3, "F"},
		{"G", 7, "D"},
		{"Eb", 2, "F"},
		// Negative semitones.
		{"C", -1, "B"},
		{"C", -2, "Bb"},
		{"C", -3, "A"},
		{"Am7", -3, "F#m7"},
		// Enharmonic spelling: index 10 renders as Bb, never A#.
		{"F#m7", 4, "Bbm7"},
		{"C", -2, "Bb"},
		{"Bb", 2, "C"},
		// Quality suffixes are preserved; the root alone moves.
		{"G7", 5, "C7"},
		{"Asus4", 2, "Bsus4"},
		{"Cadd9", -5, "Gadd9"},
		{"C#m7", 3, "Em7"},
		// Slash chords transpose the bass too.
		{"C/G", 2, "D/A"},
		{"D/F#", 1, "D#/G"},
		{"F#m7/Bb", -1, "Fm7/A"},
		// Unparseable input is returned unchanged.
		{"not a chord", 3, "not a chord"},
		{"N.C.", 2, "N.C."},
		{"", 3, ""},
	}
	for _, tc := range cases {
		if got := TransposeChord(tc.in, tc.semi); got != tc.want {
			t.Errorf("TransposeChord(%q, %d) = %q, want %q", tc.in, tc.semi, got, tc.want)
		}
	}
}

func TestFretShape(t *testing.T) {
	cases := []struct {
		chord string
		want  [6]int
	}{
		// E-shape barres (roots on the low E string).
		{"E", [6]int{0, 2, 2, 1, 0, 0}},
		{"Em", [6]int{0, 2, 2, 0, 0, 0}},
		{"Em7", [6]int{0, 2, 0, 0, 0, 0}},
		{"Emaj7", [6]int{0, 2, 1, 1, 0, 0}},
		{"F", [6]int{1, 3, 3, 2, 1, 1}},
		{"Fm", [6]int{1, 3, 3, 1, 1, 1}},
		{"F7", [6]int{1, 3, 1, 2, 1, 1}},
		{"Fm7", [6]int{1, 3, 1, 1, 1, 1}},
		{"Fmaj7", [6]int{1, 3, 3, 2, 1, 0}},
		{"Fsus4", [6]int{1, 3, 3, 3, 1, 1}},
		{"Fsus2", [6]int{1, 3, 5, 5, 1, 1}},
		{"Faug", [6]int{1, 3, 3, 2, 2, 1}},
		{"G", [6]int{3, 5, 5, 4, 3, 3}},
		{"Ab", [6]int{4, 6, 6, 5, 4, 4}},
		// A-shape barres (roots on the A string).
		{"A", [6]int{-1, 0, 2, 2, 2, 0}},
		{"Am", [6]int{-1, 0, 2, 2, 1, 0}},
		{"Amaj7", [6]int{-1, 0, 2, 1, 2, 0}},
		{"Asus2", [6]int{-1, 0, 2, 2, 0, 0}},
		{"Asus4", [6]int{-1, 0, 2, 2, 3, 0}},
		{"Bb", [6]int{-1, 1, 3, 3, 3, 1}},
		{"B", [6]int{-1, 2, 4, 4, 4, 2}},
		{"B7", [6]int{-1, 2, 4, 2, 4, 2}},
		{"Bm", [6]int{-1, 2, 4, 4, 3, 2}},
		{"Bm7", [6]int{-1, 2, 4, 2, 3, 2}},
		{"C", [6]int{-1, 3, 5, 5, 5, 3}},
		{"D", [6]int{-1, 5, 7, 7, 7, 5}},
		{"Dm", [6]int{-1, 5, 7, 7, 6, 5}},
		// Qualities without a standard barre voicing are impossible.
		{"Cdim", [6]int{-1, -1, -1, -1, -1, -1}},
		{"Cdim7", [6]int{-1, -1, -1, -1, -1, -1}},
		{"Cadd9", [6]int{-1, -1, -1, -1, -1, -1}},
		{"C9", [6]int{-1, -1, -1, -1, -1, -1}},
		{"C6", [6]int{-1, -1, -1, -1, -1, -1}},
		{"Caug", [6]int{-1, -1, -1, -1, -1, -1}},
		{"N.C.", [6]int{-1, -1, -1, -1, -1, -1}},
	}
	for _, tc := range cases {
		c, _ := ParseChord(tc.chord)
		if got := c.FretShape(); got != tc.want {
			t.Errorf("(%q).FretShape() = %v, want %v", tc.chord, got, tc.want)
		}
	}
}

// TestParseChordSheetHook parses a real fixture end-to-end: chord sheets must
// carry kind/quality_timing/raw metadata, zero bars, and normal title/artist
// extraction.
func TestParseChordSheetHook(t *testing.T) {
	tab, err := ParseFile(filepath.Join(chordFixtureDir, "amazing_grace.txt"))
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	if len(tab.Bars) != 0 {
		t.Fatalf("chord sheet parsed %d bars, want 0", len(tab.Bars))
	}
	if got := tab.Metadata["kind"]; got != "chords" {
		t.Errorf("metadata[kind] = %q, want %q", got, "chords")
	}
	if got := tab.Metadata["quality_timing"]; got != "n/a" {
		t.Errorf("metadata[quality_timing] = %q, want %q", got, "n/a")
	}
	if raw := tab.Metadata["raw"]; !strings.Contains(raw, "Amazing grace, how sweet the sound") {
		t.Errorf("metadata[raw] missing the lyric text: %q", raw)
	}
	if tab.Title != "Amazing Grace" || tab.Artist != "Traditional" {
		t.Errorf("title/artist = %q/%q, want Amazing Grace/Traditional", tab.Title, tab.Artist)
	}
}

// TestParseChordSheetMetadata pins that header extras (capo, bpm) still land
// on chord sheets via the shared metadata extractor.
func TestParseChordSheetMetadata(t *testing.T) {
	tab, err := ParseFile(filepath.Join(chordFixtureDir, "greensleeves.txt"))
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	if got := tab.Metadata[model.MetaKeyCapo]; got != "3" {
		t.Errorf("metadata[capo] = %q, want 3", got)
	}

	tab, err = ParseFile(filepath.Join(chordFixtureDir, "neon_skyline.txt"))
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	if got := tab.Metadata[model.MetaKeyBPM]; got != "118" {
		t.Errorf("metadata[bpm] = %q, want 118", got)
	}
	if tab.Metadata[model.MetaKeyTitle] != "Neon Skyline" {
		t.Errorf("metadata[title] = %q, want Neon Skyline", tab.Metadata[model.MetaKeyTitle])
	}
}

// TestParseTabSheetQuality pins that the Parse hook still runs the tab
// pipeline for tab sheets and lands quality metadata at parse time.
func TestParseTabSheetQuality(t *testing.T) {
	tab, err := ParseFile("../../tests/fixtures/sultans.txt")
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	if len(tab.Bars) == 0 {
		t.Fatal("tab sheet parsed 0 bars")
	}
	if got := tab.Metadata["kind"]; got != "" {
		t.Errorf("tab sheet metadata[kind] = %q, want absent", got)
	}
	if got := tab.Metadata["quality"]; got == "" {
		t.Error("tab sheet missing quality metadata")
	}
	qt := tab.Metadata["quality_timing"]
	if qt == "" || qt == "n/a" {
		t.Errorf("tab sheet quality_timing = %q, want a timing word", qt)
	}
}
