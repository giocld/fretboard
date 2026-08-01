package scraper

import (
	"fmt"
	"time"

	"github.com/YOUR_USERNAME/fretboard/internal/model"
)

// Client wraps multiple online tab sources with rate limiting.
type Client struct {
	ug        *ugAPIClient
	ugHTML    *ugHTMLClient
	songsterr *songsterrClient
	delay     time.Duration
}

// NewClient creates a scraper client. delay controls the minimum time between
// network requests to avoid rate limiting.
func NewClient(delay time.Duration) *Client {
	return &Client{
		ug:        newUGAPIClient(delay),
		ugHTML:    newUGHTMLClient(delay),
		songsterr: newSongsterrClient(delay),
		delay:     delay,
	}
}

// Search queries online sources. UG API is tried first, then UG HTML; Songsterr
// results are merged when available.
func (c *Client) Search(query string) ([]SearchResult, error) {
	var results []SearchResult
	var lastErr error

	if c.ug != nil {
		res, err := c.ug.Search(query)
		if err == nil && len(res) > 0 {
			results = append(results, res...)
		} else if err != nil {
			lastErr = err
		}
	}
	if len(results) == 0 && c.ugHTML != nil {
		res, err := c.ugHTML.Search(query)
		if err == nil && len(res) > 0 {
			results = append(results, res...)
		} else if err != nil && lastErr == nil {
			lastErr = err
		}
	}
	if c.songsterr != nil {
		st, err := c.songsterr.Search(query)
		if err == nil {
			results = mergeSearchResults(results, st)
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
	default:
		if c.ug != nil {
			tab, err := c.ug.Fetch(result.ID)
			if err == nil {
				return tab, nil
			}
		}
		if c.ugHTML != nil {
			return c.ugHTML.Fetch(result.ID)
		}
		return nil, fmt.Errorf("no fetch backend configured")
	}
}

func mergeSearchResults(primary, extra []SearchResult) []SearchResult {
	seen := make(map[string]bool)
	var out []SearchResult
	for _, r := range primary {
		key := resultKey(r)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, r)
	}
	for _, r := range extra {
		key := resultKey(r)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, r)
	}
	return out
}

func resultKey(r SearchResult) string {
	return string(r.Source) + "|" + r.ArtistName + "|" + r.SongName
}
