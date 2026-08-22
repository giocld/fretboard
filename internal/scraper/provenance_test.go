package scraper

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Pilfer/ultimate-guitar-scraper/pkg/ultimateguitar"
)

// --- Paywall ranking (WORKON 5.1) ---

// TestProScoreCappedBelowCommunityTabs pins the paywall ranking: a
// Pro/official-only tab scores below any fetchable community tab, even a
// weakly rated one, while Pro copies keep their relative order so the best
// Pro copy still surfaces first in an all-Pro pool.
func TestProScoreCappedBelowCommunityTabs(t *testing.T) {
	community := SearchResult{Source: SourceUG, SongName: "Sultans", ArtistName: "Dire Straits", Type: "Tabs", Rating: 3.0, Votes: 10}
	pro := SearchResult{Source: SourceUG, SongName: "Sultans", ArtistName: "Dire Straits", Type: "Tabs", Rating: 4.9, Votes: 2100, Pro: true}
	if cs, ps := resultScore(community), resultScore(pro); cs <= ps {
		t.Fatalf("community tab (%d) must outrank a pro tab (%d)", cs, ps)
	}
	weakPro := SearchResult{Source: SourceUG, SongName: "Sultans", ArtistName: "Dire Straits", Type: "Tabs", Pro: true}
	if ps, wp := resultScore(pro), resultScore(weakPro); ps <= wp {
		t.Fatalf("within the Pro pool the strongly-rated copy must rank first: %d <= %d", ps, wp)
	}
}

// TestMergeProDupKeepsCommunityCopy verifies dedup of a Pro row with a
// community row of the same song+type keeps the fetchable copy.
func TestMergeProDupKeepsCommunityCopy(t *testing.T) {
	community := SearchResult{Source: SourceUG, SongName: "Layla", ArtistName: "Clapton", Type: "Tabs", Rating: 4.2, Votes: 300}
	pro := SearchResult{Source: SourceUG, SongName: "Layla", ArtistName: "Clapton", Type: "Tabs", Rating: 4.9, Votes: 2100, Pro: true}
	merged := mergeSearchResults([]SearchResult{pro}, []SearchResult{community})
	if len(merged) != 1 {
		t.Fatalf("duplicate of the same song+type must collapse, got %+v", merged)
	}
	if merged[0].Pro {
		t.Fatalf("dedup must keep the fetchable community copy, got %+v", merged[0])
	}
}

// --- FetchBest paywall fallthrough (WORKON 5.1) ---

// fetchBestScraper fakes the UG backend for FetchBest: Search returns a
// canned result list and GetTabByID serves per-id content (or fails for
// paywalled ids).
type fetchBestScraper struct {
	search ultimateguitar.SearchResult
	tabs   map[int64]ultimateguitar.TabResult
}

func (f *fetchBestScraper) Search(ultimateguitar.SearchParams) (ultimateguitar.SearchResult, error) {
	return f.search, nil
}

func (f *fetchBestScraper) GetTabByID(id int64) (ultimateguitar.TabResult, error) {
	if t, ok := f.tabs[id]; ok {
		return t, nil
	}
	return ultimateguitar.TabResult{}, errors.New("tab behind paywall")
}

func fetchBestClient(f *fetchBestScraper) *Client {
	return &Client{
		ug: &ugAPIClient{scraper: f, rl: &rateLimiter{delay: 0}},
	}
}

// TestFetchBestFallsBackToCommunityOnPaywall pins the fallthrough: a Pro
// result whose direct fetch fails (paywall) re-searches the query and
// returns the best community copy, with the substitution reason.
func TestFetchBestFallsBackToCommunityOnPaywall(t *testing.T) {
	f := &fetchBestScraper{
		search: ultimateguitar.SearchResult{Tabs: []ultimateguitar.Tab{
			{ID: 200, SongName: "Sultans of Swing", ArtistName: "Dire Straits", Type: "Tabs"},
		}},
		tabs: map[int64]ultimateguitar.TabResult{
			200: {Content: validTabContent, SongName: "Sultans of Swing", ArtistName: "Dire Straits"},
		},
	}
	c := fetchBestClient(f)
	pro := SearchResult{ID: 100, Source: SourceUG, SongName: "Sultans of Swing", ArtistName: "Dire Straits", Type: "Pro", Pro: true}
	tab, reason, err := c.FetchBest(pro, "sultans of swing dire straits")
	if err != nil {
		t.Fatal(err)
	}
	if tab == nil || tab.Title == "" {
		t.Fatal("expected a fetched community tab")
	}
	if reason != pickReasonProFallback {
		t.Fatalf("reason = %q, want %q", reason, pickReasonProFallback)
	}
}

