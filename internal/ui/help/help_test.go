package help

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestSetSectionFiltersKeymap(t *testing.T) {
	m := NewHelpModel()
	m.SetSection(SectionLibrary)
	view := m.View()
	if !strings.Contains(view, "Library browser") {
		t.Errorf("library help should show the library block, got:\n%s", view)
	}
	if strings.Contains(view, "Tab viewer") {
		t.Errorf("library help must not show the viewer block:\n%s", view)
	}
	if !strings.Contains(view, "Global keys") {
		t.Errorf("every help view should include the global block:\n%s", view)
	}

	m.SetSection(SectionViewer)
	view = m.View()
	if !strings.Contains(view, "Tab viewer") {
		t.Errorf("viewer help should show the viewer block:\n%s", view)
	}
	if strings.Contains(view, "Library browser") {
		t.Errorf("viewer help must not show the library block:\n%s", view)
	}
}

func TestSearchFiltersLines(t *testing.T) {
	m := NewHelpModel()
	m.SetSection(SectionViewer)
	all := m.content()

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if !m.Searching() {
		t.Fatal("/ should start filter input")
	}
	for _, r := range "transpose" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if got := m.FilterValue(); got != "transpose" {
		t.Fatalf("filter = %q, want transpose", got)
	}
	filtered := m.content()
	if !strings.Contains(filtered, "transpose") {
		t.Errorf("filtered content should contain the matching line:\n%s", filtered)
	}
	if strings.Contains(filtered, "metronome") {
		t.Errorf("filtered content should drop non-matching lines:\n%s", filtered)
	}
	if strings.Count(filtered, "\n") >= strings.Count(all, "\n") {
		t.Error("filtering must reduce the line count")
	}

	// Enter keeps the filter; a backspace shortens it.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.Searching() {
		t.Fatal("enter should end filter input")
	}
	if m.FilterValue() != "transpose" {
		t.Fatalf("filter should survive enter, got %q", m.FilterValue())
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if m.FilterValue() != "transpos" {
		t.Fatalf("backspace should shorten the filter, got %q", m.FilterValue())
	}
}

func TestEscWhileSearchingClearsFilterNotScreen(t *testing.T) {
	m := NewHelpModel()
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if m.FilterValue() != "x" {
		t.Fatalf("filter = %q, want x", m.FilterValue())
	}
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		t.Fatal("esc while searching must not close the screen")
	}
	if m.Searching() || m.FilterValue() != "" {
		t.Fatalf("esc should clear the filter, searching=%v filter=%q", m.Searching(), m.FilterValue())
	}
	// A second Esc closes.
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("esc with no filter should close the screen")
	}
}

func TestFilterKeepsBlankLinesForShape(t *testing.T) {
	got := filterLines("a\n\nb", "b")
	if got != "\nb" {
		t.Fatalf("filterLines = %q, want %q", got, "\nb")
	}
}
