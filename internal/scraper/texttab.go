package scraper

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"fretboard/internal/model"
	"fretboard/internal/parser"
)

// textTabSite describes a tab site whose pages carry plain ASCII tabs inside
// <pre> blocks and whose search pages list artist -> tab anchor pairs. Two
// sites are wired up (guitartabs.cc and guitaretab.com); adding another is a
// few lines.
type textTabSite struct {
	source     Source
	origin     string
	searchPath string // fmt.Sprintf-style; %s = URL-escaped query
	songOnly   bool   // site searches the query against song names only
}

var textTabSites = []*textTabSite{
	{
		source:     SourceGuitarTabs,
		origin:     "https://www.guitartabs.cc",
		searchPath: "/search.php?tabtype=any&band=&song=%s",
		songOnly:   true,
	},
	{
		source:     SourceGuitareTab,
		origin:     "https://www.guitaretab.com",
		searchPath: "/fetch/?type=tab&query=%s",
	},
}

// textTabClient scrapes the plain-text tab sites. Search pages are parsed
// into artist/tab pairs; tab pages have their <pre> content extracted and fed
// to the same ASCII parser used for local files.
type textTabClient struct {
	http *http.Client
	rl   *rateLimiter
	base string // test seam: overrides the site origin
}

func newTextTabClient(rl *rateLimiter) *textTabClient {
	return &textTabClient{
		http: &http.Client{Timeout: 30 * time.Second},
		rl:   rl,
	}
}

// siteURL returns the absolute URL for a path, honoring the test seam.
func (c *textTabClient) siteURL(site *textTabSite, path string) string {
	if c.base != "" {
		return c.base + path
	}
	return site.origin + path
}

// Search queries every wired text-tab site and merges the results.
func (c *textTabClient) Search(query string) ([]SearchResult, error) {
	var all []SearchResult
	var lastErr error
	for _, site := range textTabSites {
		queries := []string{query}
		if site.songOnly {
			// Song-only search engines miss "Song Artist" and "Artist Song"
			// queries; retry with the leading/trailing word pairs dropped.
			queries = append(queries, dropFirstWords(query, 2), dropLastWords(query, 2))
		}
		for _, q := range queries {
			c.rl.throttle()
			u := c.siteURL(site, fmt.Sprintf(site.searchPath, url.QueryEscape(q)))
			res, err := c.http.Get(u)
			if err != nil {
				lastErr = err
				continue
			}
			body, readErr := io.ReadAll(res.Body)
			res.Body.Close()
			if readErr != nil {
				lastErr = readErr
				continue
			}
			if res.StatusCode != http.StatusOK {
				lastErr = fmt.Errorf("%s search: status %d", site.source, res.StatusCode)
				continue
			}
			results, err := parseTextTabSearch(body, query, site, c.siteURL(site, ""))
			if err != nil {
				lastErr = err
				continue
			}
			all = append(all, results...)
			if len(results) > 0 {
				break // full query matched; no need for the degraded retries
			}
		}
	}
	if len(all) == 0 && lastErr != nil {
		return nil, lastErr
	}
	return all, nil
}

// dropFirstWords / dropLastWords return the query minus n leading/trailing
// words, for song-only search engines.
func dropFirstWords(query string, n int) string {
	fields := strings.Fields(query)
	if len(fields) <= n {
		return query
	}
	return strings.Join(fields[n:], " ")
}

func dropLastWords(query string, n int) string {
	fields := strings.Fields(query)
	if len(fields) <= n {
		return query
	}
	return strings.Join(fields[:len(fields)-n], " ")
}

// Fetch retrieves and parses a text-tab page.
func (c *textTabClient) Fetch(r SearchResult) (*model.Tab, error) {
	site := textTabSiteBySource(r.Source)
	if site == nil {
		return nil, fmt.Errorf("%s: unknown text-tab site", r.Source)
	}
	c.rl.throttle()
	u := r.TabURL
	if u == "" {
		return nil, fmt.Errorf("%s fetch %d: missing tab URL", r.Source, r.ID)
	}
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", ugBrowserUA)
	res, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s fetch %d: %w", r.Source, r.ID, err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s fetch %d: http %d", r.Source, r.ID, res.StatusCode)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	content := extractPreTabs(body, site)
	if strings.TrimSpace(content) == "" {
		return nil, fmt.Errorf("%s fetch %d: no tab content on page", r.Source, r.ID)
	}
	if strings.Contains(strings.ToLower(r.Type), "drum") {
		return nil, fmt.Errorf("%s fetch %d: drum tabs are not supported", r.Source, r.ID)
	}
	content = cleanupFetchedTab(content)
	tab, err := parser.Parse(strings.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf("parse fetched tab: %w", err)
	}
	if len(tab.Bars) == 0 {
		return nil, fmt.Errorf("%s fetch %d: chord sheet with no playable bars (chord-only tabs not yet supported)", r.Source, r.ID)
	}
	// Text-tab pages rarely carry a clean header (section markers like
	// "INTRO" or usenet text win), so the search result's own song/artist
	// naming is authoritative.
	tab.Title = strings.TrimSpace(r.SongName)
	tab.Artist = strings.TrimSpace(r.ArtistName)
	normalizeFetchedTuning(tab)
	if tab.Metadata == nil {
		tab.Metadata = map[string]string{}
	}
	tab.Metadata[model.MetaKeySource] = string(r.Source)
	model.NormalizeTabBPM(tab)
	return tab, nil
}

func textTabSiteBySource(s Source) *textTabSite {
	for _, site := range textTabSites {
		if site.source == s {
			return site
		}
	}
	return nil
}
