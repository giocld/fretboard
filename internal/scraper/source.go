package scraper

import (
	"fmt"
	"strings"
)

// Source identifies which online provider returned a search result.
type Source string

const (
	SourceUG         Source = "ug"
	SourceSongsterr  Source = "songsterr"
	SourceGuitarTabs Source = "guitartabs" // guitartabs.cc
	SourceGuitareTab Source = "guitaretab" // guitaretab.com
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
	// TabURL is the canonical page URL when the provider exposes one (UG's
	// current pages use slug-based URLs that cannot be derived from the ID).
	TabURL string
	// Pro marks results whose full tab is behind the UG Pro paywall
	// (official versions and pro-only tabs). They rank below fetchable
	// community tabs, and FetchBest falls back to a community copy when
	// the direct fetch fails.
	Pro bool
	// Reconstructed marks results whose tab is not served by the provider
	// itself (e.g. Songsterr): fetching them rebuilds the tab from another
	// source, so provenance differs from a direct fetch.
	Reconstructed bool
	// SourceURL is the canonical page for the result when the provider
	// exposes one beyond TabURL (e.g. Songsterr's song page).
	SourceURL string
	// PickReason explains why a different result than the one the user
	// selected was actually opened (e.g. the UG Pro paywall fallback).
	PickReason string
}

// SourceBadge renders a compact provenance label for a result, e.g. "[UG *4.9]".
// The rating is included when the source reports one (Ultimate Guitar).
func SourceBadge(r SearchResult) string {
	label := ""
	switch r.Source {
	case SourceUG:
		label = "UG"
	case SourceSongsterr:
		label = "ST"
	case SourceGuitarTabs:
		label = "GT"
	case SourceGuitareTab:
		label = "GR"
	default:
		label = string(r.Source)
	}
	if r.Rating > 0 {
		label += fmt.Sprintf(" *%.1f", r.Rating)
	}
	return "[" + label + "]"
}

// IsTopRated reports whether a result is a strongly-rated tab (used for the
// "top" badge and for ranking): a high rating backed by a meaningful number
// of votes.
func IsTopRated(r SearchResult) bool {
	return r.Rating >= 4.0 && r.Votes >= 25
}

// IsTabType reports whether the result is a tablature (not a chord sheet).
func IsTabType(r SearchResult) bool {
	switch strings.ToLower(r.Type) {
	case "tabs", "tab", "tab pro", "pro", "bass", "bass tabs":
		return true
	}
	return false
}

// isProAccess reports whether a UG access type gates the full tab behind the
// UG Pro paywall. "public" and "user" uploads are free; "pro" and "official"
// versions are Pro-only.
func isProAccess(access string) bool {
	switch strings.ToLower(strings.TrimSpace(access)) {
	case "pro", "official":
		return true
	}
	return false
}

// isProType reports whether a UG tab type is a Pro/official-only version.
func isProType(t string) bool {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "pro", "official", "tab pro":
		return true
	}
	return false
}
