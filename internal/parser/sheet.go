package parser

import (
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"fretboard/internal/model"
)

// buildSheet reads chord-sheet lines into tab.Sheet (text with the chords
// written above it) and tab.Bars (one playable 4/4 bar per chord, fingered
// with the chord's standard shape). A chord sheet has no rhythm, so each
// chord is one measure; sync anchors (s) line the sheet up with a recording.
//
// ponytail: chord shapes are standard-tuning barre/open voicings, so the
// declared tuning is not applied -- a Drop D shape would sound wrong. Add
// per-tuning shapes when a sheet actually needs them.
func buildSheet(lines []string, tab *model.Tab) {
	for i := 0; i < len(lines); i++ {
		line := strings.TrimRight(strings.ReplaceAll(lines[i], "\t", "    "), " \r")
		if i < 30 && sheetHeaderLine(line) {
			continue
		}
		chords := chordsInLine(line)
		if chords == nil {
			tab.Sheet = append(tab.Sheet, model.SheetLine{Text: line})
			continue
		}
		sl := model.SheetLine{}
		// The words sung under these chords are the next line -- unless it
		// is blank, more chords, a header, or a section label.
		if i+1 < len(lines) {
			next := strings.TrimRight(strings.ReplaceAll(lines[i+1], "\t", "    "), " \r")
			if strings.TrimSpace(next) != "" && chordsInLine(next) == nil &&
				!isSectionLabel(next) && !(i+1 < 30 && sheetHeaderLine(next)) {
				sl.Text = next
				i++
			}
		}
		for _, c := range chords {
			c.Bar = len(tab.Bars)
			tab.Bars = append(tab.Bars, chordBar(c.Name, c.Bar+1))
			sl.Chords = append(sl.Chords, c)
		}
		tab.Sheet = append(tab.Sheet, sl)
	}
	// Trim blank air at both ends; blanks between verses stay.
	for len(tab.Sheet) > 0 && blankSheetLine(tab.Sheet[0]) {
		tab.Sheet = tab.Sheet[1:]
	}
	for len(tab.Sheet) > 0 && blankSheetLine(tab.Sheet[len(tab.Sheet)-1]) {
		tab.Sheet = tab.Sheet[:len(tab.Sheet)-1]
	}
}

func blankSheetLine(sl model.SheetLine) bool {
	return len(sl.Chords) == 0 && strings.TrimSpace(sl.Text) == ""
}

// sheetHeaderLine reports whether a line is an explicit "Key: value" header
// that extractMetadata consumes, so it must not render as sheet content.
func sheetHeaderLine(line string) bool {
	return titleRegex.MatchString(line) || artistRegex.MatchString(line) ||
		tuningRegex.MatchString(line) || capoRegex.MatchString(line) || bpmRegex.MatchString(line)
}

// sheetChordName matches chord-like tokens a sheet may write, wider than
// ParseChord's voicing/transpose vocabulary: "A7sus4" and "Cm7b5" belong on
// a chord line even when there is no shape for them yet.
var sheetChordName = regexp.MustCompile(`(?i)^([a-g])([#b]?)((?:maj|min|m|sus|aug|dim|add|no|°|ø|\+|-|[0-9#b])*)(?:/([a-g])([#b]?))?$`)

// sheetChordToken classifies one whitespace token as a chord name (with the
// canonical spelling when ParseChord knows it), or not a chord at all. A
// bare lowercase root is a lyric word ("a", "b"), not a chord -- sheets write
// roots uppercase.
func sheetChordToken(word string) (string, bool) {
	name := strings.Trim(word, "()")
	m := sheetChordName.FindStringSubmatch(name)
	if m == nil {
		return "", false
	}
	if m[3] == "" && m[4] == "" && name != strings.ToUpper(name) {
		return "", false
	}
	if c, parsed := ParseChord(name); parsed {
		return c.String(), true
	}
	return name, true
}

// chordsInLine returns the chords on a chord line, or nil when the line holds
// anything else. Bar lines, "N.C." and repeat marks may sit among chords.
func chordsInLine(line string) []model.SheetChord {
	var chords []model.SheetChord
	runes := []rune(line)
	for i := 0; i < len(runes); {
		if runes[i] == ' ' {
			i++
			continue
		}
		start := i
		for i < len(runes) && runes[i] != ' ' {
			i++
		}
		word := string(runes[start:i])
		name, ok := sheetChordToken(word)
		switch {
		case ok:
			// A "(G)" annotation sits one column right of its bracket.
			offset := 0
			for offset < len(word) && word[offset] == '(' {
				offset++
			}
			col := start + utf8.RuneCountInString(word[:offset])
			chords = append(chords, model.SheetChord{Name: name, Col: col})
		case isSheetFiller(word):
		default:
			return nil
		}
	}
	return chords
}

func isSheetFiller(word string) bool {
	switch strings.ToUpper(strings.Trim(word, "()")) {
	case "|", "||", "-", "/", "%", "*", "..", "...", "N.C.", "N.C", "NC":
		return true
	}
	return repeatCount(word) > 0
}

func isSectionLabel(line string) bool {
	t := strings.TrimSpace(line)
	return strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]")
}

// chordBar is a one-column bar sounding the chord's shape. An unknown chord
// becomes a bar of rest so the sheet keeps its length.
func chordBar(name string, number int) model.Bar {
	shape := [6]int{-1, -1, -1, -1, -1, -1}
	if c, ok := ParseChord(name); ok {
		shape = c.FretShape()
	}
	bar := model.Bar{Number: number, Strings: make([]model.StringLine, len(shape))}
	for s, f := range shape {
		seg := model.Segment{Char: '-', Width: 1}
		if f >= 0 {
			digits := strconv.Itoa(f)
			seg = model.Segment{Char: rune(digits[0]), Value: f, Position: 0, Width: len(digits)}
		}
		bar.Strings[s] = model.StringLine{Segments: []model.Segment{seg}}
	}
	return bar
}

// TransposedSheet rebuilds a chord sheet n semitones up or down, re-fingering
// every chord instead of shifting fret numbers: an open string cannot be
// shifted, and a transposed chord wants a new shape anyway.
func TransposedSheet(tab *model.Tab, semitones int) *model.Tab {
	if tab == nil || semitones == 0 || len(tab.Sheet) == 0 {
		return tab
	}
	out := *tab
	out.Sheet = make([]model.SheetLine, len(tab.Sheet))
	out.Bars = make([]model.Bar, len(tab.Bars))
	for i, sl := range tab.Sheet {
		nsl := model.SheetLine{Text: sl.Text, Chords: make([]model.SheetChord, len(sl.Chords))}
		for j, c := range sl.Chords {
			nc := c
			nc.Name = TransposeChord(c.Name, semitones)
			nsl.Chords[j] = nc
			if c.Bar >= 0 && c.Bar < len(out.Bars) {
				out.Bars[c.Bar] = chordBar(nc.Name, c.Bar+1)
			}
		}
		out.Sheet[i] = nsl
	}
	return &out
}
