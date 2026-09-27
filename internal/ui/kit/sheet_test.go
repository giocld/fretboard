package kit

import (
	"strings"
	"testing"

	"fretboard/internal/model"
	"fretboard/internal/parser"
	"github.com/charmbracelet/x/ansi"
)

func sheetFixture(t *testing.T) *model.Tab {
	t.Helper()
	tab, err := parser.Parse(strings.NewReader("Title: T\n\nAm  C\nlyric line\nG\n"))
	if err != nil {
		t.Fatal(err)
	}
	return tab
}

// TestRenderChordSheetLayout: diagrams come first, the lyrics follow with
// chord names above them, and the current chord's line is reported for
// follow-scroll.
func TestRenderChordSheetLayout(t *testing.T) {
	tab := sheetFixture(t)
	content, curLine := RenderChordSheet(tab, &TabCursor{Bar: 1}, 80)
	plain := ansi.Strip(content)
	for _, want := range []string{"Am", "C", "G", "lyric line", "●", "═"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("chord-sheet render missing %q:\n%s", want, plain)
		}
	}
	lines := strings.Split(plain, "\n")
	if curLine >= len(lines) || !strings.Contains(lines[curLine], "C") {
		t.Fatalf("curLine %d does not hold the current chord:\n%s", curLine, plain)
	}
	// Every chord's follow-scroll line holds that chord's name.
	for bar, name := range []string{"Am", "C", "G"} {
		line := ChordSheetBarLine(tab, 80, bar)
		if line >= len(lines) || !strings.Contains(lines[line], name) {
			t.Fatalf("bar %d line %d does not hold %q:\n%s", bar, line, name, plain)
		}
	}
}

// TestRenderTabPlainChordSheet: a chord sheet exports as chords over lyrics,
// never as its synthetic one-column bars.
func TestRenderTabPlainChordSheet(t *testing.T) {
	plain := RenderTabPlain(sheetFixture(t))
	if strings.Contains(plain, "|") {
		t.Fatalf("chord sheet exported as bars:\n%s", plain)
	}
	for _, want := range []string{"Am", "C", "lyric line", "G"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("chord-sheet export missing %q:\n%s", want, plain)
		}
	}
}

// TestChordDiagramDrawsShape: a barre chord is drawn from its own fret window
// with the fret label, open strings are marked, and an unvoiceable chord
// shows only its name.
func TestChordDiagramDrawsShape(t *testing.T) {
	tab := sheetFixture(t)
	// C is an A-shape barre (x35553): drawn from fret 3 with the "3fr" label.
	plain := ansi.Strip(strings.Join(ChordDiagram("C", barShape(tab, 1), false), "\n"))
	for _, want := range []string{"C", "x", "●", "3fr", "│"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("C diagram missing %q:\n%s", want, plain)
		}
	}
	if strings.Contains(plain, "═") {
		t.Fatalf("a barre shape must not draw the nut:\n%s", plain)
	}
	// Am is open (x02210): the nut edge and open-string marks appear.
	plain = ansi.Strip(strings.Join(ChordDiagram("Am", barShape(tab, 0), false), "\n"))
	for _, want := range []string{"o", "═"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("Am diagram missing %q:\n%s", want, plain)
		}
	}
	// A chord with no shape draws its name and nothing else.
	lines := ChordDiagram("Cdim", [6]int{-1, -1, -1, -1, -1, -1}, false)
	if got := strings.TrimSpace(ansi.Strip(strings.Join(lines[1:], "\n"))); got != "" {
		t.Fatalf("unvoiced chord drew a shape: %q", got)
	}
}

// TestChordDiagramKeepsTallShapes: an E-shape sus2 barre spans five frets, so
// the diagram must grow its window instead of dropping the fifth-fret notes;
// a row mixing tall and short diagrams must stay aligned without panicking.
func TestChordDiagramKeepsTallShapes(t *testing.T) {
	for _, tc := range []struct {
		name  string
		shape [6]int
		dots  int
	}{
		{"Fsus2", [6]int{1, 3, 5, 5, 1, 1}, 6},
		{"Gsus2", [6]int{3, 5, 7, 7, 3, 3}, 6},
		{"C", [6]int{-1, 3, 5, 5, 5, 3}, 5},
	} {
		plain := ansi.Strip(strings.Join(ChordDiagram(tc.name, tc.shape, false), "\n"))
		if got := strings.Count(plain, "●"); got != tc.dots {
			t.Fatalf("%s: %d dots, want %d:\n%s", tc.name, got, tc.dots, plain)
		}
	}
	tab, err := parser.Parse(strings.NewReader("Title: T\n\nFsus2  Cdim\nlyric line\n"))
	if err != nil {
		t.Fatal(err)
	}
	content, _ := RenderChordSheet(tab, nil, 40) // two diagrams per row: one tall, one short
	plain := ansi.Strip(content)
	for _, want := range []string{"Fsus2", "Cdim", "●"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("mixed-height diagram row missing %q:\n%s", want, plain)
		}
	}
}

// TestSheetPlainWidthUsesDisplayColumns: a multi-byte chord name (C°) must
// not shift the chords written after it in the plain export.
func TestSheetPlainWidthUsesDisplayColumns(t *testing.T) {
	tab, err := parser.Parse(strings.NewReader("Title: T\n\nC°   G\nlyric line\n"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(sheetChordNames(tab), " ") != "C° G" {
		t.Fatalf("fixture chords = %v", sheetChordNames(tab))
	}
	line, _, _ := strings.Cut(sheetPlain(tab), "\n")
	// G sits at source column 5; the export must keep it there (byte
	// counting put it at 4).
	runes := []rune(line)
	if len(runes) < 6 || runes[5] != 'G' {
		t.Fatalf("G at display col %d, want 5: %q", strings.Index(line, "G"), line)
	}
}

// sheetChordNames flattens a tab's chord names in playing order.
func sheetChordNames(tab *model.Tab) []string {
	var names []string
	for _, sl := range tab.Sheet {
		for _, c := range sl.Chords {
			names = append(names, c.Name)
		}
	}
	return names
}
