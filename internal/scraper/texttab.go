package scraper

import (
	"fmt"
	"hash/fnv"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
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

// normalizeFetchedTuning corrects a common inference failure on old-style
// tabs: lines like ":----X----:" (repeat marks) count as string lines, which
// can push the detected string count from 6 to 7. When the bars' modal string
// count is 6, the tuning is standard.
func normalizeFetchedTuning(tab *model.Tab) {
	if tab == nil || len(tab.Bars) == 0 || len(tab.Tuning) == 0 {
		return
	}
	counts := map[int]int{}
	for _, b := range tab.Bars {
		if n := len(b.Strings); n > 0 {
			counts[n]++
		}
	}
	mode, best := 0, 0
	for n, c := range counts {
		if c > best {
			best, mode = c, n
		}
	}
	if mode == 6 && len(tab.Tuning) != 6 {
		tab.Tuning = model.Standard
	}
}

var (
	anchorRegex = regexp.MustCompile(`(?is)<a\b[^>]*\bhref="([^"]+)"[^>]*>(.*?)</a>`)
	titleAttr   = regexp.MustCompile(`(?i)\btitle="([^"]*)"`)
	tagStrip    = regexp.MustCompile(`(?s)<[^>]*>`)
)

// textTabNavWords are root-level anchors on the search page that must never
// be mistaken for artists.
var textTabNavWords = map[string]bool{
	"tabs": true, "updates": true, "top 100": true, "upcoming": true,
	"submit tab": true, "guitar tabs": true, "privacy": true, "terms of use": true,
	"lyricsmars": true,
}

// parseTextTabSearch extracts artist -> tab anchor pairs from a search page.
// Both wired sites emit the artist link (a directory) immediately before its
// tab links (a .html page), so a single pass over the anchors works.
func parseTextTabSearch(body []byte, query string, site *textTabSite, origin string) ([]SearchResult, error) {
	terms := queryTerms(query)
	page := string(body)
	var out []SearchResult
	seen := map[string]bool{}
	artist := ""
	for _, m := range anchorRegex.FindAllStringSubmatch(page, -1) {
		href := html.UnescapeString(m[1])
		text := strings.TrimSpace(tagStrip.ReplaceAllString(m[2], ""))
		if text == "" {
			continue
		}
		lower := strings.ToLower(text)
		if strings.HasPrefix(lower, "http") || strings.HasPrefix(href, "javascript:") || strings.HasPrefix(href, "#") {
			continue
		}
		if strings.HasSuffix(href, "/") {
			// Directory link: the current artist (unless it's navigation).
			if !textTabNavWords[lower] && len(href) > 1 {
				artist = text
			}
			continue
		}
		if !strings.HasSuffix(href, ".html") && !strings.HasSuffix(href, ".php") {
			continue
		}
		if isTextTabNavHref(href) {
			continue
		}
		if artist == "" {
			continue
		}
		title := ""
		if tm := titleAttr.FindStringSubmatch(m[0]); tm != nil {
			title = html.UnescapeString(tm[1])
		}
		full := strings.ToLower(text + " " + title)
		if !matchesQueryTerms(full, terms) {
			continue
		}
		u := href
		if !strings.HasPrefix(u, "http") {
			u = origin + href
		}
		if seen[u] {
			continue
		}
		seen[u] = true
		out = append(out, SearchResult{
			ID:         int64(fnv32(u)),
			Source:     site.source,
			SongName:   cleanSongName(text),
			ArtistName: artist,
			Type:       tabTypeOf(full, href),
			TabURL:     u,
		})
		if len(out) >= 15 {
			break
		}
	}
	return out, nil
}

// queryTerms returns the meaningful words of a query (len >= 3).
func queryTerms(query string) []string {
	var terms []string
	for _, w := range strings.Fields(strings.ToLower(query)) {
		if len(w) >= 3 {
			terms = append(terms, w)
		}
	}
	return terms
}

func matchesQueryTerms(s string, terms []string) bool {
	if len(terms) == 0 {
		return true
	}
	for _, t := range terms {
		if strings.Contains(s, t) {
			return true
		}
	}
	return false
}

func isTextTabNavHref(href string) bool {
	lower := strings.ToLower(href)
	for _, frag := range []string{"/search", "/submit", "/login", "/updates", "/top_tabs", "/rss/", "/tabfaq", "/terms", "/privacy", "/my/", "/i/", "/js/"} {
		if strings.Contains(lower, frag) {
			return true
		}
	}
	return false
}

// cleanSongName strips trailing " tab/chords/bass" descriptors from the anchor
// text so the library shows "Smoke On The Water" instead of "Smoke On The
// Water Tab".
func cleanSongName(text string) string {
	lower := strings.ToLower(text)
	for _, suf := range []string{" tab", " chords", " bass tab", " drum tab", " ver ", " (ver ", " intro", " chorus", " solo"} {
		if i := strings.Index(lower, suf); i > 0 {
			return strings.TrimSpace(text[:i])
		}
	}
	return strings.TrimSpace(text)
}

func tabTypeOf(full, href string) string {
	switch {
	case strings.Contains(full, "bass"):
		return "Bass Tab"
	case strings.Contains(full, "drum"):
		return "Drum Tab"
	case strings.Contains(full, "chord") || strings.Contains(href, "_crd") || strings.Contains(href, "chords"):
		return "Chords"
	default:
		return "Tab"
	}
}

// extractPreTabs pulls the ASCII tab out of a tab page: guitartabs.cc puts
// the content directly in <pre>; guitaretab.com wraps each line in
// <span class="js-tab-row">. Blocks without tab content (page titles in
// <pre>) are skipped.
func extractPreTabs(body []byte, site *textTabSite) string {
	page := string(body)
	var blocks []string
	start := 0
	for {
		open := strings.Index(page[start:], "<pre")
		if open < 0 {
			break
		}
		open += start
		tagEnd := strings.IndexByte(page[open:], '>')
		if tagEnd < 0 {
			break
		}
		open += tagEnd + 1
		closeIdx := strings.Index(page[open:], "</pre>")
		if closeIdx < 0 {
			break
		}
		closeIdx += open
		blocks = append(blocks, page[open:closeIdx])
		start = closeIdx + 6
	}
	var out []string
	for _, b := range blocks {
		// guitaretab wraps rows in spans; unwrap them to plain lines.
		if site != nil && site.source == SourceGuitareTab {
			rows := regexp.MustCompile(`(?s)<span[^>]*class="js-tab-row"[^>]*>(.*?)</span>`).FindAllStringSubmatch(b, -1)
			var text []string
			for _, r := range rows {
				text = append(text, tagStrip.ReplaceAllString(r[1], ""))
			}
			if joined := strings.Join(text, "\n"); hasTabLineContent(joined) {
				out = append(out, joined)
			}
			continue
		}
		text := tagStrip.ReplaceAllString(b, "")
		if hasTabLineContent(text) {
			out = append(out, text)
		}
	}
	return html.UnescapeString(strings.Join(out, "\n"))
}

// cleanupFetchedTab drops the legacy usenet header block old tabs carry
// (gtabs content is often reposted from alt.guitar.tab), `:`-prefixed repeat
// marker rows (old-style tabs duplicate string rows with ':--X--:' lines),
// and any leading junk up to the first real tab line.
func cleanupFetchedTab(s string) string {
	lines := strings.Split(s, "\n")
	var kept []string
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if isUsenetHeader(trimmed) {
			continue
		}
		if strings.HasPrefix(trimmed, ":") && (strings.Contains(trimmed, "|") || strings.Contains(trimmed, "-")) {
			continue
		}
		kept = append(kept, l)
	}
	return strings.Join(kept, "\n")
}

func isUsenetHeader(l string) bool {
	lower := strings.ToLower(l)
	for _, prefix := range []string{
		"from ", "article:", "newsgroups:", "path:", "sender:",
		"organization:", "date:", "lines:", "message-id:", "subject:",
		"nntp-", "x-", "in-reply-to:", "references:", "reply-to:",
	} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

func fnv32(s string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return h.Sum32()
}
