package scraper

import "testing"

// TestResultScorePrefersTabsAndRating pins the ranking priority: tabs (by
// rating) outrank chord sheets — the type gate is hard so a strongly rated
// official tab beats everything, and any tab beats any chord sheet.
func TestResultScorePrefersTabsAndRating(t *testing.T) {
	cover := SearchResult{Source: SourceGuitareTab, SongName: "Sultans of Swing", ArtistName: "Random Dude", Type: "Tabs", Rating: 1.0}
	chords := SearchResult{Source: SourceUG, SongName: "Sultans of Swing", ArtistName: "Dire Straits", Type: "Chords", Rating: 4.0, Votes: 500}
	official := SearchResult{Source: SourceUG, SongName: "Sultans of Swing", ArtistName: "Dire Straits", Type: "Tabs", Rating: 4.9, Votes: 2100}

	sc := resultScore(cover)
	ch := resultScore(chords)
	of := resultScore(official)
	if !(of > sc && sc > ch) {
		t.Fatalf("expected official(%d) > cover-tab(%d) > chords(%d)", of, sc, ch)
	}
}

// TestMergeSearchResultsSortsBestFirst verifies the merged list surfaces the
// highest-rated copy of a duplicated result and orders tabs (by rating)
// before chord sheets, before everything else.
func TestMergeSearchResultsSortsBestFirst(t *testing.T) {
	primary := []SearchResult{
		{Source: SourceUG, SongName: "Sultans", ArtistName: "Dire Straits", Type: "Tabs", Rating: 1.5, Votes: 3},
		{Source: SourceGuitareTab, SongName: "Sultans", ArtistName: "Cover Band", Type: "Tabs"},
	}
	extra := []SearchResult{
		{Source: SourceUG, SongName: "Sultans", ArtistName: "Dire Straits", Type: "Tabs", Rating: 4.9, Votes: 2100},
		{Source: SourceUG, SongName: "Sultans", ArtistName: "Dire Straits", Type: "Chords", Rating: 4.2, Votes: 900},
	}

	merged := mergeSearchResults(primary, extra)
	if len(merged) != 3 {
		t.Fatalf("expected 3 merged results, got %d: %+v", len(merged), merged)
	}
	if merged[0].Rating != 4.9 || merged[0].Type != "Tabs" {
		t.Fatalf("duplicate should be replaced by the higher-rated copy, got %+v", merged[0])
	}
	if merged[1].ArtistName != "Cover Band" {
		t.Fatalf("anonymous cover tab should rank second (tabs > chords by type gate), got %+v", merged[1])
	}
	if merged[2].Type != "Chords" {
		t.Fatalf("chord sheet should rank last, got %+v", merged[2])
	}
}

// TestMergeKeepsTabAndChordOfSameSong verifies a tab and a chord sheet of the
// same song from the same source are distinct rows (the dedup key includes
// the performance type).
func TestMergeKeepsTabAndChordOfSameSong(t *testing.T) {
	a := SearchResult{Source: SourceUG, SongName: "Wonderwall", ArtistName: "Oasis", Type: "Tabs", Rating: 4.8, Votes: 900}
	b := SearchResult{Source: SourceUG, SongName: "Wonderwall", ArtistName: "Oasis", Type: "Chords", Rating: 4.5, Votes: 1200}
	merged := mergeSearchResults([]SearchResult{a}, []SearchResult{b})
	if len(merged) != 2 {
		t.Fatalf("tab and chord rows must both survive, got %+v", merged)
	}
	if merged[0].Type != "Tabs" || merged[1].Type != "Chords" {
		t.Fatalf("tab should rank before chords, got %+v", merged)
	}
}

func TestSourceBadgeFormats(t *testing.T) {
	cases := []struct {
		r    SearchResult
		want string
	}{
		{SearchResult{Source: SourceUG, Rating: 4.9}, "[UG ★4.9]"},
		{SearchResult{Source: SourceUG}, "[UG]"},
		{SearchResult{Source: SourceSongsterr}, "[ST]"},
		{SearchResult{Source: SourceGuitarTabs}, "[GT]"},
		{SearchResult{Source: SourceGuitareTab}, "[GR]"},
	}
	for _, c := range cases {
		if got := SourceBadge(c.r); got != c.want {
			t.Fatalf("SourceBadge(%+v) = %q, want %q", c.r, got, c.want)
		}
	}
}

func TestIsTopRated(t *testing.T) {
	if !IsTopRated(SearchResult{Rating: 4.9, Votes: 2100}) {
		t.Fatal("4.9 with 2100 votes should be top-rated")
	}
	if IsTopRated(SearchResult{Rating: 3.5, Votes: 2100}) {
		t.Fatal("3.5 stars must not be top-rated")
	}
	if IsTopRated(SearchResult{Rating: 4.9, Votes: 3}) {
		t.Fatal("a handful of votes must not be top-rated")
	}
	if IsTopRated(SearchResult{Rating: 0, Votes: 0}) {
		t.Fatal("unrated results must not be top-rated")
	}
}
