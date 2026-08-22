package parser

import (
	"regexp"
	"strings"
)

// SheetKind classifies a parsed document as tablature, a chord sheet, or
// neither. The kind is stored in tab metadata under the "kind" key
// ("tab" / "chords"); chord sheets carry no Bars and keep their raw text.
type SheetKind int

const (
	SheetTab SheetKind = iota
	SheetChord
	SheetUnknown
)

// String returns the metadata value for a SheetKind: "tab", "chords", or
// "unknown".
func (k SheetKind) String() string {
	switch k {
	case SheetTab:
		return "tab"
	case SheetChord:
		return "chords"
	default:
		return "unknown"
	}
}

// Metadata keys set by the parser on chord sheets (tab sheets get the
// quality keys from ApplyQuality instead).
const (
	metaKeyKind = "kind" // sheet type: "tab" | "chords"
	metaKeyRaw  = "raw"  // original text, kept for display
)

// Classify decides whether lines look like tablature or a chord sheet.
//
// A sheet counts as tab when at least 30% of its non-empty lines are tab
// rows — a leading string letter (eADGBE) directly followed by a bar pipe,
// a dash, or a fret digit, matching the row pattern ScoreTab uses. Rhythm
// annotation rows ("| q  e  e  q  |") also count as tab evidence: they only
// occur in tablature, so a minimal tab diluted by header/rhythm lines stays
// a tab. Lyric lines, chord names ("Am7 C/G"), and metadata all count
// against tab detection, so a chords-over-lyrics sheet lands below the bar
// and reads as SheetChord. Empty input is SheetUnknown.
func Classify(lines []string) SheetKind {
	nonEmpty, tabRows := 0, 0
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		nonEmpty++
		if tabRowRegex.MatchString(l) || looksLikeRhythmLine(l) || looksLikeBarLineRow(l) || drumRowRegex.MatchString(l) {
			tabRows++
		}
		if looksLikeBarLineRow(l) || drumRowRegex.MatchString(l) {
			tabRows++ // bar-line and drum rows are unambiguous tab evidence; count double
		}
	}
	if nonEmpty == 0 {
		return SheetUnknown
	}
	if tabRows*10 >= nonEmpty*3 { // tabRows/nonEmpty >= 30%
		return SheetTab
	}
	return SheetChord
}

// barLineRowRegex matches a bar-line row lacking the string-letter
// designator: optional leading whitespace, a pipe or repeat colon, then
// only pipes, colons, dashes, fret digits, and ending dots — e.g. the
// plain-text export's "|:0--3|" or "|0--3|". Chord-sheet rows ("| Am | F |")
// and lyrics carry letters or spaces and never match.
var barLineRowRegex = regexp.MustCompile(`^\s*[|:][0-9|\-:.]*$`)

// looksLikeBarLineRow reports whether a line is a bar-line row without a
// string letter, i.e. pure bar grid characters.
func looksLikeBarLineRow(line string) bool {
	return barLineRowRegex.MatchString(line)
}

// single letters (H/S/C/T/F) require the pipe, so lyric lines like
// "C - ..." never count. B is deliberately excluded from the single-letter
// set: it is also a guitar string, and "B|-----|" would otherwise double-
// count a normal 6-string row. Drum labels are not in eADGBE, so without
// this a drum tab would read as a chord sheet. Drum rows are unambiguous
// tab evidence, like bar-line rows.
var drumRowRegex = regexp.MustCompile(`(?i)^\s*(?:HH|SD|BD|CC|CH|FT|HT)\s*[|\-]|^\s*[HSCTF]\s*\|`)
