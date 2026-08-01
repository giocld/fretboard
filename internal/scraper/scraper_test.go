package scraper

import (
	"time"
	"encoding/json"
	"testing"
)

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
		"Eric Clapton":  "eric-clapton",
		"Layla":         "layla",
		"  AC/DC  ":     "ac-dc",
		"Queen - Live":  "queen-live",
		"O'Reilly":      "o-reilly",
		"123":           "123",
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
	c := newUGHTMLClient(time.Millisecond)
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
