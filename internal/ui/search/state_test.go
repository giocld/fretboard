package search

import (
	"strings"
	"testing"

	"fretboard/internal/scraper"
	"fretboard/internal/testutil"
	"fretboard/internal/ui/msgs"
	tea "github.com/charmbracelet/bubbletea"
)

// TestAddHistory guards G1.1: queries move to the front, duplicates collapse,
// and the list is capped.
func TestAddHistory(t *testing.T) {
	h := []string{"layla", "wonderwall"}
	h = addHistory(h, "sultans of swing")
	if len(h) != 3 || h[0] != "sultans of swing" {
		t.Fatalf("new query should move to front: %v", h)
	}
	h = addHistory(h, "layla")
	if len(h) != 3 || h[0] != "layla" || h[1] != "sultans of swing" {
		t.Fatalf("repeat query should move to front without duplicating: %v", h)
	}
	for i := 0; i < 12; i++ {
		h = addHistory(h, "query "+string(rune('a'+i)))
	}
	if len(h) != maxHistory {
		t.Fatalf("history should be capped at %d, got %d", maxHistory, len(h))
	}
	if h[0] != "query l" {
		t.Fatalf("newest should be first: %v", h)
	}
	if addHistory(h, "   ") != nil && len(addHistory(h, "   ")) != len(h) {
		t.Fatal("blank queries must be ignored")
	}
}

// TestHistoryPersistsRoundTrip guards G1.1 persistence: save then reload
// through the config dir.
func TestHistoryPersistsRoundTrip(t *testing.T) {
	testutil.RedirectConfigDir(t)
	h := addHistory(nil, "sultans of swing")
	h = addHistory(h, "layla")
	saveHistory(h)

	loaded := loadHistory()
	if len(loaded) != 2 || loaded[0] != "layla" || loaded[1] != "sultans of swing" {
		t.Fatalf("round-trip mismatch: %v", loaded)
	}
}

// TestCacheRoundTrip guards G1.2: results persist for their query and are
// not served for a different query.
func TestCacheRoundTrip(t *testing.T) {
	testutil.RedirectConfigDir(t)
	results := []scraper.SearchResult{
		{Source: scraper.SourceUG, SongName: "Sultans", ArtistName: "Dire Straits", Type: "Tabs", Rating: 4.9},
	}
	saveCache("sultans of swing", results)

	got, ok := loadCache("sultans of swing")
	if !ok || len(got) != 1 || got[0].Rating != 4.9 {
		t.Fatalf("cache round-trip failed: %v ok=%v", got, ok)
	}
	if _, ok := loadCache("layla"); ok {
		t.Fatal("cache must not serve a different query")
	}
}

// TestSearchOfflineCacheRestore guards G1.2: when a search fails and a cache
// exists, the screen serves the cached list with an explanatory note.
func TestSearchOfflineCacheRestore(t *testing.T) {
	testutil.RedirectConfigDir(t)
	saveCache("sultans of swing", []scraper.SearchResult{
		{Source: scraper.SourceUG, SongName: "Sultans", ArtistName: "Dire Straits", Type: "Tabs", Rating: 4.9},
	})

	m := NewSearchModel(nil)
	m.lastQuery = "sultans of swing"
	m, _ = m.Update(msgs.SearchPerformedMsg{Err: errSearchFailed, Gen: m.reqGen})

	if len(m.results) != 1 {
		t.Fatalf("cached results should be served, got %d", len(m.results))
	}
	if !strings.Contains(m.cacheNote, "offline cache") {
		t.Fatalf("expected an offline-cache note, got %q", m.cacheNote)
	}
	if m.errMsg != "" {
		t.Fatalf("the raw error should be folded into the note, got %q", m.errMsg)
	}
	if m.inputActive {
		t.Fatal("cached results should put the user in results mode")
	}
}

var errSearchFailed = errSearchFailedT{}

type errSearchFailedT struct{}

func (errSearchFailedT) Error() string { return "search failed: network down" }

// TestSearchHistoryRecall guards G1.1: up in an empty query box fills the
// input from history; down clears it again.
func TestSearchHistoryRecall(t *testing.T) {
	m := NewSearchModel(nil)
	m.history = []string{"layla", "wonderwall"}

	m, _ = m.Update(keyMsg("up"))
	if m.input.Value() != "layla" {
		t.Fatalf("up should recall the newest query, got %q", m.input.Value())
	}
	m, _ = m.Update(keyMsg("up"))
	if m.input.Value() != "wonderwall" {
		t.Fatalf("second up should recall the older query, got %q", m.input.Value())
	}
	m, _ = m.Update(keyMsg("down"))
	if m.input.Value() != "layla" {
		t.Fatalf("down should walk forward, got %q", m.input.Value())
	}
	m, _ = m.Update(keyMsg("down"))
	if m.input.Value() != "" {
		t.Fatalf("down past the newest should clear the box, got %q", m.input.Value())
	}
}

// TestSearchLoadMoreMerge guards G1.3: a load-more message appends new
// results, reports the count, and says "No more results" when nothing new
// arrives.
func TestSearchLoadMoreMerge(t *testing.T) {
	m := NewSearchModel(nil)
	m.results = []scraper.SearchResult{
		{Source: scraper.SourceUG, SongName: "Sultans", ArtistName: "Dire Straits", Type: "Tabs"},
	}
	m.lastQuery = "sultans of swing"

	more := []scraper.SearchResult{
		{Source: scraper.SourceSongsterr, SongName: "Sultans", ArtistName: "Dire Straits", Type: "Tabs"},
		{Source: scraper.SourceUG, SongName: "Sultans", ArtistName: "Dire Straits", Type: "Tabs"}, // dup
	}
	m, _ = m.Update(msgs.SearchPerformedMsg{Results: more, Gen: m.reqGen, More: true})
	if len(m.results) != 2 {
		t.Fatalf("load-more should append new + collapse dups, got %d: %+v", len(m.results), m.results)
	}
	if !strings.Contains(m.errMsg, "Loaded 1 more") {
		t.Fatalf("expected a load-more count note, got %q", m.errMsg)
	}

	m, _ = m.Update(msgs.SearchPerformedMsg{Results: more, Gen: m.reqGen, More: true})
	if !strings.Contains(m.errMsg, "No more results") {
		t.Fatalf("duplicate-only page should say no more, got %q", m.errMsg)
	}
}

func keyMsg(k string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}
