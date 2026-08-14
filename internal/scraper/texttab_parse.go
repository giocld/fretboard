package scraper

import (
	"hash/fnv"
	"html"
	"regexp"
	"strings"

	"fretboard/internal/model"
)

// normalizeFetchedTuning corrects a common inference failure on old-style
// tabs: lines like ":----X----:" (repeat marks) count as string lines, which
// can push the detected string count from 6 to 7. When the bars' modal string
// count is 6, the tuning is standard.
func normalizeFetchedTuning(tab *model.Tab) {
	if len(tab.Bars) == 0 || len(tab.Tuning) == 0 {
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
	anchorRegex        = regexp.MustCompile(`(?is)<a\b[^>]*\bhref="([^"]+)"[^>]*>(.*?)</a>`)
	titleAttr          = regexp.MustCompile(`(?i)\btitle="([^"]*)"`)
	tagStrip           = regexp.MustCompile(`(?s)<[^>]*>`)
	guitaretabRowRegex = regexp.MustCompile(`(?s)<span[^>]*class="js-tab-row"[^>]*>(.*?)</span>`)
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
		if site.source == SourceGuitareTab {
			rows := guitaretabRowRegex.FindAllStringSubmatch(b, -1)
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
