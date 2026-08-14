package viewer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fretboard/internal/model"
	"fretboard/internal/player"
	tea "github.com/charmbracelet/bubbletea"
)

func TestViewerNavigationStopsPlayback(t *testing.T) {
	m := NewViewerModel()
	m.tab = &model.Tab{Title: "T", Bars: []model.Bar{{}, {}}}
	m.playing = true
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.playing {
		t.Fatal("j should stop playback before scrolling")
	}
	if m.cursorBar != 1 {
		t.Fatalf("cursorBar=%d want 1", m.cursorBar)
	}
}

// TestSyncBarKeyGivesFeedbackWhenUnavailable guards US-7: pressing s outside
// audio-synced playback must explain why it can't anchor, instead of silently
// doing nothing (the footer advertises the key unconditionally).
func TestSyncBarKeyGivesFeedbackWhenUnavailable(t *testing.T) {
	m := NewViewerModel()
	m.LoadTab(sampleTab(), "", 0)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	m = updated
	if m.errMsg == "" {
		t.Fatal("s while paused should show a hint, not a silent no-op")
	}
	if len(m.syncPoints) != 0 {
		t.Fatalf("s must not anchor while paused, got %+v", m.syncPoints)
	}

	// Playing via MIDI synth is still not a real recording: hint again.
	m.errMsg = ""
	m.playing = true
	m.audioSync = false
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	m = updated
	if m.errMsg == "" {
		t.Fatal("s during MIDI playback should show a hint")
	}
	if len(m.syncPoints) != 0 {
		t.Fatalf("s must not anchor during MIDI, got %+v", m.syncPoints)
	}

	// A real audio-synced playback anchors as before.
	m.errMsg = ""
	m.audioSync = true
	m.tabID = 7
	m.cursorBar = 2
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	m = updated
	if len(m.syncPoints) != 1 || m.syncPoints[0].Bar != 3 {
		t.Fatalf("s during audio playback should anchor bar 3, got %+v", m.syncPoints)
	}
	if cmd == nil {
		t.Fatal("anchoring should persist tab prefs")
	}
}

// TestSyncBarKeyFeedbackClearsOnEsc guards the errMsg escape hatch used for
// the sync hint: Esc clears the message without navigating away.
func TestSyncBarKeyFeedbackClearsOnEsc(t *testing.T) {
	m := NewViewerModel()
	m.LoadTab(sampleTab(), "", 0)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if m.errMsg == "" {
		t.Fatal("precondition: hint should be set")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.errMsg != "" {
		t.Fatalf("Esc should clear the hint, got %q", m.errMsg)
	}
}

// TestClearSyncPointsKeyReportsEmpty guards the S key when no anchors exist.
func TestClearSyncPointsKeyReportsEmpty(t *testing.T) {
	m := NewViewerModel()
	m.LoadTab(sampleTab(), "", 0)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("S")})
	if m.errMsg == "" {
		t.Fatal("S with no anchors should say so")
	}
}

func TestManualNavDisablesFollow(t *testing.T) {
	m := NewViewerModel()
	m.LoadTab(sampleTab(), "", 0)
	m.follow = true
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if m.follow {
		t.Fatal("manual j navigation should disable follow mode")
	}
}

// TestExportKeyWritesFile guards S4.3 in the viewer: X exports the loaded
// tab to a plain-ASCII file in the working directory.
func TestExportKeyWritesFile(t *testing.T) {
	m := NewViewerModel()
	tab := &model.Tab{Title: "Viewer Export", Artist: "A", Tuning: model.Standard,
		Bars: []model.Bar{{Strings: []model.StringLine{{Segments: []model.Segment{{Char: '0', Value: 0, Position: 0, Width: 1}}}}}}}
	m.LoadTab(tab, "v.txt", 0)

	oldwd, _ := os.Getwd()
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	m, _ = m.Update(key("X"))
	data, err := os.ReadFile(filepath.Join(dir, "Viewer Export.txt"))
	if err != nil {
		t.Fatalf("export file missing: %v (info=%q)", err, m.infoMsg)
	}
	if !strings.Contains(string(data), "Viewer Export") {
		t.Fatalf("export content wrong:\n%s", data)
	}
}

