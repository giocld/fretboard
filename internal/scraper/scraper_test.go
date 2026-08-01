package scraper

import (
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
