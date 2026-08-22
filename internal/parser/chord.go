package parser

import (
	"regexp"
	"strings"
)

// Chord is a parsed chord symbol: root note, quality suffix, and optional
// slash-chord bass. "F#m7/B" parses to Root "F#", Quality "m7", Bass "B";
// a bare major chord has an empty Quality.
type Chord struct {
	Root    string
	Quality string
	Bass    string
}

// noteSemitones maps a note spelling to its pitch class (0 = C). Both
// enharmonic spellings of the black keys map to the same class.
var noteSemitones = map[string]int{
	"C": 0, "C#": 1, "Db": 1,
	"D": 2, "D#": 3, "Eb": 3,
	"E": 4,
	"F": 5, "F#": 6, "Gb": 6,
	"G": 7, "G#": 8, "Ab": 8,
	"A": 9, "A#": 10, "Bb": 10,
	"B": 11,
}

// semitoneNames is the enharmonic spelling TransposeChord emits: sharps
// throughout (C# D# F# G#) with Bb as the single flat, the convention most
// guitar charts use.
var semitoneNames = [12]string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "Bb", "B"}

// chordRegex matches one full chord symbol. The quality alternation is
// ordered longest-first so "maj7" beats "m" and "m7" beats "m". Group order:
// 1 root letter, 2 root accidental, 3 quality, 4 bass letter, 5 bass
// accidental.
var chordRegex = regexp.MustCompile(`(?i)^([a-g])([#b]?)(maj7|m7|maj|sus2|sus4|add9|dim7|dim|aug|\+|7|6|9|5|m)?(?:/([a-g])([#b]?))?$`)

// ParseChord parses a single chord symbol like "Am7", "C/G", or "F#m7/Bb".
// The root and bass are normalized to uppercase spelling ("am7" → "Am7") and
// the "+" shorthand is normalized to "aug". Returns ok=false for anything
// that is not a chord symbol, so callers can treat the input as plain text.
func ParseChord(s string) (Chord, bool) {
	m := chordRegex.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return Chord{}, false
	}
	c := Chord{
		Root:    strings.ToUpper(m[1]) + m[2],
		Quality: m[3],
	}
	if c.Quality == "+" {
		c.Quality = "aug"
	}
	if m[4] != "" {
		c.Bass = strings.ToUpper(m[4]) + m[5]
	}
	return c, true
}

// String renders the chord symbol, e.g. "A", "F#m7", "C/G".
func (c Chord) String() string {
	s := c.Root + c.Quality
	if c.Bass != "" {
		s += "/" + c.Bass
	}
	return s
}

// TransposeChord shifts a chord symbol by semitones (negative allowed) and
// returns the transposed spelling. The root — and the slash bass, if any —
// move on the 12-note chromatic scale; the quality suffix is preserved.
// Unparseable input is returned unchanged, as is a whole-octave transpose
// (which preserves the original spelling).
func TransposeChord(name string, semitones int) string {
	if semitones%12 == 0 {
		return name
	}
	c, ok := ParseChord(name)
	if !ok {
		return name
	}
	idx, ok := noteSemitones[c.Root]
	if !ok {
		return name
	}
	c.Root = semitoneNames[mod12(idx+semitones)]
	if c.Bass != "" {
		if bi, ok := noteSemitones[c.Bass]; ok {
			c.Bass = semitoneNames[mod12(bi+semitones)]
		}
	}
	return c.String()
}

// mod12 reduces n to the range [0, 12), handling negative shifts.
func mod12(n int) int {
	n %= 12
	if n < 0 {
		n += 12
	}
	return n
}

// FretShape returns barre-chord fret positions, low E string first, for an
// inline chord diagram. Roots on the low E string (E F F# G G#) use the
// E-shape barre; every other root uses the A-shape barre on the A string.
// -1 marks a muted string; qualities without a standard barre voicing return
// all -1.
func (c Chord) FretShape() [6]int {
	impossible := [6]int{-1, -1, -1, -1, -1, -1}
	idx, ok := noteSemitones[c.Root]
	if !ok {
		return impossible
	}
	if idx >= 4 && idx <= 8 { // roots E..G#: E-shape barre (F = 133211)
		R := idx - 4 // fret of the root on the low E string
		switch c.Quality {
		case "":
			return [6]int{R, R + 2, R + 2, R + 1, R, R}
		case "m":
			return [6]int{R, R + 2, R + 2, R, R, R} // Fm = 133111
		case "7":
			return [6]int{R, R + 2, R, R + 1, R, R} // F7 = 131211
		case "m7":
			return [6]int{R, R + 2, R, R, R, R} // Fm7 = 131111
		case "maj7":
			if R == 0 {
				return [6]int{0, 2, 1, 1, 0, 0} // Emaj7 open voicing (021100)
			}
			return [6]int{R, R + 2, R + 2, R + 1, R, R - 1} // Fmaj7 = 133210
		case "sus4":
			return [6]int{R, R + 2, R + 2, R + 2, R, R} // Fsus4 = 133311
		case "sus2":
			return [6]int{R, R + 2, R + 4, R + 4, R, R} // Fsus2 = 135511
		case "aug":
			return [6]int{R, R + 2, R + 2, R + 1, R + 1, R} // F+ = 133221
		}
		return impossible
	}
	// Roots A..D#: A-shape barre (C = x35553). R is the fret of the root on
	// the A string: A = 0, A# = 1, B = 2, C = 3, C# = 4, D = 5, D# = 6.
	R := (idx + 3) % 12
	switch c.Quality {
	case "":
		return [6]int{-1, R, R + 2, R + 2, R + 2, R}
	case "m":
		return [6]int{-1, R, R + 2, R + 2, R + 1, R} // Bm = x24432
	case "7":
		return [6]int{-1, R, R + 2, R, R + 2, R} // B7 = x24242
	case "m7":
		return [6]int{-1, R, R + 2, R, R + 1, R} // Bm7 = x24232
	case "maj7":
		return [6]int{-1, R, R + 2, R + 1, R + 2, R} // Bmaj7 = x24342
	case "sus2":
		return [6]int{-1, R, R + 2, R + 2, R, R} // Asus2 = x02200
	case "sus4":
		return [6]int{-1, R, R + 2, R + 2, R + 3, R} // Asus4 = x02230
	}
	return impossible
}
