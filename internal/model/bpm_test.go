package model

import "testing"

func TestParseBPMFromText(t *testing.T) {
	if got := ParseBPMFromText("Based on the video. 107 bpm."); got != 107 {
		t.Fatalf("ParseBPMFromText = %d, want 107", got)
	}
}

// TestTransposedTab guards S5.2: fretted notes shift by semitones, clamp at
// fret 0, open strings and structure stay intact, metadata is preserved.
func TestTransposedTab(t *testing.T) {
	tab := &Tab{
		Title:  "X",
		Tuning: Standard,
		Metadata: map[string]string{"capo": "3"},
		Bars: []Bar{{
			Strings: []StringLine{{Segments: []Segment{
				{Char: '0', Value: 0, Position: 0, Width: 1}, // open string
				{Char: '3', Value: 3, Position: 2, Width: 1},
				{Char: '1', Value: 1, Position: 4, Width: 1}, // transposing down clamps at 0
			}}},
		}},
	}
	up := TransposedTab(tab, 2)
	if up == tab {
		t.Fatal("transpose should return a copy")
	}
	segs := up.Bars[0].Strings[0].Segments
	if segs[0].Value != 0 || segs[1].Value != 5 || segs[2].Value != 3 {
		t.Fatalf("shift +2 wrong: %+v", segs)
	}
	if segs[2].Width != 1 {
		t.Fatalf("width should match digits: %+v", segs[2])
	}
	if up.Metadata["capo"] != "3" || up.Title != "X" {
		t.Fatalf("metadata lost: %+v", up)
	}
	// Original untouched.
	if tab.Bars[0].Strings[0].Segments[1].Value != 3 {
		t.Fatal("original tab mutated")
	}

	down := TransposedTab(tab, -2)
	if got := down.Bars[0].Strings[0].Segments[2].Value; got != 0 {
		t.Fatalf("clamp at 0: got %d", got)
	}
	if TransposedTab(nil, 2) != nil || TransposedTab(tab, 0) != tab {
		t.Fatal("nil/zero transpose should pass through")
	}
}

// TestNoteNameAt guards S5.3: fret-to-note naming uses the tuning's
// semitone math (3rd fret on the low E = G2).
func TestNoteNameAt(t *testing.T) {
	if got := Standard.NoteNameAt(0, 3); got != "G2" {
		t.Fatalf("3rd fret low E should be G2, got %q", got)
	}
	if got := Standard.NoteNameAt(5, 0); got != "E4" {
		t.Fatalf("open high e should be E4, got %q", got)
	}
	if got := Standard.NoteNameAt(0, 0); got != "E2" {
		t.Fatalf("open low E should be E2, got %q", got)
	}
	if got := Standard.NoteNameAt(9, 0); got != "" {
		t.Fatalf("out-of-range string should be empty, got %q", got)
	}
}
