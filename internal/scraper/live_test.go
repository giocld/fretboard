//go:build live

package scraper

import (
	"os"
	"strings"
	"testing"
	"time"
)

// Live integration checks against real Ultimate Guitar pages. Run with:
// go test -tags live -run LiveUG ./internal/scraper/
func TestLiveUGSearchAndFetch(t *testing.T) {
	query := os.Getenv("UG_QUERY")
	if query == "" {
		query = "layla"
	}
	html := newUGHTMLClient(&rateLimiter{delay: 1100 * time.Millisecond})
	results, err := html.Search(query)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("search returned no results")
	}
	for _, r := range results {
		if r.ID == 0 {
			t.Errorf("result with zero ID: %+v", r)
		}
		if r.TabURL == "" {
			t.Errorf("result without tab_url: %+v", r)
		}
	}
	tab, err := html.Fetch(results[0])
	if err != nil {
		// Chord-only pages are rejected with a clear error until chord
		// sheets are supported (BUG-018); anything else is a failure.
		if strings.Contains(err.Error(), "chord sheet") {
			t.Logf("chord sheet rejected as expected: %v", err)
			return
		}
		t.Fatalf("fetch %+v: %v", results[0], err)
	}
	if len(tab.Bars) == 0 {
		t.Fatal("fetched tab has no bars")
	}
	t.Logf("fetched %q by %q, %d bars via %s", tab.Title, tab.Artist, len(tab.Bars), results[0].TabURL)
}

// TestLiveSearchRankedOfficialFirst is the acceptance check for S1: a real
// multi-source search for a famous song must surface the official, top-rated
// tab above covers and chord sheets. Run with:
// go test -tags live -run LiveSearchRanked ./internal/scraper/ -v
func TestLiveSearchRankedOfficialFirst(t *testing.T) {
	c := NewClient(1100 * time.Millisecond)
	results, err := c.Search("sultans of swing dire straits")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("search returned no results")
	}
	t.Logf("top 8 of %d results:", len(results))
	for i, r := range results {
		if i >= 8 {
			break
		}
		t.Logf("  %d. %s %s — %s [%s] %.1f★ %dv", i+1, SourceBadge(r), r.SongName, r.ArtistName, r.Type, r.Rating, r.Votes)
	}
	top := results[0]
	if !IsTabType(top) {
		t.Fatalf("top result should be a tab, got type %q: %+v", top.Type, top)
	}
	if strings.ToLower(top.ArtistName) != "dire straits" {
		t.Fatalf("top result should be by Dire Straits, got %q", top.ArtistName)
	}
	if top.Rating < 4.0 {
		t.Fatalf("top result should be strongly rated, got %.1f (%+v)", top.Rating, top)
	}
	t.Logf("PASS: top result is the official tab: %s — %s (%.1f★, %d votes, %s)", top.SongName, top.ArtistName, top.Rating, top.Votes, top.Source)
}

// TestLiveSearchPage2FindsMore guards G1.3: a second page returns results
// beyond the first page (new or at least still valid, deduped by the UI).
// go test -tags live -run LiveSearchPage2 ./internal/scraper/ -v
func TestLiveSearchPage2FindsMore(t *testing.T) {
	c := NewClient(1100 * time.Millisecond)
	p1, err := c.SearchPage("sultans of swing dire straits", 1)
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	p2, err := c.SearchPage("sultans of swing dire straits", 2)
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	if len(p1) == 0 {
		t.Fatal("page 1 returned nothing")
	}
	merged := MergeResults(p1, p2)
	t.Logf("page1=%d page2=%d merged=%d", len(p1), len(p2), len(merged))
	if len(merged) <= len(p1) {
		t.Fatalf("page 2 should surface more results, merged=%d <= page1=%d", len(merged), len(p1))
	}
}
