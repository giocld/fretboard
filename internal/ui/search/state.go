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
	if len(history) > maxHistory {
		history = history[:maxHistory]
	}
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
	if len(out) > maxHistory {
		out = out[:maxHistory]
	}
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
	path := cachePath()
	if path == "" {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var entry cacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, false
	}
	if trimSpace(entry.Query) != trimSpace(query) || len(entry.Results) == 0 {
		return nil, false
	}
	return entry.Results, true
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
