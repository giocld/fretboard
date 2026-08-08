package scraper

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"fretboard/internal/model"
)

// guitartabsSearchPage mirrors the real guitartabs.cc search markup: a
// tabslist table with artist + song anchors, plus navigation links.
const guitartabsSearchPage = `<html><body>
<div class="menu"><a href="/">Tabs</a> <a href="/updates/">Updates</a> <a href="/top_tabs.html">Top 100</a></div>
<table class="tabslist">
<tr><td><a href="/tabs/d/deep_purple/">Deep Purple</a></td>
    <td><a href="/tabs/d/deep_purple/smoke_on_the_water_tab.html" class="ryzh22">Smoke On The Water Tab</a></td>
    <td>Tab</td></tr>
<tr><td><a href="/tabs/d/deep_purple/">Deep Purple</a></td>
    <td><a href="/tabs/d/deep_purple/smoke_on_the_water_crd.html" class="ryzh22">Smoke On The Water Chords</a></td>
    <td>Chords</td></tr>
<tr><td><a href="/tabs/d/deep_purple/">Deep Purple</a></td>
    <td><a href="/tabs/d/deep_purple/smoke_on_the_water_btab.html" class="ryzh22">Smoke On The Water Bass Tab</a></td>
    <td>Bass Tab</td></tr>
<tr><td><a href="/tabs/l/led_zeppelin/">Led Zeppelin</a></td>
    <td><a href="/tabs/l/led_zeppelin/smoke_on_the_water_tab.html" class="ryzh22">Smoke On The Water Tab</a></td>
    <td>Tab</td></tr>
</table>
<div class="paging"><a href="?tabtype=any&band=&song=smoke+on+the+water&p=2">Next</a></div>
</body></html>`

// TestTextTabSearchParsesGuitarTabsPage guards the guitartabs.cc search
// parser: artist/tab pairs, tab types, and URL construction.
func TestTextTabSearchParsesGuitarTabsPage(t *testing.T) {
	results, err := parseTextTabSearch([]byte(guitartabsSearchPage), "smoke on the water", textTabSites[0], "https://www.guitartabs.cc")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 4 {
		t.Fatalf("expected 4 results, got %d: %+v", len(results), results)
	}
	first := results[0]
	if first.ArtistName != "Deep Purple" || first.SongName != "Smoke On The Water" {
		t.Fatalf("first result = %q by %q, want Smoke On The Water / Deep Purple", first.SongName, first.ArtistName)
	}
	if first.Type != "Tab" {
		t.Fatalf("first result type = %q, want Tab", first.Type)
	}
	if first.TabURL != "https://www.guitartabs.cc/tabs/d/deep_purple/smoke_on_the_water_tab.html" {
		t.Fatalf("TabURL = %q", first.TabURL)
	}
	if results[1].Type != "Chords" || results[2].Type != "Bass Tab" {
		t.Fatalf("types wrong: %+v", results)
	}
	if results[3].ArtistName != "Led Zeppelin" {
		t.Fatalf("artist switching broken: %q", results[3].ArtistName)
	}
}

// guitaretabSearchPage mirrors the real guitaretab.com search markup:
// secondary (artist) links followed by primary (tab) links.
const guitaretabSearchPage = `<html><body>
<a href="/d/deep-purple/" class="gt-link gt-link--secondary" title="Deep Purple">Deep Purple</a>
<a href="/d/deep-purple/4950.html" class="gt-link gt-link--primary" title="Smoke On The Water tab">Smoke On The Water tab</a>
<a href="/d/deep-purple/" class="gt-link gt-link--secondary" title="Deep Purple">Deep Purple</a>
<a href="/d/deep-purple/289523.html" class="gt-link gt-link--primary" title="Smoke On The Water chords (ver 3)">Smoke On The Water chords (ver 3)</a>
</body></html>`

func TestTextTabSearchParsesGuitareTabPage(t *testing.T) {
	results, err := parseTextTabSearch([]byte(guitaretabSearchPage), "smoke on the water", textTabSites[1], "https://www.guitaretab.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d: %+v", len(results), results)
	}
	if results[0].SongName != "Smoke On The Water" || results[0].ArtistName != "Deep Purple" {
		t.Fatalf("first = %q by %q", results[0].SongName, results[0].ArtistName)
	}
	if results[0].Type != "Tab" || results[1].Type != "Chords" {
		t.Fatalf("types = %q, %q", results[0].Type, results[1].Type)
	}
	if results[1].TabURL != "https://www.guitaretab.com/d/deep-purple/289523.html" {
		t.Fatalf("TabURL = %q", results[1].TabURL)
	}
}

// TestTextTabSearchFiltersJunk guards against navigation links and unrelated
// songs polluting results.
func TestTextTabSearchFiltersJunk(t *testing.T) {
	page := `<html><body>
<a href="/submit.php">Submit tab</a>
<a href="/search.php?tabtype=any">Search</a>
<a href="/tabs/a/acdc/">AC/DC</a>
<a href="/tabs/a/acdc/back_in_black_tab.html">Back In Black Tab</a>
<a href="javascript:void(0)">Click</a>
</body></html>`
	results, err := parseTextTabSearch([]byte(page), "smoke on the water", textTabSites[0], "https://www.guitartabs.cc")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("unrelated song must be filtered, got %+v", results)
	}
}

