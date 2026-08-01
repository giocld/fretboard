### FILE: tuning.go

#### WHAT IT DOES
Represents a guitar tuning (e.g., EADGBE, Drop D, DADGAD) and maps string+fret
to MIDI note numbers for playback.

#### HOW TO THINK ABOUT IT
A tuning is just an ordered list of open string notes, from lowest-pitch string
(index 0) to highest (index 5 for 6-string). "E Standard" is ["E2","A2","D3","G3","B3","E4"].
But you need numbers, not names, to generate audio.

#### STEP-BY-STEP
1. Define `Tuning` as `[]string` (note names like "E2","A2"...).
2. Define a map `noteToMIDI` that maps note+octave strings to MIDI numbers:
   `{"C":0, "C#":1, "D":2, ..., "B":11}` + `octave * 12`.
3. Write `func (t Tuning) MIDINote(stringIndex int, fret int) int`:
   - Look up the open string MIDI number (from tuning name + map).
   - Add the fret number.
   - Return the sum.
4. Define standard tunings as constants/package vars:
   ```
   var Standard = Tuning{"E2","A2","D3","G3","B3","E4"}
   var DropD    = Tuning{"D2","A2","D3","G3","B3","E4"}
   ```
5. Write `func ParseTuning(s string) Tuning` — takes a line like "Tuning: EADGBE"
   and converts to a Tuning slice with octave numbers.

#### GO CONCEPTS
- Custom type: `type Tuning []string`. You can define methods on it.
- Maps as lookup tables.
- `strings` package for parsing tuning strings.

#### GOTCHAS
- String index 0 = thickest string (low E), index 5 = thinnest (high e).
  This is the standard in tab notation but opposite of how you hold the guitar.
- MIDI note 69 = A4 (440 Hz). Middle C is 60. E2 = 40, E4 = 64.
- Check: E2 + 12 semitones = E3 (octave up). So MIDINote(0, 12) should equal MIDINote(1, 7)
  (12th fret on E = 7th fret on A, both = E3).

#### IF STUCK
- "MIDI note number chart" — shows all note-to-number mappings.
- "golang type alias vs type definition" — `type Tuning []string` vs `type Tuning = []string`.
- "golang map literal initialization"

#### SKELETON

var noteToSemitone = map[string]int{
    "C":  0, "C#": 1, "Db": 1, "D": 2, "D#": 3, "Eb": 3,
    "E":  4, "F":  5, "F#": 6, "Gb": 6, "G": 7,
    "G#": 8, "Ab": 8, "A":  9, "A#": 10, "Bb": 10, "B": 11,
}

func parseNoteOctave(s string) int {
    // "E2" → note="E", octave=2 → (octave+1)*12 + noteToSemitone["E"]
    // MIDI formula: (octave + 1) * 12 + semitone
    note := s[:len(s)-1]
    octave, _ := strconv.Atoi(s[len(s)-1:])
    return (octave+1)*12 + noteToSemitone[note]
}

func (t Tuning) Semitone(stringIdx, fret int) int {
    return parseNoteOctave(t[stringIdx]) + fret
}