// TestTransposeKeysShiftDisplayAndPlayback guards S5.2: T/Z adjust the
// session transpose, the display tab shifts frets, playback uses the
// transposed tab, and R resets.
func TestTransposeKeysShiftDisplayAndPlayback(t *testing.T) {
	m := NewViewerModel()
	tab := &model.Tab{Title: "X", Artist: "Y", Tuning: model.Standard,
		Bars: []model.Bar{{Strings: []model.StringLine{
			{Segments: []model.Segment{{Char: '3', Value: 3, Position: 0, Width: 1}}},
		}}}}
	m.LoadTab(tab, "x.txt", 0)

	m, _ = m.Update(key("T"))
	m, _ = m.Update(key("T"))
	if m.transpose != 2 {
		t.Fatalf("transpose = %d, want 2", m.transpose)
	}
	display := m.displayTab()
	if display == m.tab {
		t.Fatal("display tab should be a transposed copy")
	}
	if got := display.Bars[0].Strings[0].Segments[0].Value; got != 5 {
		t.Fatalf("display fret = %d, want 5", got)
	}
	// Playback schedule comes from the transposed tab.
	sched := player.BuildSchedule(m.displayTab())
	if len(sched) == 0 {
		t.Fatal("empty schedule from transposed tab")
	}
	// Status row shows the transpose.
	m, _ = m.Update(key("R"))
	if m.transpose != 0 {
		t.Fatalf("R should reset transpose, got %d", m.transpose)
	}
	if m.displayTab() != m.tab {
		t.Fatal("after reset the original tab renders again")
	}
}

// TestSearchInTab guards S5.1: / opens the search, patterns match fret
// digits, n/N cycle matches, Enter jumps and closes.
func TestSearchInTab(t *testing.T) {
	m := NewViewerModel()
	tab := &model.Tab{Title: "X", Tuning: model.Standard, Bars: []model.Bar{
		{Number: 1, Strings: []model.StringLine{{Segments: []model.Segment{
			{Char: '0', Value: 0, Position: 0, Width: 1},
			{Char: '-', Position: 1},
			{Char: '3', Value: 3, Position: 2, Width: 1},
			{Char: '-', Position: 3},
			{Char: '5', Value: 5, Position: 4, Width: 1},
		}}}},
		{Number: 2, Strings: []model.StringLine{{Segments: []model.Segment{
			{Char: '3', Value: 3, Position: 0, Width: 1},
		}}}},
	}}
	m.LoadTab(tab, "x.txt", 0)

	m, _ = m.Update(key("/"))
	if !m.searchActive {
		t.Fatal("/ should open the search box")
	}
	// Type "35": matches bar 1 (digits 035), not bar 2 (3).
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("35")})
	if len(m.searchMatches) != 1 || m.searchMatches[0].bar != 0 {
		t.Fatalf("matches = %+v, want one match in bar 1", m.searchMatches)
	}
	// Type "3": matches both bars; n/N cycle.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	if len(m.searchMatches) != 2 {
		t.Fatalf("two bars contain a 3, got %+v", m.searchMatches)
	}
	m, _ = m.Update(key("n"))
	if m.searchIdx != 1 || m.cursorBar != m.searchMatches[1].bar {
		t.Fatalf("n should move to match 2: idx=%d bar=%d", m.searchIdx, m.cursorBar)
	}
	m, _ = m.Update(key("N"))
	if m.searchIdx != 0 {
		t.Fatalf("N should wrap back to match 1, got %d", m.searchIdx)
	}
	// Bar-number search: "2" jumps to bar 2.
	m, _ = m.Update(key("esc"))
	m, _ = m.Update(key("/"))
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	if len(m.searchMatches) != 1 || m.searchMatches[0].bar != 1 {
		t.Fatalf("bar-number search should match bar 2, got %+v", m.searchMatches)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.searchActive {
		t.Fatal("Enter should close the search")
	}
	if m.cursorBar != 1 {
		t.Fatalf("Enter should jump to bar 2, cursor at %d", m.cursorBar)
	}
}

