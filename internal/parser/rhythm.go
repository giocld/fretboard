package parser

import (
	"strings"
	"unicode"
)

// rhythmSymbols maps common ASCII rhythm markers to MIDI ticks (quarter=480).
var rhythmSymbols = map[rune]int{
	'w': 1920, // whole
	'h': 960,  // half
	'q': 480,  // quarter
	'e': 240,  // eighth
	's': 120,  // sixteenth
	't': 60,   // thirty-second
}

// looksLikeRhythmLine returns true when a line is a rhythm annotation row
// above the tab (e.g. "| q  e  e  q  |").
func looksLikeRhythmLine(line string) bool {
	if !strings.Contains(line, "|") {
		return false
	}
	letters := 0
	digits := 0
	hyphens := 0
	for _, r := range line {
		switch {
		case r == '|' || unicode.IsSpace(r):
		case r >= '0' && r <= '9':
			digits++
		case r == '-':
			hyphens++
		default:
			if _, ok := rhythmSymbols[unicode.ToLower(r)]; ok {
				letters++
			}
		}
	}
	return letters >= 2 && digits == 0 && hyphens < 3
}

// parseRhythmLine extracts rhythm marks aligned to tab columns.
func parseRhythmLine(line string) []struct {
	Position int
	Ticks    int
} {
	var out []struct {
		Position int
		Ticks    int
	}
	pos := 0
	for _, r := range line {
		switch {
		case r == '|' || unicode.IsSpace(r):
			pos++
		default:
			ticks, ok := rhythmSymbols[unicode.ToLower(r)]
			if ok {
				out = append(out, struct {
					Position int
					Ticks    int
				}{Position: pos, Ticks: ticks})
			}
			pos++
		}
	}
	return out
}
