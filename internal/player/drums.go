package player

import (
	"regexp"
	"strings"

	"fretboard/internal/model"
)

// DetectDrumTab reports whether tab is a percussion part rather than a
// fretted-instrument part. The Viewer uses it to disable transpose/tuning
// and label the tab as drums; the SMF writer uses it to route to channel 9.
//
// Heuristic (cheap, deterministic — any single condition fires):
//
//	(a) a string row carries a drum row label (HH, SD, BD, Sd, Bd, H, S, B,
//	    C, T, F, CC, CH, FT, HT — case-insensitive). The ASCII parser does
//	    not retain row labels in parsed bars, and drum tabs fail its tab
//	    classification and are stored as chord sheets, so the labels are
//	    read from the original text in Metadata["raw"].
//
//	(b) x/o hit characters appear in >= 2 distinct string rows. Parsed
//	    bars are scanned for 'x'/'o' segments (the parser keeps 'x'; 'o'
//	    is scanned too so the rule survives parser changes); raw text is
//	    scanned for percussion-shaped rows — only x/o marks, dashes and
//	    pipes — which is where real drum tabs live.
//
// Known tradeoff: a fretted tab that mutes power chords across 2+ strings
// ('x' segments) matches (b); that is the price of x/o detection without
// row labels. A single muted string never matches (rows are counted
// distinctly).
func DetectDrumTab(tab *model.Tab) bool {
	if tab == nil {
		return false
	}
	hitRows := 0
	if raw := tab.Metadata["raw"]; raw != "" {
		for _, line := range strings.Split(raw, "\n") {
			if drumRowLabelRe.MatchString(line) {
				return true
			}
			if xoHitRow(line) {
				hitRows++
			}
		}
	}
	// Distinct string rows with an x/o hit, across all bars.
	hit := make(map[int]bool)
	for _, b := range tab.Bars {
		for s, sl := range b.Strings {
			if rowHasXOHit(sl) {
				hit[s] = true
			}
		}
	}
	return hitRows+len(hit) >= 2
}

// drumRowLabelRe matches a string row whose leading label is a drum row
// name: optional whitespace, the label (longest alternatives first so
// "HT"/"HH"/"SD" win over "H"/"S"), then a bar pipe or dash grid.
var drumRowLabelRe = regexp.MustCompile(`(?i)^\s*(?:HH|SD|BD|CC|CH|FT|HT|H|S|B|C|T|F)\s*[|\-]`)

// xoHitRow reports whether a raw text line is a percussion-style row: only
// x/o hit marks, dashes, pipes and spaces, containing at least one x/o and
// at least one pipe. Lyric and chord lines carry other letters and never
// match; a labeled drum row also fails here (its label is a letter), but
// drumRowLabelRe catches it.
func xoHitRow(line string) bool {
	hasHit, hasPipe := false, false
	for _, r := range line {
		switch r {
		case 'x', 'o':
			hasHit = true
		case '|':
			hasPipe = true
		case '-', ' ', '\t':
		default:
			return false
		}
	}
	return hasPipe && hasHit
}

// rowHasXOHit reports whether a parsed string line carries an x or o hit
// segment (the parser keeps 'x' as a technique char; 'o' is dropped today
// but checked anyway so the rule survives parser changes).
func rowHasXOHit(sl model.StringLine) bool {
	for _, seg := range sl.Segments {
		if seg.Char == 'x' || seg.Char == 'o' {
			return true
		}
	}
	return false
}

// drumPercussionByString maps a string index to a GM percussion note, the
// fallback used when a drum row has no label (labels do not survive the
// ASCII parse): lowest string -> kick, next -> snare, then hats, then toms
// ascending. GM: 36 kick, 38 snare, 42 closed hat, 43 high floor tom,
// 45 low tom, 48 hi-mid tom, 50 high tom.
var drumPercussionByString = [...]int{36, 38, 42, 43, 45, 48, 50}

// drumNoteForIndex maps a string index (0 = lowest) to its GM percussion
// note, clamped to the high tom beyond the table.
func drumNoteForIndex(i int) int {
	if i < 0 || i >= len(drumPercussionByString) {
		return drumPercussionByString[len(drumPercussionByString)-1]
	}
	return drumPercussionByString[i]
}

// drumNoteForLabel maps a drum row label (case-insensitive) to its GM
// percussion note. open selects the open hi-hat (46) — an 'o' hit on an HH
// row — instead of the closed hat (42). Labels come from raw text, which
// the writer cannot see per string row, so the writer maps by string index;
// this table documents the intended mapping and serves callers that do hold
// labels. GM: 36 kick, 38 snare, 42/46 closed/open hat, 49 crash, 51 ride,
// 41/43/45 low-floor/high-floor/high toms.
func drumNoteForLabel(label string, open bool) (int, bool) {
	switch strings.ToUpper(strings.TrimSpace(label)) {
	case "BD", "B":
		return 36, true
	case "SD", "S":
		return 38, true
	case "HH", "H":
		if open {
			return 46, true
		}
		return 42, true
	case "C", "CC", "CH", "CY":
		return 49, true
	case "T":
		return 41, true
	case "FT", "F":
		return 43, true
	case "HT":
		return 45, true
	case "R":
		return 51, true
	}
	return 0, false
}
