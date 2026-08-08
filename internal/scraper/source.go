package scraper

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
