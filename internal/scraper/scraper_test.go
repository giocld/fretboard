package scraper

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Pilfer/ultimate-guitar-scraper/pkg/ultimateguitar"
)

// TestUGAPIClientHasTimeout guards against the dependency's default client
// (no timeout): a hung UG request would block the TUI forever. The HTML and
// Songsterr backends already set their own timeouts.
func TestUGAPIClientHasTimeout(t *testing.T) {
	c := newUGAPIClient(&rateLimiter{delay: 0})
	sc, ok := c.scraper.(*ultimateguitar.Scraper)
	if !ok {
		t.Skip("scraper is not the real UG client")
	}
	if sc.Client == nil || sc.Client.Timeout != ugRequestTimeout {
		t.Fatalf("UG API client timeout = %v, want %v", sc.Client.Timeout, ugRequestTimeout)
	}
}

// TestRateLimiterSharedAcrossBackends guards the documented contract that
// delay is the minimum time between network requests across all backends.
// Each backend used to own a private limiter, so UG API + UG HTML + Songsterr
// requests fired back-to-back on a single search.
func TestRateLimiterSharedAcrossBackends(t *testing.T) {
	c := NewClient(time.Hour)
	if c.ug.rl != c.ugHTML.rl || c.ug.rl != c.songsterr.rl {
		t.Fatal("all backends must share one rate limiter")
	}
	if c.ug.rl == nil {
		t.Fatal("nil rate limiter")
	}
}

// TestUGHTMLSearchRejectsBadStatus guards the missing status check: on
// 403/429/5xx the error page used to be parsed and surfaced as a misleading
// "data-content not found" instead of a status error.
func TestUGHTMLSearchRejectsBadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	c := &ugHTMLClient{http: srv.Client(), rl: &rateLimiter{}, base: srv.URL}
	_, err := c.Search("layla")
	if err == nil {
		t.Fatal("expected error on non-200 status")
	}
	if !strings.Contains(err.Error(), "429") {
		t.Fatalf("error should mention the status, got %q", err)
	}
}

// TestNormalizeContentIsDeterministic guards map-iteration-order decoding:
// double-encoded entities must always decode to the same output.
func TestNormalizeContentIsDeterministic(t *testing.T) {
	input := "&amp;quot;hello&amp;quot;"
	first := normalizeContent(input)
	for i := 0; i < 200; i++ {
		if got := normalizeContent(input); got != first {
			t.Fatalf("run %d differs: %q vs %q", i, got, first)
		}
	}
	// Specific expected outcome for the fixed order: "&quot;" first never
	// matches inside "&amp;quot;", then "&amp;" decodes, leaving "&quot;"
	// intact. (With map iteration order this used to flip between the two
	// decodings run to run.)
	if first != "&quot;hello&quot;" {
		t.Fatalf("unexpected decode: %q", first)
	}
}

func TestMergeSearchResultsDedupes(t *testing.T) {
	a := []SearchResult{{Source: SourceUG, ArtistName: "A", SongName: "S"}}
	b := []SearchResult{
		{Source: SourceUG, ArtistName: "A", SongName: "S"},
		{Source: SourceSongsterr, ArtistName: "B", SongName: "T"},
	}
	out := mergeSearchResults(a, b)
	if len(out) != 2 {
		t.Fatalf("expected 2 results, got %d", len(out))
	}
}

func TestExtractUGDataContent(t *testing.T) {
	body := []byte(`<html><div data-content="{&quot;store&quot;:{&quot;page&quot;:{&quot;data&quot;:{&quot;x&quot;:1}}}}"></div></html>`)
	payload, err := extractUGDataContent(body)
	if err != nil {
		t.Fatal(err)
	}
	var v map[string]any
	if err := json.Unmarshal(payload, &v); err != nil {
		t.Fatal(err)
	}
}

func TestUGTabURLSlugPattern(t *testing.T) {
	r := SearchResult{ID: 63588, ArtistName: "Eric Clapton", SongName: "Layla", Type: "Chords"}
	want := "https://tabs.ultimate-guitar.com/tab/eric-clapton/layla-chords-63588"
	if got := ugTabURL(r); got != want {
		t.Fatalf("ugTabURL = %q, want %q", got, want)
	}
}

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Eric Clapton": "eric-clapton",
		"Layla":        "layla",
		"  AC/DC  ":    "ac-dc",
		"Queen - Live": "queen-live",
		"O'Reilly":     "o-reilly",
		"123":          "123",
	}
	for in, want := range cases {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestUGHTMLSearchFiltersNonPublicResults(t *testing.T) {
	body := []byte(`<html><div data-content="{&quot;store&quot;:{&quot;page&quot;:{&quot;data&quot;:{&quot;results&quot;:[
		{&quot;id&quot;:1,&quot;song_name&quot;:&quot;A&quot;,&quot;artist_name&quot;:&quot;B&quot;,&quot;type&quot;:&quot;Tabs&quot;,&quot;tab_url&quot;:&quot;https://tabs.ultimate-guitar.com/tab/b/a-tabs-1&quot;},
		{&quot;tab_id&quot;:2,&quot;song_name&quot;:&quot;Official&quot;,&quot;artist_name&quot;:&quot;B&quot;,&quot;type&quot;:&quot;&quot;},
		{&quot;id&quot;:3,&quot;song_name&quot;:&quot;C&quot;,&quot;artist_name&quot;:&quot;B&quot;,&quot;type&quot;:&quot;Pro&quot;}
	]}}}}"></div></html>`)
	c := newUGHTMLClient(&rateLimiter{delay: time.Millisecond})
	results, err := c.parseSearch(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1 (official/pro rows must be filtered)", len(results))
	}
	if results[0].ID != 1 || results[0].TabURL != "https://tabs.ultimate-guitar.com/tab/b/a-tabs-1" {
		t.Fatalf("unexpected result: %+v", results[0])
	}
}