// guitartabsTabPage mirrors the real page: a small title <pre>, then the tab
// inside <pre> with a legacy usenet header block.
const guitartabsTabPage = `<html><body>
<h1>Deep Purple - Smoke On The Water</h1>
<pre>Smoke On The Water Tab</pre>
<pre>#----------------------------------PLEASE NOTE---------------------------------#
#This file is the author's own work and represents their interpretation of the #
#song. You may only use this file for private study, scholarship, or research. #
#------------------------------------------------------------------------------##
From uunet!munnari.oz.au!uniwa!cujo!marsh!rob Mon Jun 22 11:42:54 PDT 1992
Article: 270 of alt.guitar.tab
Newsgroups: alt.guitar.tab
Subject: TAB: Smoke on the Water (all)
Date: Mon, 22 Jun 1992 14:29:22 GMT
Lines: 354

112 BPM

e|---------------------------------|-----------------|
B|---3---3---2---0---0---0---3---0-|---1---0---------|
G|---------------------------------|-----------------|
D|---------------------------------|-----------------|
A|---------------------------------|-----------------|
E|---------------------------------|-----------------|
:----0---3---5---0---0---3---6---5-:----3---3---2---0:|
</pre>
</body></html>`

func TestTextTabFetchExtractsPreAndParses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(guitartabsTabPage))
	}))
	defer srv.Close()

	c := newTextTabClient(&rateLimiter{})
	c.base = srv.URL
	tab, err := c.Fetch(SearchResult{
		Source:     SourceGuitarTabs,
		ID:         42,
		SongName:   "Smoke On The Water",
		ArtistName: "Deep Purple",
		TabURL:     srv.URL + "/tabs/d/deep_purple/smoke_on_the_water_tab.html",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(tab.Bars) == 0 {
		t.Fatal("expected parsed bars")
	}
	if tab.Title != "Smoke On The Water" || tab.Artist != "Deep Purple" {
		t.Fatalf("metadata backfill missing: %q by %q", tab.Title, tab.Artist)
	}
	if len(tab.Tuning) != 6 {
		t.Fatalf("tuning should be normalized to 6 strings, got %v (%s)", tab.Tuning, tab.Tuning.Label())
	}
	if bpm := tab.Metadata[model.MetaKeyBPM]; bpm != "112" {
		t.Fatalf("bpm = %q, want 112", bpm)
	}
}

// guitaretabTabPage mirrors the real page: tab rows wrapped in
// <span class="js-tab-row">.
const guitaretabTabPage = `<html><body>
<pre><div class="js-text-tab" style="display: inline-block"><span class="js-tab-row"  style="display: block">e|---0---0---0---0---|</span><span class="js-tab-row"  style="display: block">B|---1---1---1---1---|</span><span class="js-tab-row"  style="display: block">G|---2---2---2---2---|</span><span class="js-tab-row"  style="display: block">D|---2---2---2---2---|</span><span class="js-tab-row"  style="display: block">A|---0---0---0---0---|</span><span class="js-tab-row"  style="display: block">E|-------------------|</span></div></pre>
</body></html>`

func TestTextTabFetchGuitareTabSpans(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(guitaretabTabPage))
	}))
	defer srv.Close()

	c := newTextTabClient(&rateLimiter{})
	c.base = srv.URL
	tab, err := c.Fetch(SearchResult{
		Source: SourceGuitareTab, ID: 7,
		SongName: "Wonderwall", ArtistName: "Oasis",
		TabURL: srv.URL + "/d/oasis/123.html",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(tab.Bars) == 0 {
		t.Fatal("expected parsed bars")
	}
	if tab.Title != "Wonderwall" || tab.Artist != "Oasis" {
		t.Fatalf("metadata backfill missing: %q by %q", tab.Title, tab.Artist)
	}
}

// TestTextTabFetchRejectsChordOnly guards that lyric/chord pages (no playable
// bars) fail with a clear error instead of importing garbage.
func TestTextTabFetchRejectsChordOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body><pre>[Verse 1]
Some lyrics here with [Am] chords [C] [G]
And more lyrics [Dm] here</pre></body></html>`))
	}))
	defer srv.Close()

	c := newTextTabClient(&rateLimiter{})
	c.base = srv.URL
	_, err := c.Fetch(SearchResult{Source: SourceGuitarTabs, ID: 1, TabURL: srv.URL + "/tab.html"})
	if err == nil {
		t.Fatal("chord-only page must be rejected")
	}
	if !strings.Contains(err.Error(), "no playable bars") {
		t.Fatalf("error should explain the rejection, got %v", err)
	}
}

// TestTextTabFetchRejectsBadStatus guards the status check on tab pages.
func TestTextTabFetchRejectsBadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "gone", http.StatusNotFound)
	}))
	defer srv.Close()

	c := newTextTabClient(&rateLimiter{})
	c.base = srv.URL
	_, err := c.Fetch(SearchResult{Source: SourceGuitareTab, ID: 1, TabURL: srv.URL + "/tab.html"})
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("expected 404 error, got %v", err)
	}
}

func TestCleanupFetchedTabStripsUsenetHeaders(t *testing.T) {
	content := `#---- header ----#
From uunet!foo Mon Jun 22 1992
Article: 270 of alt.guitar.tab
Newsgroups: alt.guitar.tab
Path: nevada!uunet!munnari
Subject: TAB: Smoke on the Water
Lines: 354
112 BPM

e|---0---|
`
	got := cleanupFetchedTab(content)
	for _, junk := range []string{"From uunet", "Article:", "Newsgroups:", "Path:", "Subject:", "Lines:"} {
		if strings.Contains(got, junk) {
			t.Fatalf("usenet header %q survived cleanup:\n%s", junk, got)
		}
	}
	if !strings.Contains(got, "112 BPM") || !strings.Contains(got, "e|---0---|") {
		t.Fatalf("tab content must survive cleanup:\n%s", got)
	}
}

func TestNormalizeFetchedTuning(t *testing.T) {
	// Bars with 6 strings each; a misinferred 7-string tuning must become
	// standard.
	tab := &model.Tab{Tuning: model.ParseTuning("BEADGBE")}
	tab.Bars = []model.Bar{{Strings: make([]model.StringLine, 6)}, {Strings: make([]model.StringLine, 6)}}
	normalizeFetchedTuning(tab)
	if len(tab.Tuning) != 6 || tab.Tuning.Label() != "EADGBE" {
		t.Fatalf("tuning = %s, want EADGBE", tab.Tuning.Label())
	}
	// A genuinely 7-string tab must be left alone.
	tab7 := &model.Tab{Tuning: model.ParseTuning("BEADGBE")}
	tab7.Bars = []model.Bar{{Strings: make([]model.StringLine, 7)}}
	normalizeFetchedTuning(tab7)
	if len(tab7.Tuning) != 7 {
		t.Fatalf("7-string tuning must not be normalized, got %v", tab7.Tuning)
	}
}

// TestTextTabSearchMergesAcrossSites guards the end-to-end search across both
// wired text-tab sites via httptest.
func TestTextTabSearchMergesAcrossSites(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "search.php"):
			w.Write([]byte(guitartabsSearchPage))
		default:
			w.Write([]byte(guitaretabSearchPage))
		}
	}))
	defer srv.Close()

	c := newTextTabClient(&rateLimiter{})
	c.base = srv.URL
	results, err := c.Search("smoke on the water")
	if err != nil {
		t.Fatal(err)
	}
	sources := map[Source]int{}
	for _, r := range results {
		sources[r.Source]++
	}
	if sources[SourceGuitarTabs] != 4 || sources[SourceGuitareTab] != 2 {
		t.Fatalf("merged sources = %+v, want 4 guitartabs + 2 guitaretab", sources)
	}
}

// TestTextTabSongOnlyRetries guards the degraded query retries for song-only
// search engines (guitartabs.cc): an "artist song" query must still find tabs
// once the leading words are dropped.
func TestTextTabSongOnlyRetries(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		song := r.URL.Query().Get("song")
		if song == "smoke on the water" {
			w.Write([]byte(guitartabsSearchPage))
			return
		}
		w.Write([]byte(`<html><body><table class="tabslist"></table></body></html>`))
	}))
	defer srv.Close()

	c := newTextTabClient(&rateLimiter{})
	c.base = srv.URL
	results, err := c.Search("led zeppelin smoke on the water")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 4 {
		t.Fatalf("expected results from the degraded retry, got %d (requests=%d)", len(results), requests)
	}
	// guitartabs: full query + first-2-drop (which matches) = 2; guitaretab: 1.
	if requests != 3 {
		t.Fatalf("expected 3 requests total, got %d", requests)
	}
}

func TestDropWords(t *testing.T) {
	if got := dropFirstWords("dire straits sultans of swing", 2); got != "sultans of swing" {
		t.Fatalf("dropFirstWords = %q", got)
	}
	if got := dropLastWords("sultans of swing dire straits", 2); got != "sultans of swing" {
		t.Fatalf("dropLastWords = %q", got)
	}
	if got := dropFirstWords("smoke", 2); got != "smoke" {
		t.Fatalf("short query must survive, got %q", got)
	}
}

// TestTextTabFetchRejectsDrumTabs guards that drum tabs (no frets) are
// rejected up front with a clear message.
func TestTextTabFetchRejectsDrumTabs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(guitartabsTabPage))
	}))
	defer srv.Close()

	c := newTextTabClient(&rateLimiter{})
	c.base = srv.URL
	_, err := c.Fetch(SearchResult{Source: SourceGuitarTabs, ID: 1, Type: "Drum Tab", TabURL: srv.URL + "/x.html"})
	if err == nil || !strings.Contains(err.Error(), "drum") {
		t.Fatalf("drum tabs must be rejected, got %v", err)
	}
}
