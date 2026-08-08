// Package scraper provides online tab sources: Ultimate Guitar (API + HTML
// fallback) and Songsterr search.
package scraper

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"fretboard/internal/model"
	"fretboard/internal/parser"
	"github.com/Pilfer/ultimate-guitar-scraper/pkg/ultimateguitar"
)

type ugAPIClient struct {
	scraper ugScraper
	rl      *rateLimiter
}

// ugScraper is the subset of the Ultimate Guitar scraper the API client
// needs, so tests can substitute a fake.
type ugScraper interface {
	Search(params ultimateguitar.SearchParams) (ultimateguitar.SearchResult, error)
	GetTabByID(id int64) (ultimateguitar.TabResult, error)
}

// ugRequestTimeout bounds UG API requests. The library's default client has
// no timeout, so a hung request would block the TUI spinner (and its tea.Cmd
// goroutine) forever; the HTML and Songsterr backends already set their own.
const ugRequestTimeout = 30 * time.Second

func newUGAPIClient(rl *rateLimiter) *ugAPIClient {
	s := ultimateguitar.New()
	if s.Client != nil {
		s.Client.Timeout = ugRequestTimeout
	}
	return &ugAPIClient{scraper: &s, rl: rl}
}

// Search queries Ultimate Guitar API for tabs matching query.
func (c *ugAPIClient) Search(query string) ([]SearchResult, error) {
	return c.SearchPage(query, 1)
}

// SearchPage queries a specific page of Ultimate Guitar results.
func (c *ugAPIClient) SearchPage(query string, page int) ([]SearchResult, error) {
	if page < 1 {
		page = 1
	}
	c.rl.throttle()
	res, err := c.scraper.Search(ultimateguitar.SearchParams{
		Title: query,
		Type:  []ultimateguitar.TabType{ultimateguitar.TabTypeTabs},
		Page:  int32(page),
	})
	if err != nil {
		return nil, fmt.Errorf("ug search: %w", err)
	}
	var out []SearchResult
	for _, t := range res.Tabs {
		out = append(out, SearchResult{
			ID:         t.ID,
			Source:     SourceUG,
			SongName:   t.SongName,
			ArtistName: string(t.ArtistName),
			Type:       string(t.Type),
			Rating:     t.Rating,
			Votes:      t.Votes,
		})
	}
	return out, nil
}

// Fetch retrieves a tab by its Ultimate Guitar ID and parses it into the
// internal model.
func (c *ugAPIClient) Fetch(id int64) (*model.Tab, error) {
	c.rl.throttle()
	res, err := c.scraper.GetTabByID(id)
	if err != nil {
		return nil, fmt.Errorf("ug fetch %d: %w", id, err)
	}
	content := res.Content
	if strings.TrimSpace(content) == "" {
		return nil, fmt.Errorf("ug fetch %d: empty tab content", id)
	}
	if isAlbumTab(res) {
		return nil, fmt.Errorf("ug fetch %d: album page (%q) is not a single tab", id, res.Part)
	}
	content = normalizeContent(content)
	tab, err := parser.Parse(strings.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf("parse fetched tab: %w", err)
	}
	applyUGMetadata(tab, ugTabMeta{
		SongName:   res.SongName,
		ArtistName: res.ArtistName,
		Tuning:     res.Tuning,
		Capo:       res.Capo,
	})
	if res.VersionDescription != nil {
		if bpm := model.ParseBPMFromText(*res.VersionDescription); bpm > 0 {
			tab.Metadata[model.MetaKeyBPM] = strconv.Itoa(bpm)
		}
	}
	model.NormalizeTabBPM(tab)
	return tab, nil
}

// ugTabMeta carries the UG-provided metadata both fetch backends apply to a
// parsed tab.
type ugTabMeta struct {
	SongName   string
	ArtistName string
	Tuning     string
	Capo       int
}

// applyUGMetadata backfills parsed-tab metadata (title, artist, tuning, capo)
// from the UG page data.
func applyUGMetadata(tab *model.Tab, res ugTabMeta) {
	if tab.Title == "" {
		tab.Title = res.SongName
	}
	if tab.Artist == "" {
		tab.Artist = res.ArtistName
	}
	if len(tab.Tuning) == 0 && res.Tuning != "" {
		tab.Tuning = model.ParseTuning(strings.ReplaceAll(res.Tuning, " ", ""))
	}
	if res.Capo > 0 {
		if tab.Metadata == nil {
			tab.Metadata = map[string]string{}
		}
		tab.Metadata[model.MetaKeyCapo] = strconv.Itoa(res.Capo)
	}
	if tab.Metadata == nil {
		tab.Metadata = map[string]string{}
	}
}

// isAlbumTab reports whether a UG result is an album/multi-track page, whose
// content is a track listing ("Track 01 - X … Included") rather than a single
// tab. Importing those produces garbage tabs.
func isAlbumTab(res ultimateguitar.TabResult) bool {
	if res.Part == "album" {
		return true
	}
	content := res.Content
	if len(content) > 400 {
		content = content[:400]
	}
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "Track ") && (strings.Contains(trimmed, "Included") || strings.Contains(trimmed, "Not Included")) {
			return true
		}
	}
	return false
}

func normalizeContent(s string) string {
	// Ordered, not a map: replacement order must be deterministic so
	// double-encoded sequences like "&amp;quot;" always decode the same way.
	replacements := []struct{ old, new string }{
		{"&quot;", "\""},
		{"&#039;", "'"},
		{"&lt;", "<"},
		{"&gt;", ">"},
		{"&nbsp;", " "},
		{"&amp;", "&"},
		{"[ch]", ""},
		{"[/ch]", ""},
		{"[tab]", ""},
		{"[/tab]", ""},
	}
	for _, r := range replacements {
		s = strings.ReplaceAll(s, r.old, r.new)
	}
	return trimNonTabLines(s)
}

func trimNonTabLines(s string) string {
	lines := strings.Split(s, "\n")
	start := 0
	for i, line := range lines {
		if hasTabLineContent(line) {
			start = i
			break
		}
	}
	return strings.Join(lines[start:], "\n")
}

func hasTabLineContent(line string) bool {
	for _, r := range line {
		if r >= '0' && r <= '9' || r == '-' || r == '|' {
			return true
		}
	}
	return false
}
