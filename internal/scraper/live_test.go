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
