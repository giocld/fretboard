package scraper

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"fretboard/internal/model"
)

type songsterrClient struct {
	http *http.Client
	rl   *rateLimiter
}

func newSongsterrClient(rl *rateLimiter) *songsterrClient {
	return &songsterrClient{
		http: &http.Client{Timeout: 30 * time.Second},
		rl:   rl,
	}
}

// songsterrTrack is one playable version of a song on Songsterr, shared by
// the search response decode and the instrument picker.
type songsterrTrack struct {
	Instrument string `json:"instrument"`
	Views      int    `json:"views"`
}

// SearchPage queries Songsterr with a per-page result size (page 1 = 15,
// page 2 = 30, ...). Songsterr has no real pagination, so later pages simply
// ask for more matches.
func (c *songsterrClient) SearchPage(query string, page int) ([]SearchResult, error) {
	if page < 1 {
		page = 1
	}
	c.rl.throttle()
	size := 15 * page
	u := "https://www.songsterr.com/api/songs?pattern=" + url.QueryEscape(query) + "&size=" + strconv.Itoa(size)
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", ugBrowserUA)
	req.Header.Set("Accept", "application/json")

	res, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("songsterr search: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("songsterr search: status %d", res.StatusCode)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	var songs []struct {
		SongID int64            `json:"songId"`
		Artist string           `json:"artist"`
		Title  string           `json:"title"`
		Tracks []songsterrTrack `json:"tracks"`
	}
	if err := json.Unmarshal(body, &songs); err != nil {
		return nil, fmt.Errorf("songsterr decode: %w", err)
	}
	var out []SearchResult
	for _, s := range songs {
		inst := pickSongsterrInstrument(s.Tracks)
		out = append(out, SearchResult{
			ID:            s.SongID,
			Source:        SourceSongsterr,
			SongName:      s.Title,
			ArtistName:    s.Artist,
			Type:          inst,
			Reconstructed: true, // Songsterr serves no tab text; Fetch rebuilds it via UG
			SourceURL:     songsterrSongURL(s.SongID),
		})
	}
	return out, nil
}

// songsterrSongURL returns the canonical Songsterr page for a song id.
func songsterrSongURL(id int64) string {
	return fmt.Sprintf("https://www.songsterr.com/a/wsa/%d", id)
}

func pickSongsterrInstrument(tracks []songsterrTrack) string {
	best := "Guitar"
	bestViews := -1
	for _, t := range tracks {
		inst := strings.ToLower(t.Instrument)
		if strings.Contains(inst, "guitar") || strings.Contains(inst, "bass") {
			if t.Views > bestViews {
				bestViews = t.Views
				best = t.Instrument
			}
		}
	}
	return best
}

// Fetch resolves a Songsterr hit by searching UG for the same song.
func (c *songsterrClient) Fetch(id int64, artist, title string, ug *Client) (*model.Tab, error) {
	if ug == nil {
		return nil, fmt.Errorf("songsterr tab %d: fetch requires UG fallback (not configured)", id)
	}
	q := strings.TrimSpace(artist + " " + title)
	if q == "" {
		return nil, fmt.Errorf("songsterr tab %d: missing artist/title for fallback", id)
	}
	results, err := ug.Search(q)
	if err != nil {
		return nil, fmt.Errorf("songsterr fallback search: %w", err)
	}
	for _, r := range results {
		if r.Source != SourceUG {
			continue
		}
		tab, err := ug.Fetch(r)
		if err == nil {
			// ug.Fetch always runs applyUGMetadata, which initializes
			// Metadata, so it is safe to write keys directly.
			tab.Metadata[model.MetaKeySource] = "songsterr-via-ug"
			tab.Metadata[model.MetaKeySongsterrID] = fmt.Sprintf("%d", id)
			return tab, nil
		}
	}
	return nil, fmt.Errorf("songsterr tab %d: no matching UG tab for %q", id, q)
}
