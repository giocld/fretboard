package browser

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"fretboard/internal/library"
	"fretboard/internal/model"
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

// TestFormatRowTuningEmptyGuards against tabs without a tuning rendering the
// literal strings "null" or "[]" in the library list (and polluting fuzzy
// search with the text "null").
func TestFormatRowTuningEmpty(t *testing.T) {
	for _, raw := range []string{"null", "[]"} {
		if got := formatRowTuning(raw); got != "" {
			t.Fatalf("formatRowTuning(%q) = %q, want empty", raw, got)
		}
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

// previewStore returns a browser bound to a store holding two tabs.
func previewStore(t *testing.T) (*library.Store, BrowserModel) {
	t.Helper()
	st, err := library.NewStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	for i, name := range []string{"Sultans of Swing", "Layla"} {
		f := filepath.Join(t.TempDir(), name+".txt")
		tab := fmt.Sprintf("Dire Straits\n%s\nTuning: E Standard\n\ne|%d-%d-%d-|\n", name, i+1, i+2, i+3)
		if err := os.WriteFile(f, []byte(tab), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := st.ImportFile(f); err != nil {
			t.Fatal(err)
		}
	}
	m := NewBrowserModel(st)
	rows, err := st.List()
	if err != nil {
		t.Fatal(err)
	}
	m.tabs = rows
	m.loaded = true
	m.width = 140
	m.height = 30
	return st, m
}

// TestBrowserPreviewLoadsForSelectedRow guards US-10: selecting a row loads
// its tab in the background and renders it in the preview panel.
func TestBrowserPreviewLoadsForSelectedRow(t *testing.T) {
	_, m := previewStore(t)
	m, cmd := m.Update(msgs.TabsLoadedMsg{Tabs: m.tabs})
	if cmd == nil {
		t.Fatal("loading tabs should request a preview")
	}
	updated, cmd := m.Update(cmd())
	m = updated
	if m.preview == "" {
		t.Fatal("preview should be populated after load")
	}
	if m.previewTitle != "Layla" {
		t.Fatalf("previewTitle = %q, want first sorted row", m.previewTitle)
	}
	if cmd != nil {
		t.Fatal("no re-request for the same row")
	}
	view := m.View()
	if !strings.Contains(view, "Preview · Layla") {
		t.Fatalf("split view should show the preview panel, got:\n%s", view)
	}
}

// TestBrowserPreviewFollowsCursor guards preview reload on row change.
func TestBrowserPreviewFollowsCursor(t *testing.T) {
	_, m := previewStore(t)
	m, cmd := m.Update(msgs.TabsLoadedMsg{Tabs: m.tabs})
	if cmd == nil {
		t.Fatal("expected preview request on load")
	}
	m, _ = m.Update(cmd())
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if cmd == nil {
		t.Fatal("moving the cursor should request a new preview")
	}
	m, _ = m.Update(cmd())
	if m.previewTitle != "Sultans of Swing" {
		t.Fatalf("preview should follow the cursor to %q, got %q", "Sultans of Swing", m.previewTitle)
	}
}

// TestBrowserPreviewStaleGenerationIgnored guards the request-generation
// guard: a slow load for a row the cursor has left must not clobber the
// current preview.
func TestBrowserPreviewStaleGenerationIgnored(t *testing.T) {
	_, m := previewStore(t)
	m, cmd := m.Update(msgs.TabsLoadedMsg{Tabs: m.tabs})
	gen1 := m.previewGen
	m, _ = m.Update(cmd()) // preview of row 1 (gen1)
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if cmd == nil {
		t.Fatal("expected preview request on cursor move")
	}
	// Deliver a stale gen1 message for a different row.
	m, _ = m.Update(msgs.BrowserPreviewMsg{Gen: gen1, TabID: 99, Title: "Stale", Preview: "stale content"})
	if m.previewTitle == "Stale" {
		t.Fatal("stale generation must not clobber the current preview")
	}
}

// TestBrowserPreviewCollapsesOnNarrowTerminal guards the responsive layout:
// on narrow terminals the browser renders the list full width, as before.
func TestBrowserPreviewCollapsesOnNarrowTerminal(t *testing.T) {
	_, m := previewStore(t)
	m, cmd := m.Update(msgs.TabsLoadedMsg{Tabs: m.tabs})
	m, _ = m.Update(cmd())
	m.width = 80
	m.refresh()
	view := m.View()
	if strings.Contains(view, "Preview ·") {
		t.Fatalf("narrow terminal must not split into a preview panel, got:\n%s", view)
	}
	if m.splitActive() {
		t.Fatal("splitActive must be false on a narrow terminal")
	}
}

// TestBrowserRowShowsSourceBadge guards S1: online-imported tabs carry their
// provenance badge into the library list so the user can see where a tab
// came from and how well it is rated before opening it.
func TestBrowserRowShowsSourceBadge(t *testing.T) {
	st, err := library.NewStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.Import("online://ug/123", &model.Tab{
		Title:    "Sultans of Swing",
		Artist:   "Dire Straits",
		Tuning:   model.Standard,
		Metadata: map[string]string{model.MetaKeySourceBadge: "[UG ★4.9]"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Import("local.txt", &model.Tab{
		Title:  "Local Song",
		Artist: "Local Artist",
		Tuning: model.Standard,
	}); err != nil {
		t.Fatal(err)
	}

	m := NewBrowserModel(st)
	rows, err := st.List()
	if err != nil {
		t.Fatal(err)
	}
	m.tabs = rows
	m.loaded = true
	m.width = 140
	m.height = 30
	m.apply()

	view := m.View()
	if !strings.Contains(view, "[UG ★4.9]") {
		t.Fatalf("browser should show the source badge, got:\n%s", view)
	}
	if !strings.Contains(view, "Local Song") {
		t.Fatalf("local row should still render, got:\n%s", view)
	}
}

// TestBrowserEditMetadataFlow guards S4.1: e starts a two-step editor
// (title then artist); Enter saves each field and the store reflects it.
func TestBrowserEditMetadataFlow(t *testing.T) {
	st, err := library.NewStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.Import("s.txt", &model.Tab{Title: "Old", Artist: "Old Artist", Tuning: model.Standard}); err != nil {
		t.Fatal(err)
	}
	m := NewBrowserModel(st)
	rows, err := st.List()
	if err != nil {
		t.Fatal(err)
	}
	m.tabs = rows
	m.loaded = true
	m.width = 140
	m.height = 30
	m.apply()

	// e → edit title: input starts empty (current value is the placeholder).
	m, _ = m.Update(keyFor("e"))
	if !m.editing || m.editField != 1 {
		t.Fatalf("e should start title editing: editing=%v field=%d", m.editing, m.editField)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("New Title")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.editing || m.editField != 2 {
		t.Fatalf("after title Enter should edit artist: editing=%v field=%d", m.editing, m.editField)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Artist")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.editing {
		t.Fatal("editing should end after artist Enter")
	}
	row, err := st.GetRow(rows[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if row.Title != "New Title" || row.Artist != "Artist" {
		t.Fatalf("store not updated: %+v", row)
	}
}

// TestBrowserEditEmptyKeepsOldValue guards the empty-Enter path: typing
// nothing into a field keeps the previous value.
func TestBrowserEditEmptyKeepsOldValue(t *testing.T) {
	st, err := library.NewStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.Import("s.txt", &model.Tab{Title: "Old", Artist: "Old Artist", Tuning: model.Standard}); err != nil {
		t.Fatal(err)
	}
	m := NewBrowserModel(st)
	rows, _ := st.List()
	m.tabs = rows
	m.loaded = true
	m.width = 140
	m.height = 30
	m.apply()

	m, _ = m.Update(keyFor("e"))
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // empty title: keep "Old"
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("New Artist")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	row, err := st.GetRow(rows[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if row.Title != "Old" || row.Artist != "New Artist" {
		t.Fatalf("empty Enter should keep the old title: %+v", row)
	}
}

// TestBrowserFavoritesFilter guards S4.2: F narrows the list to favorites
// and combines with the fuzzy filter.
func TestBrowserFavoritesFilter(t *testing.T) {
	st, err := library.NewStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	for i, name := range []string{"Alpha", "Beta"} {
		id, err := st.Import(fmt.Sprintf("%s.txt", name), &model.Tab{Title: name, Artist: "A", Tuning: model.Standard})
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			if err := st.SetFavorite(id, true); err != nil {
				t.Fatal(err)
			}
		}
	}
	m := NewBrowserModel(st)
	rows, err := st.List()
	if err != nil {
		t.Fatal(err)
	}
	m.tabs = rows
	m.loaded = true
	m.width = 140
	m.height = 30
	m.apply()

	m, _ = m.Update(keyFor("F"))
	if !m.favOnly || len(m.filtered) != 1 || m.filtered[0].Title != "Alpha" {
		t.Fatalf("favorites filter wrong: favOnly=%v filtered=%+v", m.favOnly, m.filtered)
	}
	// Off again restores the full list.
	m, _ = m.Update(keyFor("F"))
	if m.favOnly || len(m.filtered) != 2 {
		t.Fatalf("filter should toggle off: favOnly=%v n=%d", m.favOnly, len(m.filtered))
	}
}

// TestBrowserExportRow guards S4.3: x writes the tab's plain ASCII to a file
// in the working directory and reports it.
func TestBrowserExportRow(t *testing.T) {
	st, err := library.NewStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	tab := &model.Tab{Title: "Export Me", Artist: "Someone", Tuning: model.Standard,
		Bars: []model.Bar{{Strings: []model.StringLine{{Segments: []model.Segment{{Char: '0', Value: 0, Position: 0, Width: 1}}}}}}}
	id, err := st.Import("e.txt", tab)
	if err != nil {
		t.Fatal(err)
	}
	m := NewBrowserModel(st)
	rows, err := st.List()
	if err != nil {
		t.Fatal(err)
	}
	m.tabs = rows
	m.loaded = true
	m.width = 140
	m.height = 30
	m.apply()

	oldwd, _ := os.Getwd()
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	m, _ = m.Update(keyFor("x"))
	data, err := os.ReadFile(filepath.Join(dir, "Export Me.txt"))
	if err != nil {
		t.Fatalf("export file missing: %v (msg=%q)", err, m.errMsg)
	}
	content := string(data)
	for _, want := range []string{"Export Me", "Someone", "Tuning: EADGBE", "|0|"} {
		if !strings.Contains(content, want) {
			t.Fatalf("export content missing %q:\n%s", want, content)
		}
	}
	if !strings.Contains(m.errMsg, "Exported Export Me.txt") {
		t.Fatalf("status should report the export, got %q", m.errMsg)
	}
	_ = id
}

func keyFor(k string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}

// TestMouseWheelMovesCursor guards G5.1 in the browser: wheel scrolls the
// library list.
func TestMouseWheelMovesCursor(t *testing.T) {
	st, err := library.NewStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	for _, name := range []string{"A", "B", "C"} {
		if _, err := st.Import(name+".txt", &model.Tab{Title: name, Tuning: model.Standard}); err != nil {
			t.Fatal(err)
		}
	}
	m := NewBrowserModel(st)
	rows, _ := st.List()
	m.tabs = rows
	m.loaded = true
	m.width = 140
	m.height = 30
	m.apply()

	m, _ = m.Update(tea.MouseMsg{Type: tea.MouseWheelDown})
	if m.cursor != 1 {
		t.Fatalf("wheel down should move to row 2, got %d", m.cursor)
	}
	m, _ = m.Update(tea.MouseMsg{Type: tea.MouseWheelUp})
	if m.cursor != 0 {
		t.Fatalf("wheel up should move back, got %d", m.cursor)
	}
}