// TestFetchBestSucceedsDirectlyWhenProTabFetchable verifies no fallback when
// the Pro tab itself fetches (e.g. a Pro session): no reason, no re-search.
func TestFetchBestSucceedsDirectlyWhenProTabFetchable(t *testing.T) {
	f := &fetchBestScraper{
		tabs: map[int64]ultimateguitar.TabResult{
			100: {Content: validTabContent, SongName: "Sultans of Swing", ArtistName: "Dire Straits"},
		},
	}
	c := fetchBestClient(f)
	pro := SearchResult{ID: 100, Source: SourceUG, SongName: "Sultans of Swing", ArtistName: "Dire Straits", Type: "Pro", Pro: true}
	tab, reason, err := c.FetchBest(pro, "sultans of swing dire straits")
	if err != nil {
		t.Fatal(err)
	}
	if reason != "" {
		t.Fatalf("direct fetch should carry no reason, got %q", reason)
	}
	if tab == nil || tab.Title == "" {
		t.Fatal("expected the fetched pro tab")
	}
}

// TestFetchBestNonProFailsHard verifies a plain (non-Pro) fetch failure
// surfaces the error instead of silently substituting another result.
func TestFetchBestNonProFailsHard(t *testing.T) {
	f := &fetchBestScraper{
		search: ultimateguitar.SearchResult{Tabs: []ultimateguitar.Tab{
			{ID: 200, SongName: "Sultans of Swing", ArtistName: "Dire Straits", Type: "Tabs"},
		}},
		tabs: map[int64]ultimateguitar.TabResult{
			200: {Content: validTabContent, SongName: "Sultans of Swing", ArtistName: "Dire Straits"},
		},
	}
	c := fetchBestClient(f)
	plain := SearchResult{ID: 100, Source: SourceUG, SongName: "Sultans of Swing", ArtistName: "Dire Straits", Type: "Tabs"}
	tab, reason, err := c.FetchBest(plain, "sultans of swing dire straits")
	if err == nil {
		t.Fatal("expected a hard error for a non-Pro fetch failure")
	}
	if reason != "" || tab != nil {
		t.Fatalf("no fallback expected: tab=%v reason=%q", tab, reason)
	}
}

// TestFetchBestFailsWithoutCommunityVersion verifies the fallback only fails
// when nothing else exists: a search that returns only paywalled copies
// yields an explanatory error, never a fabricated tab.
func TestFetchBestFailsWithoutCommunityVersion(t *testing.T) {
	f := &fetchBestScraper{
		search: ultimateguitar.SearchResult{Tabs: []ultimateguitar.Tab{
			{ID: 100, SongName: "Sultans of Swing", ArtistName: "Dire Straits", Type: "Pro", TabAccessType: "pro"},
		}},
		tabs: map[int64]ultimateguitar.TabResult{},
	}
	c := fetchBestClient(f)
	pro := SearchResult{ID: 100, Source: SourceUG, SongName: "Sultans of Swing", ArtistName: "Dire Straits", Type: "Pro", Pro: true}
	_, _, err := c.FetchBest(pro, "sultans of swing dire straits")
	if err == nil {
		t.Fatal("expected an error when no community version exists")
	}
	if !strings.Contains(err.Error(), "no community version") {
		t.Fatalf("error should explain the missing community version, got %q", err)
	}
}

// TestFetchBestRestrictsFallbackToSameArtist verifies the fallback stays on
// the same query+artist: a cover by a different artist is not a substitute.
func TestFetchBestRestrictsFallbackToSameArtist(t *testing.T) {
	f := &fetchBestScraper{
		search: ultimateguitar.SearchResult{Tabs: []ultimateguitar.Tab{
			{ID: 200, SongName: "Sultans of Swing", ArtistName: "Some Cover Band", Type: "Tabs"},
		}},
		tabs: map[int64]ultimateguitar.TabResult{
			200: {Content: validTabContent, SongName: "Sultans of Swing", ArtistName: "Some Cover Band"},
		},
	}
	c := fetchBestClient(f)
	pro := SearchResult{ID: 100, Source: SourceUG, SongName: "Sultans of Swing", ArtistName: "Dire Straits", Type: "Pro", Pro: true}
	if _, _, err := c.FetchBest(pro, "sultans of swing dire straits"); err == nil {
		t.Fatal("fallback must stay on the same artist; a cover version is not a substitute")
	}
}

