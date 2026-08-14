package scraper

import "testing"

// TestUGTabURLTabsType pins the "Tabs" and unknown-type branches of ugTabURL.
// TestUGTabURLSlugPattern only covers "Chords"; both other branches must still
// resolve to the "tabs" path in the URL.
func TestUGTabURLTabsType(t *testing.T) {
	r := SearchResult{ID: 1, ArtistName: "AC/DC", SongName: "Thunderstruck", Type: "Tabs"}
	want := "https://tabs.ultimate-guitar.com/tab/ac-dc/thunderstruck-tabs-1"
	if got := ugTabURL(r); got != want {
		t.Fatalf("ugTabURL(Tabs) = %q, want %q", got, want)
	}
	r.Type = "Bass"
	if got := ugTabURL(r); got != want {
		t.Fatalf("ugTabURL(unknown type) = %q, want %q", got, want)
	}
}
