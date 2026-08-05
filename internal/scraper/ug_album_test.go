package scraper

import (
	"strings"
	"testing"
	"time"

	"github.com/Pilfer/ultimate-guitar-scraper/pkg/ultimateguitar"
)

type fakeUGScraper struct {
	tab   ultimateguitar.TabResult
	tabID int64
	err   error
}

func (f *fakeUGScraper) Search(ultimateguitar.SearchParams) (ultimateguitar.SearchResult, error) {
	return ultimateguitar.SearchResult{}, nil
}

func (f *fakeUGScraper) GetTabByID(id int64) (ultimateguitar.TabResult, error) {
	return f.tab, f.err
}

const validTabContent = "Tuning: E Standard\n\ne|0-3-5|\nB|------|\nG|------|\nD|------|\nA|------|\nE|------|\n"

func TestFetchRejectsAlbumPart(t *testing.T) {
	client := &ugAPIClient{
		scraper: &fakeUGScraper{tab: ultimateguitar.TabResult{Part: "album", Content: validTabContent}},
		rl:      &rateLimiter{delay: time.Millisecond},
	}
	tab, err := client.Fetch(1)
	if err == nil {
		t.Fatal("expected error for album part, got tab")
	}
	if tab != nil {
		t.Fatalf("expected nil tab, got %+v", tab)
	}
	if !strings.Contains(err.Error(), "album") {
		t.Fatalf("error should mention album, got %q", err)
	}
}

func TestFetchAcceptsRegularTab(t *testing.T) {
	client := &ugAPIClient{
		scraper: &fakeUGScraper{tab: ultimateguitar.TabResult{Part: "", Content: validTabContent, SongName: "Test Song", ArtistName: "Test Artist"}},
		rl:      &rateLimiter{delay: time.Millisecond},
	}
	tab, err := client.Fetch(1)
	if err != nil {
		t.Fatal(err)
	}
	if tab == nil || tab.Title == "" {
		t.Fatal("expected a parsed tab")
	}
}

func TestIsAlbumTabDetectsTrackListing(t *testing.T) {
	cases := []struct {
		name string
		res  ultimateguitar.TabResult
		want bool
	}{
		{"album part", ultimateguitar.TabResult{Part: "album"}, true},
		{"track listing in content", ultimateguitar.TabResult{
			Content: "Track 01 - Sultans of Swing       - Included\nTrack 02 - Lady Writer - Not Included - Anyone?",
		}, true},
		{"regular content", ultimateguitar.TabResult{Content: validTabContent}, false},
		{"empty", ultimateguitar.TabResult{}, false},
	}
	for _, tc := range cases {
		if got := isAlbumTab(tc.res); got != tc.want {
			t.Errorf("%s: isAlbumTab = %v, want %v", tc.name, got, tc.want)
		}
	}
}
