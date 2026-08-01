package browser

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf8"

	"fretboard/internal/library"
	"fretboard/internal/ui/msgs"
	tea "github.com/charmbracelet/bubbletea"
)

func TestBrowserSortRecentOrdersByLastPlayed(t *testing.T) {
	m := BrowserModel{sortMode: SortRecent}
	rows := []library.TabRow{
		{ID: 1, Title: "A", LastPlayed: "2026-01-01 10:00:00"},
		{ID: 2, Title: "B", LastPlayed: "2026-03-01 10:00:00"},
		{ID: 3, Title: "C"},
	}
	out := m.filterAndSort(rows)
	if len(out) != 3 || out[0].ID != 2 || out[1].ID != 1 || out[2].ID != 3 {
		t.Fatalf("recent sort should prefer last_played, got %+v", out)
	}
}

func TestBrowserSearchEscClearsThenHome(t *testing.T) {
	m := NewBrowserModel(nil)
	m.searchActive = true
	m.searchInput = "layla"
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.searchInput != "" {
		t.Fatal("first esc should clear filter text")
	}
	if !m.searchActive {
		t.Fatal("should stay in search mode after clearing text")
	}
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("second esc should navigate home")
	}
}

func TestBrowserSearchOOpensOnline(t *testing.T) {
	m := NewBrowserModel(nil)
	m.searchActive = true
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	if cmd == nil {
		t.Fatal("o during filter should open online search")
	}
}

func TestFormatRowTuning(t *testing.T) {
	got := formatRowTuning("[40,45,50,55,59,64]")
	if got == "" || got == "[40,45,50,55,59,64]" {
		t.Fatalf("expected readable tuning label, got %q", got)
	}
}

func TestBrowserTabsLoadErrorShowsMessage(t *testing.T) {
	m := NewBrowserModel(nil)
	m, _ = m.Update(msgs.TabsLoadErrorMsg{Err: fmt.Errorf("disk full")})
	if m.errMsg == "" {
		t.Fatal("expected load error message")
	}
	if !m.loaded {
		t.Fatal("should mark loaded after error")
	}
}

func TestBrowserFilterEmptyResetsCursor(t *testing.T) {
	m := NewBrowserModel(nil)
	m.tabs = []library.TabRow{{ID: 1, Title: "A"}, {ID: 2, Title: "B"}}
	m.cursor = 1
	m.searchInput = "zzz"
	m.apply()
	if m.cursor != 0 {
		t.Fatalf("cursor should reset to 0 on empty filter, got %d", m.cursor)
	}
}

func TestBrowserFilterTypingResetsCursor(t *testing.T) {
	m := NewBrowserModel(nil)
	m.tabs = []library.TabRow{{ID: 1, Title: "Alpha"}, {ID: 2, Title: "Beta"}}
	m.searchActive = true
	m.cursor = 1
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'z'}})
	if m.cursor != 0 {
		t.Fatalf("cursor=%d want 0 after filter change", m.cursor)
	}
}

func TestBrowserNormalEscClearsPassiveFilter(t *testing.T) {
	m := NewBrowserModel(nil)
	m.searchInput = "layla"
	m.searchActive = false
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		t.Fatal("first esc should clear passive filter without navigating home")
	}
	if m.searchInput != "" {
		t.Fatal("expected filter cleared")
	}
}

func TestBrowserFilterAllowsJKLetters(t *testing.T) {
	m := NewBrowserModel(nil)
	m.searchActive = true
	m.tabs = []library.TabRow{{ID: 1, Title: "Jack"}, {ID: 2, Title: "Jill"}}
	m.apply()
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.searchInput != "j" {
		t.Fatalf("searchInput = %q, want j while filtering", m.searchInput)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if m.searchInput != "jk" {
		t.Fatalf("searchInput = %q, want jk while filtering", m.searchInput)
	}
}

func TestBrowserJKMoveCursor(t *testing.T) {
	m := NewBrowserModel(nil)
	m.tabs = []library.TabRow{{ID: 1, Title: "A"}, {ID: 2, Title: "B"}, {ID: 3, Title: "C"}}
	m.apply()
	key := func(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }
	m, _ = m.Update(key('j'))
	m, _ = m.Update(key('j'))
	m, _ = m.Update(key('j'))
	if m.cursor != 2 {
		t.Fatalf("cursor=%d, want 2 after three j presses", m.cursor)
	}
	m, _ = m.Update(key('j'))
	if m.cursor != 2 {
		t.Fatalf("cursor=%d, want clamp at 2 when pressing j past end", m.cursor)
	}
	m, _ = m.Update(key('k'))
	m, _ = m.Update(key('k'))
	m, _ = m.Update(key('k'))
	if m.cursor != 0 {
		t.Fatalf("cursor=%d, want clamp at 0 when pressing k past start", m.cursor)
	}
}

func TestBrowserSearchQDoesNotQuit(t *testing.T) {
	m := NewBrowserModel(nil)
	m.searchActive = true
	m.tabs = []library.TabRow{{ID: 1, Title: "Question"}}
	m.apply()
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd != nil {
		t.Fatalf("q while filtering should not quit, got cmd %v", cmd)
	}
	if m.searchInput != "q" {
		t.Fatalf("searchInput=%q, want q while filtering", m.searchInput)
	}
}

func TestBrowserBackspaceRemovesFullRune(t *testing.T) {
	m := NewBrowserModel(nil)
	m.searchActive = true
	m.searchInput = "中"
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if m.searchInput != "" {
		t.Fatalf("backspace should remove the full rune, got %q", m.searchInput)
	}
	if !utf8.ValidString(m.searchInput) {
		t.Fatalf("searchInput must remain valid UTF-8 after backspace, got %q", m.searchInput)
	}
}

func TestBrowserDeleteRequiresConfirmation(t *testing.T) {
	st, err := library.NewStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	f := filepath.Join(t.TempDir(), "one.txt")
	if err := os.WriteFile(f, []byte("Tuning: E Standard\n\ne|0-3-5|\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ImportFile(f); err != nil {
		t.Fatal(err)
	}

	m := NewBrowserModel(st)
	tabs, err := st.List()
	if err != nil {
		t.Fatal(err)
	}
	m.tabs = tabs
	m.loaded = true
	m.apply()
	if len(m.filtered) == 0 {
		t.Fatal("expected a filtered row")
	}
	m.cursor = 0

	press := func(r rune) {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	press('d')
	if m.confirmDelete == nil {
		t.Fatal("d should arm the confirmation prompt, not delete")
	}
	if got, _ := st.List(); len(got) != 1 {
		t.Fatal("tab must not be deleted before confirming")
	}

	press('n')
	if m.confirmDelete != nil {
		t.Fatal("n should cancel the confirmation")
	}
	if got, _ := st.List(); len(got) != 1 {
		t.Fatal("tab must still exist after cancelling")
	}

	press('d')
	press('y')
	if m.confirmDelete != nil {
		t.Fatal("y should confirm and clear the prompt")
	}
	if got, _ := st.List(); len(got) != 0 {
		t.Fatal("tab should be deleted after confirming with y")
	}
}
