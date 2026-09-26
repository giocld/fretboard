package scraper

import (
	"strings"
	"testing"
	"time"

	"fretboard/internal/model"
	"github.com/Pilfer/ultimate-guitar-scraper/pkg/ultimateguitar"
)

// UG chord pages open with lyrics and wrap chords in [ch] tags. The whole
// song must survive (trimming to the first tab-like line cut the start), a
// "Tabs" page that is really chords is still read, and a page that is empty
// once UG's markup is stripped is rejected on the path both fetch backends
// share (the raw-content emptiness check can't see through the tags).
func TestParseUGContentChordPages(t *testing.T) {
	// Real UG chord pages open with a title/video line, then a "Capo: 2nd
	// fret" line -- whose digit makes trimNonTabLines (built for tab pages,
	// which start with dashes) mistake it for the first line of tab
	// content and cut everything before it, here the "[Video]" line.
	page := "[Video]\nCapo: 2nd fret\n[Intro]\n[ch]Em7[/ch]  [ch]G[/ch]\n\n[Verse 1]\n[tab][ch]Em7[/ch]          [ch]G[/ch]\nToday is gonna be the day[/tab]\n"
	for _, typ := range []string{"Chords", "Tabs"} {
		tab, err := parseUGContent(page, typ, 1)
		if err != nil {
			t.Fatalf("%s page: %v", typ, err)
		}
		if tab.Metadata["kind"] != "chords" || !strings.HasPrefix(strings.TrimSpace(tab.Metadata["raw"]), "[Video]") {
			t.Fatalf("%s page: kind %q, raw starts %q", typ, tab.Metadata["kind"], firstLine(tab.Metadata["raw"]))
		}
	}
	if _, err := parseUGContent("[tab][/tab]\n[ch][/ch]\n", "Chords", 3); err == nil {
		t.Fatal("a page that is only markup must be rejected")
	}
	tab, err := parseUGContent("e|--0--|\nB|--1--|\nG|--0--|\nD|--2--|\nA|--3--|\nE|-----|\n", "Tabs", 4)
	if err != nil || len(tab.Bars) != 1 {
		t.Fatalf("plain tab: %v, %d bars", err, len(tab.Bars))
	}
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return line
}

// typedUGScraper answers each search with results of the requested type,
// and -- like the live API did for "wonderwall oasis" -- only chord sheets
// when asked for both types at once.
type typedUGScraper struct{ fakeUGScraper }

func (f *typedUGScraper) Search(p ultimateguitar.SearchParams) (ultimateguitar.SearchResult, error) {
	types := p.Type
	if len(types) > 1 {
		types = []ultimateguitar.TabType{ultimateguitar.TabTypeChords}
	}
	var tabs []ultimateguitar.Tab
	for _, typ := range types {
		name := map[ultimateguitar.TabType]string{ultimateguitar.TabTypeTabs: "Tabs", ultimateguitar.TabTypeChords: "Chords"}[typ]
		tabs = append(tabs, ultimateguitar.Tab{ID: int64(typ), SongName: "Song", Type: ultimateguitar.Type(name)})
	}
	return ultimateguitar.SearchResult{Tabs: tabs}, nil
}

// Chord pages are searched on their own request: asked together, UG's API
// can fill the page with chord sheets and push every tab out.
func TestUGSearchAsksForTabsAndChordsSeparately(t *testing.T) {
	client := &ugAPIClient{scraper: &typedUGScraper{}, rl: &rateLimiter{delay: time.Millisecond}}
	res, err := client.SearchPage("song", 1)
	if err != nil || len(res) != 2 || res[0].Type != "Tabs" || res[1].Type != "Chords" {
		t.Fatalf("want one tabs request then one chords request, got %+v (%v)", res, err)
	}
}

// UG's own song and artist names are authoritative for a UG page; the
// parser's guess from the first lines (a chord chart like "G 3-x-0-0-3-3")
// must not win over them.
func TestUGNamesOverrideParserGuess(t *testing.T) {
	tab := &model.Tab{Title: "G       3-x-0-0-3-3", Artist: "Em7     0-2-2-0-3-3"}
	applyUGMetadata(tab, ugTabMeta{SongName: "Wonderwall", ArtistName: "Oasis"})
	if tab.Title != "Wonderwall" || tab.Artist != "Oasis" {
		t.Fatalf("got %q by %q", tab.Title, tab.Artist)
	}
	kept := &model.Tab{Title: "From the file"}
	applyUGMetadata(kept, ugTabMeta{})
	if kept.Title != "From the file" {
		t.Fatalf("empty UG names must not erase the parsed title, got %q", kept.Title)
	}
}
