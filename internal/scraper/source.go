package scraper

import (
	"fmt"
	"strings"
)

// Source identifies which online provider returned a search result.
type Source string

const (
	SourceUG         Source = "ug"
	SourceSongsterr  Source = "songsterr"
	SourceGuitarTabs Source = "guitartabs" // guitartabs.cc
	SourceGuitareTab Source = "guitaretab" // guitaretab.com
)

// SearchResult is a single tab found online.
type SearchResult struct {
	ID         int64
	Source     Source
	SongName   string
	ArtistName string
	Type       string
	Rating     float64
	Votes      int64
	// TabURL is the canonical page URL when the provider exposes one (UG's
	// current pages use slug-based URLs that cannot be derived from the ID).
	TabURL string
}

// SourceBadge renders a compact provenance label for a result, e.g. "[UG *4.9]".
// The rating is included when the source reports one (Ultimate Guitar).
func SourceBadge(r SearchResult) string {
	label := ""
	switch r.Source {
	case SourceUG:
		label = "UG"
	case SourceSongsterr:
		label = "ST"
	case SourceGuitarTabs:
		label = "GT"
	case SourceGuitareTab:
		label = "GR"
	default:
		label = string(r.Source)
	}
	if r.Rating > 0 {
		label += fmt.Sprintf(" *%.1f", r.Rating)
	}
	return "[" + label + "]"
}

// IsTopRated reports whether a result is a strongly-rated tab (used for the
// "top" badge and for ranking): a high rating backed by a meaningful number
// of votes.
func IsTopRated(r SearchResult) bool {
	return r.Rating >= 4.0 && r.Votes >= 25
}

// IsTabType reports whether the result is a tablature (not a chord sheet).
func IsTabType(r SearchResult) bool {
	switch strings.ToLower(r.Type) {
	case "tabs", "tab", "tab pro", "pro", "bass", "bass tabs":
		return true
	}
	return false
}
