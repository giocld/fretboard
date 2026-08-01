package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestAppSearchQueryQDoesNotQuit(t *testing.T) {
	app := NewApp()
	app.view = viewSearch
	model, cmd := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	app = model.(AppModel)
	if cmd != nil {
		if _, isQuit := cmd().(tea.QuitMsg); isQuit {
			t.Fatal("typing q in search query should not quit")
		}
	}
	if app.search.input.Value() != "q" {
		t.Fatalf("input value = %q, want %q", app.search.input.Value(), "q")
	}
}

func TestAppLibraryFilterQDoesNotQuit(t *testing.T) {
	app := NewApp()
	app.view = viewLibrary
	app.library.searchActive = true
	model, cmd := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	app = model.(AppModel)
	if cmd != nil {
		if _, isQuit := cmd().(tea.QuitMsg); isQuit {
			t.Fatal("typing q in library filter should not quit")
		}
	}
	if app.library.searchInput != "q" {
		t.Fatalf("searchInput = %q, want %q", app.library.searchInput, "q")
	}
}

func TestAppQQuitsWhenNoInputActive(t *testing.T) {
	app := NewApp()
	_, cmd := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("q on home should quit")
	}
	if _, isQuit := cmd().(tea.QuitMsg); !isQuit {
		t.Fatalf("expected QuitMsg, got %#v", cmd())
	}
}

func TestAppShutdownMsgQuits(t *testing.T) {
	app := NewApp()
	model, cmd := app.Update(ShutdownMsg{})
	app = model.(AppModel)
	if cmd == nil {
		t.Fatal("ShutdownMsg should return a quit command")
	}
	if _, isQuit := cmd().(tea.QuitMsg); !isQuit {
		t.Fatalf("expected QuitMsg, got %#v", cmd())
	}
	// Shutdown must be idempotent: a second ShutdownMsg still quits cleanly.
	if _, cmd = app.Update(ShutdownMsg{}); cmd == nil {
		t.Fatal("second ShutdownMsg should still return a quit command")
	}
}

func TestAppQuestionMarkTypesInSearchQuery(t *testing.T) {
	app := NewApp()
	app.view = viewSearch
	model, cmd := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	app = model.(AppModel)
	if app.view != viewSearch {
		t.Fatalf("view = %v, want viewSearch", app.view)
	}
	if cmd != nil {
		if _, isQuit := cmd().(tea.QuitMsg); isQuit {
			t.Fatal("typing ? in search query should not quit")
		}
	}
	if app.search.input.Value() != "?" {
		t.Fatalf("input value = %q, want %q", app.search.input.Value(), "?")
	}
}