// --- Pro detection (WORKON 5.1) ---

// proSearchScraper fakes the UG API backend returning a fixed tab list.
type proSearchScraper struct {
	search ultimateguitar.SearchResult
}

func (f *proSearchScraper) Search(ultimateguitar.SearchParams) (ultimateguitar.SearchResult, error) {
	return f.search, nil
}

func (f *proSearchScraper) GetTabByID(int64) (ultimateguitar.TabResult, error) {
	return ultimateguitar.TabResult{}, nil
}

// TestUGAPISearchFlagsPro pins pro/official detection on the API backend:
// the tab_access_type and Pro/Official types surface as Pro=true.
func TestUGAPISearchFlagsPro(t *testing.T) {
	f := &proSearchScraper{search: ultimateguitar.SearchResult{Tabs: []ultimateguitar.Tab{
		{ID: 1, SongName: "A", ArtistName: "B", Type: "Tabs", TabAccessType: "public"},
		{ID: 2, SongName: "B", ArtistName: "C", Type: "Tabs", TabAccessType: "pro"},
		{ID: 3, SongName: "C", ArtistName: "D", Type: "Official"},
	}}}
	c := &ugAPIClient{scraper: f, rl: &rateLimiter{delay: 0}}
	res, err := c.SearchPage("x", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 3 {
		t.Fatalf("got %d results, want 3", len(res))
	}
	if res[0].Pro {
		t.Fatalf("public access tab must not be flagged Pro: %+v", res[0])
	}
	if !res[1].Pro || !res[2].Pro {
		t.Fatalf("pro access type and Official type must be flagged Pro: %+v", res[1:])
	}
}

// TestUGHTMLSearchFlagsProAccess pins pro detection on the HTML backend:
// Tabs/Chords rows with a pro/official tab_access_type pass the type filter
// but are flagged Pro so they rank below fetchable rows.
func TestUGHTMLSearchFlagsProAccess(t *testing.T) {
	body := []byte(`<html><div data-content="{&quot;store&quot;:{&quot;page&quot;:{&quot;data&quot;:{&quot;results&quot;:[
		{&quot;id&quot;:1,&quot;song_name&quot;:&quot;A&quot;,&quot;artist_name&quot;:&quot;B&quot;,&quot;type&quot;:&quot;Tabs&quot;,&quot;tab_url&quot;:&quot;u&quot;},
		{&quot;id&quot;:2,&quot;song_name&quot;:&quot;Pro&quot;,&quot;artist_name&quot;:&quot;B&quot;,&quot;type&quot;:&quot;Tabs&quot;,&quot;tab_url&quot;:&quot;v&quot;,&quot;tab_access_type&quot;:&quot;pro&quot;},
		{&quot;id&quot;:3,&quot;song_name&quot;:&quot;Off&quot;,&quot;artist_name&quot;:&quot;B&quot;,&quot;type&quot;:&quot;Chords&quot;,&quot;tab_url&quot;:&quot;w&quot;,&quot;tab_access_type&quot;:&quot;official&quot;}
	]}}}}"></div></html>`)
	c := newUGHTMLClient(&rateLimiter{delay: time.Millisecond})
	results, err := c.parseSearch(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}
	if results[0].Pro || !results[1].Pro || !results[2].Pro {
		t.Fatalf("pro flags wrong: %+v", results)
	}
}

// --- Songsterr provenance (WORKON 5.2) ---

// TestSongsterrSearchSetsProvenance pins the Songsterr provenance: results
// are flagged reconstructed (the tab is rebuilt via UG at fetch time) and
// carry the canonical song page as SourceURL.
func TestSongsterrSearchSetsProvenance(t *testing.T) {
	body := `[{"songId":12345,"artist":"Dire Straits","title":"Sultans of Swing","tracks":[{"instrument":"Guitar","views":10}]}]`
	c := &songsterrClient{
		http: &http.Client{Transport: &songsterrRoundTripper{code: http.StatusOK, body: body}},
		rl:   &rateLimiter{delay: 0},
	}
	res, err := c.SearchPage("sultans", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 {
		t.Fatalf("got %d results, want 1", len(res))
	}
	r := res[0]
	if !r.Reconstructed {
		t.Fatal("songsterr results must be marked reconstructed")
	}
	if want := "https://www.songsterr.com/a/wsa/12345"; r.SourceURL != want {
		t.Fatalf("SourceURL = %q, want %q", r.SourceURL, want)
	}
}
