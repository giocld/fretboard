package e2e_test

import (
	"strings"
	"testing"

	"fretboard/internal/parser"
	"fretboard/internal/player"
	"fretboard/internal/ui/viewer"
)

// TestChordSheetPlayableE2E walks a real chord sheet through the whole
// pipeline: parse -> one playable 4/4 bar per chord -> schedule -> viewer
// rendering with the lyrics and chord diagrams.
func TestChordSheetPlayableE2E(t *testing.T) {
	tab, err := parser.ParseFile(fixturePath("chords/amazing_grace.txt"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if tab.Metadata["kind"] != "chords" {
		t.Fatalf("kind = %q, want chords", tab.Metadata["kind"])
	}
	chords := 0
	for _, sl := range tab.Sheet {
		chords += len(sl.Chords)
	}
	if chords == 0 || chords != len(tab.Bars) {
		t.Fatalf("sheet has %d chords and %d bars, want one bar per chord", chords, len(tab.Bars))
	}
	schedule := player.BuildSchedule(tab)
	if len(schedule) != len(tab.Bars) {
		t.Fatalf("schedule has %d steps for %d chords", len(schedule), len(tab.Bars))
	}
	for i, step := range schedule {
		if step.Ticks != 4*480 {
			t.Fatalf("chord %d is %d ticks, want one 4/4 measure", i, step.Ticks)
		}
	}

	m := viewer.NewViewerModel()
	m.LoadTab(tab, "amazing_grace.txt", 0)
	view := m.View()
	for _, want := range []string{"Amazing Grace", "Amazing grace, how sweet the sound", "G", "C", "●"} {
		if !strings.Contains(view, want) {
			t.Fatalf("chord-sheet view missing %q:\n%s", want, view)
		}
	}
}
