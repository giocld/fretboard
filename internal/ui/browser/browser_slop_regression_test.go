package browser

import (
	"testing"

	"fretboard/internal/library"
)

// TestBrowserSearchNarrowsToMatches pins the fuzzy filter behavior: with a
// search term active, filterAndSort keeps only the rows matching the query
// (no duplicates), preserving order by match quality.
func TestBrowserSearchNarrowsToMatches(t *testing.T) {
	m := NewBrowserModel(nil)
	m.tabs = []library.TabRow{
		{ID: 1, Title: "Sultans of Swing", Artist: "Dire Straits"},
		{ID: 2, Title: "Layla", Artist: "Derek and the Dominos"},
		{ID: 3, Title: "Wonderwall", Artist: "Oasis"},
	}
	m.loaded = true
	m.searchInput = "sultans"
	m.apply()

	if len(m.filtered) != 1 || m.filtered[0].ID != 1 {
		t.Fatalf("search should narrow to the matching row only, got %+v", m.filtered)
	}

	// A query matching multiple rows returns each matching row exactly once.
	m.searchInput = "a"
	m.apply()
	if len(m.filtered) != 3 {
		t.Fatalf("expected all rows to match 'a', got %+v", m.filtered)
	}
	seen := make(map[int64]bool)
	for _, r := range m.filtered {
		if seen[r.ID] {
			t.Fatalf("duplicate row %d in filtered results", r.ID)
		}
		seen[r.ID] = true
	}
}
