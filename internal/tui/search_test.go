package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/YOUR_USERNAME/fretboard/internal/scraper"
	tea "github.com/charmbracelet/bubbletea"
)

func TestSearchEmptyEnterKeepsQueryFocus(t *testing.T) {
	m := NewSearchModel(nil)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.inputActive {
		t.Fatal("empty Enter should keep query focus")
	}
	if m.errMsg == "" {
		t.Fatal("expected validation error on empty query")
	}
}

func TestSearchSlashRefocusesQuery(t *testing.T) {
	m := NewSearchModel(nil)
	m.focusResults()
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if !m.inputActive {
		t.Fatal("/ should refocus query")
	}
}

func TestSearchResetClearsState(t *testing.T) {
	m := NewSearchModel(nil)
	m.results = []scraper.SearchResult{{SongName: "Layla", ArtistName: "Clapton"}}
	m.input.SetValue("layla")
	m.focusResults()
	m.Reset()
	if len(m.results) != 0 {
		t.Fatal("reset should clear results")
	}
	if m.input.Value() != "" {
		t.Fatal("reset should clear query")
	}
	if !m.inputActive {
		t.Fatal("reset should focus query")
	}
}

func TestSearchPerformedWithResultsMovesToResults(t *testing.T) {
	m := NewSearchModel(nil)
	m, _ = m.Update(SearchPerformedMsg{Results: []scraper.SearchResult{{SongName: "A", ArtistName: "B"}}})
	if m.inputActive {
		t.Fatal("results should receive focus after successful search")
	}
	if m.cursor != 0 {
		t.Fatal("cursor should reset to first result")
	}
}

func TestSearchTabMovesToResults(t *testing.T) {
	m := NewSearchModel(nil)
	m.results = []scraper.SearchResult{{SongName: "A", ArtistName: "B"}}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.inputActive {
		t.Fatal("Tab should move focus to results")
	}
}

func TestSearchEscCancelsLoading(t *testing.T) {
	m := NewSearchModel(nil)
	m.loading = true
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.loading {
		t.Fatal("esc should cancel loading state")
	}
	if !m.inputActive {
		t.Fatal("esc should return focus to query after cancel")
	}
}

func TestOnlineTabPath(t *testing.T) {
	p := OnlineTabPath(scraper.SearchResult{Source: scraper.SourceUG, ID: 42})
	if p != "online://ug/42" {
		t.Fatalf("unexpected path %q", p)
	}
}

func TestSearchRenderShowsFocusIndicator(t *testing.T) {
	m := NewSearchModel(nil)
	view := m.View()
	if !strings.Contains(view, "Query") {
		t.Fatal("view should include query panel")
	}
}

func TestSearchImportShowsFetchingMessage(t *testing.T) {
	m := NewSearchModel(nil)
	m.loading = true
	m.importing = true
	body := m.renderResults()
	if !strings.Contains(body, "Fetching tab") {
		t.Fatalf("expected fetching message, got %q", body)
	}
}

func TestSearchIgnoresStaleSearchResults(t *testing.T) {
	m := NewSearchModel(nil)
	m.reqGen = 2
	m, _ = m.Update(SearchPerformedMsg{
		Results: []scraper.SearchResult{{SongName: "Layla", ArtistName: "Clapton"}},
		Gen:     1,
	})
	if len(m.results) != 0 {
		t.Fatal("stale search results should be ignored")
	}
}

func TestSearchEscInvalidatesInFlightImport(t *testing.T) {
	m := NewSearchModel(nil)
	m.loading = true
	m.importing = true
	before := m.reqGen
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.reqGen != before+1 {
		t.Fatalf("expected reqGen increment on cancel, got %d -> %d", before, m.reqGen)
	}
}

func TestSearchImportErrorKeepsResultsFocus(t *testing.T) {
	m := NewSearchModel(nil)
	m.results = []scraper.SearchResult{{SongName: "Layla", ArtistName: "Clapton"}}
	m.focusResults()
	m, _ = m.Update(TabImportErrorMsg{Err: fmt.Errorf("network down")})
	if m.inputActive {
		t.Fatal("import error should keep results focus")
	}
	if m.errMsg == "" {
		t.Fatal("expected import error message")
	}
	if len(m.results) == 0 {
		t.Fatal("results list should be preserved after import error")
	}
}

func TestSearchJKMovesResultsWhileTyping(t *testing.T) {
	m := NewSearchModel(nil)
	m.results = []scraper.SearchResult{{SongName: "A"}, {SongName: "B"}}
	m.inputActive = true
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.inputActive {
		t.Fatal("j should move to results")
	}
	if m.cursor != 0 {
		t.Fatalf("cursor=%d want 0", m.cursor)
	}
}

func TestSearchUpMovesResultsWhileTyping(t *testing.T) {
	m := NewSearchModel(nil)
	m.results = []scraper.SearchResult{{SongName: "A", ArtistName: "B"}, {SongName: "C", ArtistName: "D"}}
	m.cursor = 1
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.inputActive {
		t.Fatal("up should move results while query focused")
	}
	if m.cursor != 0 {
		t.Fatalf("cursor=%d want 0", m.cursor)
	}
}

func TestSearchResetInvalidatesInFlight(t *testing.T) {
	m := NewSearchModel(nil)
	m.reqGen = 1
	m.Reset()
	if m.reqGen != 2 {
		t.Fatalf("reset should bump reqGen, got %d", m.reqGen)
	}
	m, _ = m.Update(SearchPerformedMsg{
		Results: []scraper.SearchResult{{SongName: "Layla", ArtistName: "Clapton"}},
		Gen:     1,
	})
	if len(m.results) != 0 {
		t.Fatal("stale search results should be ignored after reset")
	}
}

func TestSearchResultsResetViewport(t *testing.T) {
	m := NewSearchModel(nil)
	m.results = []scraper.SearchResult{{SongName: "Old"}}
	m.viewport.SetYOffset(5)
	m.reqGen = 1
	m, _ = m.Update(SearchPerformedMsg{
		Results: []scraper.SearchResult{{SongName: "Layla", ArtistName: "Clapton"}},
		Gen:     1,
	})
	if m.viewport.YOffset != 0 {
		t.Fatalf("viewport offset = %d, want 0 on new results", m.viewport.YOffset)
	}
}

func TestSearchTypeQDoesNotQuitWhileTyping(t *testing.T) {
	m := NewSearchModel(nil)
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd != nil {
		if _, isQuit := cmd().(tea.QuitMsg); isQuit {
			t.Fatal("typing q while query focused should not quit")
		}
	}
	if m.input.Value() != "q" {
		t.Fatalf("input value = %q, want %q", m.input.Value(), "q")
	}
}

func TestSearchQQuitsWhenResultsFocused(t *testing.T) {
	m := NewSearchModel(nil)
	m.focusResults()
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if _, isQuit := cmd().(tea.QuitMsg); !isQuit {
		t.Fatalf("expected QuitMsg from q with results focused, got %#v", cmd())
	}
}
