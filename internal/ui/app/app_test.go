package app

import (
	"testing"

	"github.com/YOUR_USERNAME/fretboard/internal/ui/msgs"
	tea "github.com/charmbracelet/bubbletea"
)

func TestAppSearchQueryQDoesNotQuit(t *testing.T) {
	a := NewApp()
	a.view = viewSearch
	model, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	a = model.(AppModel)
	if cmd != nil {
		if _, isQuit := cmd().(tea.QuitMsg); isQuit {
			t.Fatal("typing q in search query should not quit")
		}
	}
	if a.search.QueryValue() != "q" {
		t.Fatalf("input value = %q, want %q", a.search.QueryValue(), "q")
	}
}

func TestAppLibraryFilterQDoesNotQuit(t *testing.T) {
	a := NewApp()
	a.view = viewLibrary
	a.library.SetSearchActive(true)
	model, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	a = model.(AppModel)
	if cmd != nil {
		if _, isQuit := cmd().(tea.QuitMsg); isQuit {
			t.Fatal("typing q in library filter should not quit")
		}
	}
	if a.library.FilterValue() != "q" {
		t.Fatalf("searchInput = %q, want %q", a.library.FilterValue(), "q")
	}
}

func TestAppQQuitsWhenNoInputActive(t *testing.T) {
	a := NewApp()
	_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("q on home should quit")
	}
	if _, isQuit := cmd().(tea.QuitMsg); !isQuit {
		t.Fatalf("expected QuitMsg, got %#v", cmd())
	}
}

func TestAppShutdownMsgQuits(t *testing.T) {
	a := NewApp()
	model, cmd := a.Update(msgs.ShutdownMsg{})
	a = model.(AppModel)
	if cmd == nil {
		t.Fatal("ShutdownMsg should return a quit command")
	}
	if _, isQuit := cmd().(tea.QuitMsg); !isQuit {
		t.Fatalf("expected QuitMsg, got %#v", cmd())
	}
	// Shutdown must be idempotent: a second ShutdownMsg still quits cleanly.
	if _, cmd = a.Update(msgs.ShutdownMsg{}); cmd == nil {
		t.Fatal("second ShutdownMsg should still return a quit command")
	}
}

func TestAppQuestionMarkTypesInSearchQuery(t *testing.T) {
	a := NewApp()
	a.view = viewSearch
	model, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	a = model.(AppModel)
	if a.view != viewSearch {
		t.Fatalf("view = %v, want viewSearch", a.view)
	}
	if cmd != nil {
		if _, isQuit := cmd().(tea.QuitMsg); isQuit {
			t.Fatal("typing ? in search query should not quit")
		}
	}
	if a.search.QueryValue() != "?" {
		t.Fatalf("input value = %q, want %q", a.search.QueryValue(), "?")
	}
}
