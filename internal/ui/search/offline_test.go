package search

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fretboard/internal/config"
	"fretboard/internal/scraper"
	"fretboard/internal/testutil"
	"fretboard/internal/ui/msgs"
)

// writeCacheWithTime writes a cache entry with a chosen save time, so the
// offline banner date and stale marker are deterministic in tests.
func writeCacheWithTime(t *testing.T, query string, results []scraper.SearchResult, savedAt time.Time) {
	t.Helper()
	dir, err := config.Dir()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(cacheEntry{Query: query, SavedAt: savedAt, Results: results})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "search_cache.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestSearchOfflineBannerShowsDate verifies the offline fallback banner
// dates the cached results from saved_at; a fresh cache carries no stale
// marker.
func TestSearchOfflineBannerShowsDate(t *testing.T) {
	testutil.RedirectConfigDir(t)
	savedAt := time.Now().Add(-24 * time.Hour)
	writeCacheWithTime(t, "sultans of swing", []scraper.SearchResult{
		{Source: scraper.SourceUG, SongName: "Sultans", ArtistName: "Dire Straits", Type: "Tabs", Rating: 4.9},
	}, savedAt)

	m := NewSearchModel(nil)
	m.lastQuery = "sultans of swing"
	m, _ = m.Update(msgs.SearchPerformedMsg{Err: errSearchFailed, Gen: m.reqGen})

	body := m.renderResults()
	want := "offline — showing cached results from " + savedAt.Format("Jan 2, 2006")
	if !strings.Contains(body, want) {
		t.Fatalf("banner should date the cache, want %q inside %q", want, body)
	}
	if strings.Contains(body, "(stale)") {
		t.Fatalf("a fresh cache must not be flagged stale: %q", body)
	}
}

// TestSearchOfflineBannerStaleMarker verifies a cache past the 7-day TTL is
// still served but flagged "(stale)" in the banner.
func TestSearchOfflineBannerStaleMarker(t *testing.T) {
	testutil.RedirectConfigDir(t)
	writeCacheWithTime(t, "sultans of swing", []scraper.SearchResult{
		{Source: scraper.SourceUG, SongName: "Sultans", ArtistName: "Dire Straits", Type: "Tabs"},
	}, time.Now().Add(-8*24*time.Hour))

	m := NewSearchModel(nil)
	m.lastQuery = "sultans of swing"
	m, _ = m.Update(msgs.SearchPerformedMsg{Err: errSearchFailed, Gen: m.reqGen})

	if body := m.renderResults(); !strings.Contains(body, "(stale)") {
		t.Fatalf("cache past the TTL must be flagged stale, got %q", body)
	}
}

// TestSearchBackOnlineMergesFreshOverCached verifies the newest-wins merge
// on the success path: results cached for the query are combined with the
// fresh page (fresh copy wins per key) and the offline note clears.
func TestSearchBackOnlineMergesFreshOverCached(t *testing.T) {
	testutil.RedirectConfigDir(t)
	writeCacheWithTime(t, "sultans of swing", []scraper.SearchResult{
		{Source: scraper.SourceUG, SongName: "Sultans", ArtistName: "Dire Straits", Type: "Tabs", Rating: 3.0},
		{Source: scraper.SourceSongsterr, SongName: "Sultans", ArtistName: "Dire Straits", Type: "Tabs"},
	}, time.Now().Add(-24*time.Hour))

	m := NewSearchModel(nil)
	m.lastQuery = "sultans of swing"
	m, _ = m.Update(msgs.SearchPerformedMsg{
		Results: []scraper.SearchResult{
			{Source: scraper.SourceUG, SongName: "Sultans", ArtistName: "Dire Straits", Type: "Tabs", Rating: 4.9, Votes: 2100},
		},
		Gen: m.reqGen,
	})

	if len(m.results) != 2 {
		t.Fatalf("fresh results should merge with the cached set, got %d: %+v", len(m.results), m.results)
	}
	for i := range m.results {
		if m.results[i].Source == scraper.SourceUG && m.results[i].Rating != 4.9 {
			t.Fatalf("fresh copy must replace the cached row, got %+v", m.results[i])
		}
	}
	if m.cacheNote != "" {
		t.Fatalf("no offline note after a successful search, got %q", m.cacheNote)
	}
}

// TestMergeCacheFresh pins the merge contract: dedup by result key with the
// fresh copy winning, cached-only rows preserved.
func TestMergeCacheFresh(t *testing.T) {
	cached := []scraper.SearchResult{
		{Source: scraper.SourceUG, SongName: "Sultans", ArtistName: "Dire Straits", Type: "Tabs", Rating: 3.0},
		{Source: scraper.SourceSongsterr, SongName: "Sultans", ArtistName: "Dire Straits", Type: "Tabs"},
	}
	fresh := []scraper.SearchResult{
		{Source: scraper.SourceUG, SongName: "Sultans", ArtistName: "Dire Straits", Type: "Tabs", Rating: 4.9, Votes: 2100},
		{Source: scraper.SourceGuitarTabs, SongName: "Sultans", ArtistName: "Cover Band", Type: "Tabs"},
	}
	merged := MergeCacheFresh(cached, fresh)
	if len(merged) != 3 {
		t.Fatalf("got %d results, want 3 (dup collapsed, fresh copy wins)", len(merged))
	}
	var ug *scraper.SearchResult
	for i := range merged {
		if merged[i].Source == scraper.SourceUG {
			ug = &merged[i]
		}
	}
	if ug == nil || ug.Rating != 4.9 {
		t.Fatalf("fresh copy must win the dedup, got %+v", merged)
	}
}
