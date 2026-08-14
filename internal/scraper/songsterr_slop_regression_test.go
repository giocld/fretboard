package scraper

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"fretboard/internal/model"

	"github.com/Pilfer/ultimate-guitar-scraper/pkg/ultimateguitar"
)

// songsterrRoundTripper serves a canned HTTP response for any request, so the
// Songsterr client's decode path can run without a network round trip.
type songsterrRoundTripper struct {
	code int
	body string
}

func (rt *songsterrRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: rt.code,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(rt.body)),
	}, nil
}

// songsterrUGScraper fakes the UG backend behind a Client so the Songsterr
// fallback in Fetch can be exercised end to end.
type songsterrUGScraper struct {
	search ultimateguitar.SearchResult
	tab    ultimateguitar.TabResult
}

func (f *songsterrUGScraper) Search(ultimateguitar.SearchParams) (ultimateguitar.SearchResult, error) {
	return f.search, nil
}

func (f *songsterrUGScraper) GetTabByID(int64) (ultimateguitar.TabResult, error) {
	return f.tab, nil
}

// TestSongsterrSearchPageDecodes pins the /api/songs decode: songId/artist/
// title map to a SearchResult and the most-viewed guitar/bass track becomes
// the result Type (non-guitar instruments are ignored).
func TestSongsterrSearchPageDecodes(t *testing.T) {
	body := `[{"songId":100,"artist":"Dire Straits","title":"Sultans of Swing","tracks":[
		{"instrument":"Piano","views":900},
		{"instrument":"Guitar","views":200},
		{"instrument":"Bass","views":300}
	]}]`
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
	if r.ID != 100 || r.SongName != "Sultans of Swing" || r.ArtistName != "Dire Straits" {
		t.Fatalf("unexpected result: %+v", r)
	}
	if r.Source != SourceSongsterr {
		t.Fatalf("source = %q, want %q", r.Source, SourceSongsterr)
	}
	if r.Type != "Bass" {
		t.Fatalf("type = %q, want the highest-viewed guitar/bass track (Bass)", r.Type)
	}
}

// TestSongsterrSearchPageDefaultsInstrument pins the "Guitar" default when no
// track is a guitar/bass part.
func TestSongsterrSearchPageDefaultsInstrument(t *testing.T) {
	body := `[{"songId":1,"artist":"A","title":"B","tracks":[{"instrument":"Piano","views":10}]}]`
	c := &songsterrClient{
		http: &http.Client{Transport: &songsterrRoundTripper{code: http.StatusOK, body: body}},
		rl:   &rateLimiter{delay: 0},
	}
	res, err := c.SearchPage("x", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].Type != "Guitar" {
		t.Fatalf("type = %q, want default Guitar", res[0].Type)
	}
}

// TestSongsterrFetchStampsFallbackMetadata pins the UG fallback in Fetch:
// a successful fallback stamps source + songsterr_id onto the tab metadata.
func TestSongsterrFetchStampsFallbackMetadata(t *testing.T) {
	ug := &Client{
		ug: &ugAPIClient{
			scraper: &songsterrUGScraper{
				search: ultimateguitar.SearchResult{Tabs: []ultimateguitar.Tab{
					{ID: 1, SongName: "Layla", ArtistName: "Eric Clapton"},
				}},
				tab: ultimateguitar.TabResult{Content: validTabContent, SongName: "Layla", ArtistName: "Eric Clapton"},
			},
			rl: &rateLimiter{delay: 0},
		},
	}
	sc := &songsterrClient{}
	tab, err := sc.Fetch(42, "Eric Clapton", "Layla", ug)
	if err != nil {
		t.Fatal(err)
	}
	if tab.Metadata[model.MetaKeySource] != "songsterr-via-ug" {
		t.Fatalf("source = %q, want %q", tab.Metadata[model.MetaKeySource], "songsterr-via-ug")
	}
	if tab.Metadata[model.MetaKeySongsterrID] != "42" {
		t.Fatalf("songsterr_id = %q, want %q", tab.Metadata[model.MetaKeySongsterrID], "42")
	}
}

// TestSongsterrFetchRequiresUGFallback pins the nil-ug guard and the
// missing-artist/title validation in Fetch.
func TestSongsterrFetchRequiresUGFallback(t *testing.T) {
	sc := &songsterrClient{}
	if _, err := sc.Fetch(1, "a", "b", nil); err == nil {
		t.Fatal("expected error for nil UG fallback")
	}
	if _, err := sc.Fetch(1, "   ", "", &Client{}); err == nil {
		t.Fatal("expected error for empty artist/title")
	}
}
