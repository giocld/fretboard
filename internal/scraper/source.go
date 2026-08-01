package scraper

// Source identifies which online provider returned a search result.
type Source string

const (
	SourceUG        Source = "ug"
	SourceSongsterr Source = "songsterr"
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
}