// TestNoteNamesKey guards S5.3: e toggles the note-name view and the status
// row announces it.
func TestNoteNamesKey(t *testing.T) {
	m := NewViewerModel()
	tab := &model.Tab{Title: "X", Tuning: model.Standard,
		Bars: []model.Bar{{Strings: []model.StringLine{{Segments: []model.Segment{{Char: '3', Value: 3, Position: 0, Width: 1}}}}}}}
	m.LoadTab(tab, "x.txt", 0)
	m, _ = m.Update(key("e"))
	if !m.showNotes {
		t.Fatal("e should enable the note-name view")
	}
	if !strings.Contains(m.View(), "notes") {
		t.Fatalf("status should mention notes:\n%s", m.View())
	}
	m, _ = m.Update(key("e"))
	if m.showNotes {
		t.Fatal("e should toggle notes back off")
	}
}

// TestSearchMatchesSectionNames guards G2.3: typing a section name in the
// in-tab search jumps to that section's first bar.
func TestSearchMatchesSectionNames(t *testing.T) {
	m := NewViewerModel()
	tab := &model.Tab{Title: "X", Tuning: model.Standard, Bars: []model.Bar{
		{Number: 1, Section: "Intro", Strings: []model.StringLine{{Segments: []model.Segment{{Char: '0', Value: 0, Position: 0, Width: 1}}}}},
		{Number: 2, Section: "Intro", Strings: []model.StringLine{{Segments: []model.Segment{{Char: '0', Value: 0, Position: 0, Width: 1}}}}},
		{Number: 3, Section: "Chorus", Strings: []model.StringLine{{Segments: []model.Segment{{Char: '5', Value: 5, Position: 0, Width: 1}}}}},
	}}
	m.LoadTab(tab, "x.txt", 0)

	m, _ = m.Update(key("/"))
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("chorus")})
	if len(m.searchMatches) != 1 || m.searchMatches[0].bar != 2 {
		t.Fatalf("section search should match the chorus first bar, got %+v", m.searchMatches)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.cursorBar != 2 {
		t.Fatalf("Enter should jump to the chorus, cursor at %d", m.cursorBar)
	}
	// Status row shows the current section.
	if !strings.Contains(m.View(), "Chorus") {
		t.Fatalf("status should name the current section:\n%s", m.View())
	}
}

// TestMouseWheelMovesCursor guards G5.1: wheel messages scroll the viewer
// like j/k.
func TestMouseWheelMovesCursor(t *testing.T) {
	m := NewViewerModel()
	tab := &model.Tab{Title: "X", Tuning: model.Standard, Bars: []model.Bar{
		{Number: 1, Strings: []model.StringLine{{Segments: []model.Segment{{Char: '0', Value: 0, Position: 0, Width: 1}}}}},
		{Number: 2, Strings: []model.StringLine{{Segments: []model.Segment{{Char: '0', Value: 0, Position: 0, Width: 1}}}}},
		{Number: 3, Strings: []model.StringLine{{Segments: []model.Segment{{Char: '0', Value: 0, Position: 0, Width: 1}}}}},
	}}
	m.LoadTab(tab, "x.txt", 0)

	m, _ = m.Update(tea.MouseMsg{Type: tea.MouseWheelDown})
	if m.cursorBar != 1 {
		t.Fatalf("wheel down should move to bar 2, got %d", m.cursorBar)
	}
	m, _ = m.Update(tea.MouseMsg{Type: tea.MouseWheelUp})
	if m.cursorBar != 0 {
		t.Fatalf("wheel up should move back to bar 1, got %d", m.cursorBar)
	}
}
