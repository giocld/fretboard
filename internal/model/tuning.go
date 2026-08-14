package model

import (
	"strconv"
	"strings"
)

// Tuning is the open-string MIDI numbers, indexed by string (0 = lowest-pitched
// by convention, but the order is purely user-defined). The Tuning type makes
// no assumption about the number of strings — 6, 7, 8, or non-monotonic
// configurations (BAGDAD, etc.) all work.
type Tuning []int

// Standard tunings (open-string MIDI numbers, low to high).
var (
	Standard     = Tuning{40, 45, 50, 55, 59, 64}     // E2 A2 D3 G3 B3 E4
	Standard7    = Tuning{35, 40, 45, 50, 55, 59, 64} // B1 E2 A2 D3 G3 B3 E4
	DropD        = Tuning{38, 45, 50, 55, 59, 64}     // D2 A2 D3 G3 B3 E4
	DADGAD       = Tuning{38, 45, 50, 55, 57, 62}     // D2 A2 D3 G3 A3 D4
	OpenG        = Tuning{38, 43, 50, 55, 59, 62}     // D2 G2 D3 G3 B3 D4
	OpenD        = Tuning{38, 45, 50, 54, 57, 62}     // D2 A2 D3 F#3 A3 D4
	HalfStepDown = Tuning{39, 44, 49, 54, 58, 63}     // Eb Standard
	FullStepDown = Tuning{38, 43, 48, 53, 57, 62}     // D Standard
)

var noteToSemitone = map[string]int{
	"C": 0, "B#": 0, "C#": 1, "Db": 1, "D": 2, "D#": 3, "Eb": 3,
	"E": 4, "Fb": 4, "F": 5, "E#": 5, "F#": 6, "Gb": 6, "G": 7,
	"G#": 8, "Ab": 8, "A": 9, "A#": 10, "Bb": 10, "B": 11, "Cb": 11,
}

var noteNames = []string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}

// Semitone returns the MIDI note number for fretting `fret` on `stringIdx`.
// Returns 0 for out-of-range indices.
func (t Tuning) Semitone(stringIdx, fret int) int {
	if stringIdx < 0 || stringIdx >= len(t) {
		return 0
	}
	return t[stringIdx] + fret
}

// NoteName returns the open-string display label, e.g. "E2".
func (t Tuning) NoteName(stringIdx int) string {
	if stringIdx < 0 || stringIdx >= len(t) {
		return ""
	}
	return midiToNoteName(t[stringIdx])
}

// NoteNameAt returns the display label of the note played at fret on
// stringIdx, e.g. "G2" for the 3rd fret of the low E string.
func (t Tuning) NoteNameAt(stringIdx, fret int) string {
	if stringIdx < 0 || stringIdx >= len(t) {
		return ""
	}
	return midiToNoteName(t[stringIdx] + fret)
}

// TransposedTab returns a copy of tab with every fretted note shifted by
// semitones (clamped at fret 0); open strings, rests, rhythm, and metadata
// are preserved. A nil tab or a zero shift returns the input unchanged.
func TransposedTab(tab *Tab, semitones int) *Tab {
	if tab == nil || semitones == 0 {
		return tab
	}
	out := *tab
	out.Bars = make([]Bar, len(tab.Bars))
	for i, b := range tab.Bars {
		nb := b
		nb.Strings = make([]StringLine, len(b.Strings))
		for s, sl := range b.Strings {
			nsl := StringLine{Segments: make([]Segment, len(sl.Segments))}
			for j, seg := range sl.Segments {
				ns := seg
				if seg.Value > 0 {
					ns.Value = seg.Value + semitones
					if ns.Value < 0 {
						ns.Value = 0
					}
					digits := strconv.Itoa(ns.Value)
					ns.Width = len(digits)
				}
				nsl.Segments[j] = ns
			}
			nb.Strings[s] = nsl
		}
		out.Bars[i] = nb
	}
	return &out
}

// Strings returns the number of strings in the tuning.
func (t Tuning) Strings() int { return len(t) }

// Label renders the tuning as a compact display label like "EADGBE".
func (t Tuning) Label() string {
	parts := make([]string, len(t))
	for i, m := range t {
		parts[i] = stripOctave(midiToNoteName(m))
	}
	return strings.Join(parts, "")
}

func midiToNoteName(midi int) string {
	if midi <= 0 {
		return ""
	}
	octave := (midi / 12) - 1
	idx := midi % 12
	return noteNames[idx] + strconv.Itoa(octave)
}

func stripOctave(noteName string) string {
	for i, r := range noteName {
		if r >= '0' && r <= '9' {
			return noteName[:i]
		}
	}
	return noteName
}

// ParseTuning parses a tuning label like "EADGBE" or "BEADGBE" into MIDI
// numbers. The first string is snapped to the lowest sensible guitar MIDI
// for its pitch class; subsequent strings' MIDI numbers are computed from
// the actual pitch-class deltas (so non-monotonic tunings like BAGDAD
// work). For tunings the heuristic can't handle, construct Tuning{...}
// directly with explicit MIDI numbers.
func ParseTuning(s string) Tuning {
	fields := splitTuningFields(s)
	if len(fields) == 0 {
		return Tuning{}
	}
	semis := make([]int, len(fields))
	for i, f := range fields {
		sm, ok := noteToSemitone[f]
		if !ok {
			// Unknown note in this position — bail out.
			return Tuning{}
		}
		semis[i] = sm
	}
	first := lowestGuitarMIDI(semis[0])
	if first == 0 {
		first = 40
	}
	out := make(Tuning, len(semis))
	out[0] = first
	for i := 1; i < len(semis); i++ {
		delta := semis[i] - semis[i-1]
		if delta <= 0 {
			delta += 12
		}
		out[i] = out[i-1] + delta
	}
	return out
}

// lowestGuitarMIDI returns the lowest MIDI in standard guitar range
// (MIDI 35-64) for the given pitch class.
func lowestGuitarMIDI(semi int) int {
	switch semi {
	case 11: // B
		return 35 // B1
	case 4: // E
		return 40 // E2
	case 9: // A
		return 45 // A2
	case 2: // D
		return 38 // D2
	case 7: // G
		return 43 // G2
	case 0: // C
		return 36 // C2
	case 5: // F
		return 41 // F2
	case 3: // Eb
		return 39
	case 10: // Bb
		return 46
	case 8: // Ab
		return 44
	case 1: // Db
		return 37
	case 6: // F#
		return 42
	}
	return 0
}

// NoteLetters pulls just the note sequence from a tuning label like
// "EADGBE" -> "EADGBE", "Eb Standard" -> "Eb".
func NoteLetters(s string) string {
	var out strings.Builder
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'G':
			out.WriteRune(r)
		case r == 'b' || r == '#':
			out.WriteRune(r)
		}
	}
	return out.String()
}

func splitTuningFields(s string) []string {
	var out []string
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'G':
			out = append(out, string(r))
		case r == 'b' || r == '#':
			if len(out) > 0 {
				out[len(out)-1] += string(r)
			}
		}
	}
	return out
}
