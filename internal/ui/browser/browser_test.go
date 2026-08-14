package browser

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
		Metadata: map[string]string{model.MetaKeySourceBadge: "[UG *4.9]"},
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
	if !strings.Contains(view, "[UG *4.9]") {
		t.Fatalf("browser should show the source badge, got:\n%s", view)
	}
	if !strings.Contains(view, "Local Song") {
		t.Fatalf("local row should still render, got:\n%s", view)
	}
}
