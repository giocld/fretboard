package scraper

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"fretboard/internal/model"
)

// Client wraps multiple online tab sources with rate limiting.
type Client struct {
	ug        *ugAPIClient
	ugHTML    *ugHTMLClient
	songsterr *songsterrClient
	textTabs  *textTabClient
}

// NewClient creates a scraper client. delay controls the minimum time between
// network requests to avoid rate limiting; the limiter is shared by every
// backend so back-to-back requests across sources are spaced out too.
func NewClient(delay time.Duration) *Client {
	rl := &rateLimiter{delay: delay}
	return &Client{
		ug:        newUGAPIClient(rl),
		ugHTML:    newUGHTMLClient(rl),
		songsterr: newSongsterrClient(rl),
		textTabs:  newTextTabClient(rl),
	}
}

// Search queries online sources: UG API, then UG HTML (fallback), Songsterr,
// and the plain-text tab sites (guitartabs.cc, guitaretab.com), merged and
// deduplicated.
func (c *Client) Search(query string) ([]SearchResult, error) {
	return c.SearchPage(query, 1)
}

// SearchPage searches every backend for one page of results (page 1 = the
// default search; later pages ask UG for its next page and Songsterr for a
// larger result set; sources without pagination return their single page,
// which callers deduplicate against the existing list).
func (c *Client) SearchPage(query string, page int) ([]SearchResult, error) {
	var results []SearchResult
	var lastErr error

	if c.ug != nil {
		res, err := c.ug.SearchPage(query, page)
		if err == nil && len(res) > 0 {
			results = append(results, res...)
		} else if err != nil {
			lastErr = err
		}
	}
	if page <= 1 && len(results) == 0 && c.ugHTML != nil {
		res, err := c.ugHTML.Search(query)
		if err == nil && len(res) > 0 {
			results = append(results, res...)
		} else if err != nil && lastErr == nil {
			lastErr = err
		}
	}
	if c.songsterr != nil {
		st, err := c.songsterr.SearchPage(query, page)
		if err == nil {
			results = mergeSearchResults(results, st)
		} else if len(results) == 0 && lastErr == nil {
			lastErr = err
		}
	}
	if c.textTabs != nil {
		tt, err := c.textTabs.Search(query)
		if err == nil {
			results = mergeSearchResults(results, tt)
		} else if len(results) == 0 && lastErr == nil {
			lastErr = err
		}
	}
	if len(results) == 0 && lastErr != nil {
		return nil, lastErr
	}
	return results, nil
}

// Fetch retrieves a tab using the result's source.
func (c *Client) Fetch(result SearchResult) (*model.Tab, error) {
	switch result.Source {
	case SourceSongsterr:
		if c.songsterr == nil {
			return nil, fmt.Errorf("songsterr client not configured")
		}
		return c.songsterr.Fetch(result.ID, result.ArtistName, result.SongName, c)
	case SourceGuitarTabs, SourceGuitareTab:
		if c.textTabs == nil {
			return nil, fmt.Errorf("text-tab client not configured")
		}
		return c.textTabs.Fetch(result)
	default:
		if c.ug != nil {
			tab, err := c.ug.Fetch(result.ID)
			if err == nil {
				return tab, nil
			}
		}
		if c.ugHTML != nil {
			return c.ugHTML.Fetch(result)
		}
		return nil, fmt.Errorf("no fetch backend configured")
	}
}

// resultScore ranks a search result for display: tabs beat chord sheets,
// high ratings and vote counts beat anonymous uploads, and well-known
// sources beat obscure archives. Higher is better. The score drives the
// merged list order so the official, top-rated tab of a song surfaces
// above low-rated covers and chord sheets.
func resultScore(r SearchResult) int {
	score := 0
	switch {
	case IsTabType(r):
		score += 1000
	case strings.EqualFold(r.Type, "chords") || strings.EqualFold(r.Type, "chord"):
		score += 200
	}
	if r.Rating > 0 {
		score += int(r.Rating * 100)
	}
	if r.Votes > 0 {
		v := r.Votes
		if v > 2000 {
			v = 2000
		}
		score += int(v / 20)
	}
	switch r.Source {
	case SourceUG:
		score += 50
	case SourceSongsterr:
		score += 30
	case SourceGuitarTabs:
		score += 20
	case SourceGuitareTab:
		score += 10
	}
	return score
}

// MergeResults combines two result sets, deduplicating by source + artist +
// song + type and keeping the higher-rated copy of a duplicate. The merged
// list is ranked best-first (tabs, then rating/votes/source trust).
func MergeResults(primary, extra []SearchResult) []SearchResult {
	return mergeSearchResults(primary, extra)
}

func mergeSearchResults(primary, extra []SearchResult) []SearchResult {
	best := make(map[string]int) // result key -> index into out
	var out []SearchResult
	keep := func(r SearchResult) {
		key := resultKey(r)
		if idx, ok := best[key]; ok {
			// Same song from the same source twice (e.g. UG API + HTML):
			// keep the higher-rated copy.
			if resultScore(r) > resultScore(out[idx]) {
				out[idx] = r
			}
			return
		}
		best[key] = len(out)
		out = append(out, r)
	}
	for _, r := range primary {
		keep(r)
	}
	for _, r := range extra {
		keep(r)
	}
	// Best match first: tabs with the strongest ratings from the most
	// trusted sources float to the top; ties keep source order.
	sort.SliceStable(out, func(i, j int) bool { return resultScore(out[i]) > resultScore(out[j]) })
	return out
}

func resultKey(r SearchResult) string {
	// Type is part of the key so a tab and a chord sheet of the same song
	// stay distinct rows; cross-source duplicates of the same performance
	// type still collapse onto one row.
	return string(r.Source) + "|" + r.ArtistName + "|" + r.SongName + "|" + r.Type
}
