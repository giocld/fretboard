package search

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"fretboard/internal/config"
	"fretboard/internal/scraper"
)

// Persistent search state: a small query history and a cache of the last
// successful result set, both stored in the config dir so searches survive
// restarts and flaky connections.

const (
	maxHistory = 8
	// cacheTTL is how long a cached result set is considered fresh. Older
	// caches are still served offline but flagged "(stale)" in the banner.
	cacheTTL = 7 * 24 * time.Hour
)

func historyPath() string {
	if dir, err := config.Dir(); err == nil {
		return filepath.Join(dir, "search_history.json")
	}
	return ""
}

func loadHistory() []string {
	path := historyPath()
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []string
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}

func saveHistory(history []string) {
	path := historyPath()
	if path == "" {
		return
	}
	history = history[:min(len(history), maxHistory)]
	data, _ := json.Marshal(history)
	_ = os.WriteFile(path, data, 0o644)
}

func addHistory(history []string, query string) []string {
	query = trimSpace(query)
	if query == "" {
		return history
	}
	var out []string
	for _, h := range history {
		if h == query {
			continue
		}
		out = append(out, h)
	}
	out = append([]string{query}, out...)
	out = out[:min(len(out), maxHistory)]
	return out
}

type cacheEntry struct {
	Query   string                 `json:"query"`
	SavedAt time.Time              `json:"saved_at"`
	Results []scraper.SearchResult `json:"results"`
}

func cachePath() string {
	if dir, err := config.Dir(); err == nil {
		return filepath.Join(dir, "search_cache.json")
	}
	return ""
}

func saveCache(query string, results []scraper.SearchResult) {
	path := cachePath()
	if path == "" || len(results) == 0 {
		return
	}
	data, _ := json.Marshal(cacheEntry{Query: trimSpace(query), SavedAt: time.Now(), Results: results})
	_ = os.WriteFile(path, data, 0o644)
}

func loadCache(query string) ([]scraper.SearchResult, bool) {
	res, _, ok := loadCacheEntry(query)
	return res, ok
}

// loadCacheEntry returns the cached result set for a query together with the
// time it was saved, so offline fallback can date the banner and age the
// cache. Caches written before the saved_at field existed load with a zero
// SavedAt (backward compatible).
func loadCacheEntry(query string) ([]scraper.SearchResult, time.Time, bool) {
	path := cachePath()
	if path == "" {
		return nil, time.Time{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, time.Time{}, false
	}
	var entry cacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, time.Time{}, false
	}
	if trimSpace(entry.Query) != trimSpace(query) || len(entry.Results) == 0 {
		return nil, time.Time{}, false
	}
	return entry.Results, entry.SavedAt, true
}

// isCacheStale reports whether a cached result set is older than the TTL.
// Zero (unknown) saved times are treated as fresh rather than flagged.
func isCacheStale(savedAt time.Time) bool {
	return !savedAt.IsZero() && time.Since(savedAt) > cacheTTL
}

// MergeCacheFresh combines a cached result set for a query with a freshly
// fetched one, deduplicating by result key and preferring the fresh copy of
// any duplicate. Rows only the cache knows about are preserved so a partial
// or page-shifted fetch does not truncate the earlier set.
func MergeCacheFresh(cached, fresh []scraper.SearchResult) []scraper.SearchResult {
	best := make(map[string]int, len(cached)+len(fresh))
	out := make([]scraper.SearchResult, 0, len(cached)+len(fresh))
	keep := func(r scraper.SearchResult) {
		key := scraper.ResultKey(r)
		if idx, ok := best[key]; ok {
			out[idx] = r // fresh wins: later writes replace earlier
			return
		}
		best[key] = len(out)
		out = append(out, r)
	}
	for _, r := range cached {
		keep(r)
	}
	for _, r := range fresh {
		keep(r)
	}
	return out
}

// trimSpace is a tiny helper so this file has no surprises.
func trimSpace(s string) string {
	start := 0
	for start < len(s) && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	end := len(s)
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}
