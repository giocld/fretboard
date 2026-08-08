package app

import (
	"path/filepath"
	"testing"

	"fretboard/internal/config"
	"fretboard/internal/library"
	"fretboard/internal/model"
	"fretboard/internal/ui/msgs"
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

// TestRestoreSessionOpensLastTab guards G4.1: a persisted session reopens
// the tab at the saved cursor bar with the saved settings.
func TestRestoreSessionOpensLastTab(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	st, err := library.NewStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	id, err := st.Import("s.txt", &model.Tab{Title: "Sultans", Artist: "Dire Straits", Tuning: model.Standard,
		Bars: []model.Bar{
			{Number: 1, Strings: []model.StringLine{{Segments: []model.Segment{{Char: '0', Value: 0, Position: 0, Width: 1}}}}},
			{Number: 2, Strings: []model.StringLine{{Segments: []model.Segment{{Char: '3', Value: 3, Position: 0, Width: 1}}}}},
		}})
	if err != nil {
		t.Fatal(err)
	}
	if err := config.SaveSession(config.Session{TabID: id, Bar: 2, BPM: 96, Linear: true}); err != nil {
		t.Fatal(err)
	}

	m := NewAppWithOptions(st, nil, "", nil)
	cmd := m.RestoreSession()
	if cmd == nil {
		t.Fatal("RestoreSession should return a startup command")
	}
	if m.viewer.Tab() == nil {
		t.Fatal("session tab should be loaded")
	}
	if m.viewer.CursorBar() != 1 {
		t.Fatalf("cursor should restore to bar 2 (0-based 1), got %d", m.viewer.CursorBar())
	}
	if m.viewer.BPM() != 96 || !m.viewer.Linear() {
		t.Fatalf("settings not restored: bpm=%d linear=%v", m.viewer.BPM(), m.viewer.Linear())
	}
}

// TestShutdownSavesSession guards G4.1: quitting persists the open tab and
// its cursor position.
func TestShutdownSavesSession(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	st, err := library.NewStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	id, err := st.Import("s.txt", &model.Tab{Title: "Sultans", Artist: "Dire Straits", Tuning: model.Standard,
		Bars: []model.Bar{{Number: 1, Strings: []model.StringLine{{Segments: []model.Segment{{Char: '0', Value: 0, Position: 0, Width: 1}}}}}}})
	if err != nil {
		t.Fatal(err)
	}
	m := NewAppWithOptions(st, nil, "", nil)
	cmd := m.openTab(id)
	_ = cmd
	m.viewer.SetCursorBar(0)
	m.viewer.SetBPM(132)

	m.Shutdown()

	s := config.LoadSession()
	if s.TabID != id || s.BPM != 132 || s.Bar != 1 {
		t.Fatalf("session not saved on shutdown: %+v", s)
	}
}

// TestSettingsScreenRoundTrip guards G6: opening settings from home, changing
// values, and going back applies them live and persists the config.
func TestSettingsScreenRoundTrip(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	a := NewApp()
	a.view = viewHome

	model, _ := a.Update(msgs.HomeSettingsMsg{})
	a = model.(AppModel)
	if a.view != viewSettings {
		t.Fatalf("HomeSettingsMsg should open settings, view=%d", a.view)
	}

	// Strict toggle, volume down twice, strict on again, theme cycle.
	for _, k := range []string{"j", "enter", "k", "left", "left", "j", "enter", "j", "right"} {
		model, _ := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
		a = model.(AppModel)
	}
	cfg := a.settings.Config()
	if cfg.VolumePercent != 60 {
		t.Fatalf("volume should be 60 after two lefts, got %d", cfg.VolumePercent)
	}
	if !cfg.StrictAudioSelection {
		t.Fatal("strict audio should have toggled back on")
	}

	// Back applies and persists.
	model, cmd := a.Update(msgs.SettingsBackMsg{})
	a = model.(AppModel)
	_ = cmd
	if a.view != viewHome {
		t.Fatalf("back should return home, view=%d", a.view)
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.VolumePercent != 60 || !loaded.StrictAudioSelection {
		t.Fatalf("config not persisted: %+v", loaded)
	}
}

// TestSettingsFromLibraryKey guards the S key in the browser.
func TestSettingsFromLibraryKey(t *testing.T) {
	a := NewApp()
	a.view = viewLibrary
	model, _ := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("S")})
	a = model.(AppModel)
	if a.view != viewSettings {
		t.Fatalf("S in the library should open settings, view=%d", a.view)
	}
}
