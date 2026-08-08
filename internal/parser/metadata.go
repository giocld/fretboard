package parser

import (
	"regexp"
	"strconv"
	"strings"

	"fretboard/internal/model"
)

var (
	titleRegex  = regexp.MustCompile(`(?i)^\s*(?:title|track|song)\s*[:\-]\s*(.+)$`)
	artistRegex = regexp.MustCompile(`(?i)^\s*(?:artist|by)\s*[:\-]\s*(.+)$`)
	tuningRegex = regexp.MustCompile(`(?i)^\s*tuning\s*[:\-]?\s*(.+)$`)
	capoRegex   = regexp.MustCompile(`(?i)capo\s*[:\-]?\s*(\d+)`)
	// Both "112 BPM" (guitartabs.cc style) and "BPM: 112" (UG style) are
	// common; either capture group may hold the number.
	bpmRegex = regexp.MustCompile(`(?i)\b(?:bpm\s*[:\-]?\s*(\d+)|\b(\d+)\s*bpm)\b`)
)

// extractMetadata scans the first non-blank lines for title, artist, tuning,
// capo, and bpm. Returns the index where scanning stopped (so we don't
// re-scan these as tab content).
func extractMetadata(lines []string, tab *model.Tab) int {
	scanLimit := len(lines)
	if scanLimit > 30 {
		scanLimit = 30
	}
	for i := 0; i < scanLimit; i++ {
		l := lines[i]
		trim := strings.TrimSpace(l)
		if trim == "" {
			continue
		}
		// Stop scanning once we hit something that looks like a tab line or a
		// section header — both belong to the tab region, not the header.
		if looksLikeRhythmLine(l) || looksLikeStringLine(l) || sectionHeader(l) != "" {
			return i
		}
		if m := titleRegex.FindStringSubmatch(l); m != nil {
			tab.Title = strings.TrimSpace(m[1])
			tab.Metadata[model.MetaKeyTitle] = tab.Title
			continue
		}
		if m := artistRegex.FindStringSubmatch(l); m != nil {
			tab.Artist = strings.TrimSpace(m[1])
			tab.Metadata[model.MetaKeyArtist] = tab.Artist
			continue
		}
		if m := tuningRegex.FindStringSubmatch(l); m != nil {
			tab.Metadata[model.MetaKeyTuningRaw] = strings.TrimSpace(m[1])
			continue
		}
		if m := capoRegex.FindStringSubmatch(l); m != nil {
			if n, err := strconv.Atoi(m[1]); err == nil {
				tab.Metadata[model.MetaKeyCapo] = strconv.Itoa(n)
			}
			continue
		}
		if m := bpmRegex.FindStringSubmatch(l); m != nil {
			for _, g := range m[1:] {
				if g != "" {
					tab.Metadata[model.MetaKeyBPM] = g
					break
				}
			}
			continue
		}
		// Heuristic: if this is the first non-blank line, treat as artist.
		// Second non-blank line, treat as title. This matches UG's
		// "Artist\nTitle\nTuning..." convention.
		if tab.Artist == "" {
			tab.Artist = trim
			tab.Metadata[model.MetaKeyArtist] = trim
			continue
		}
		if tab.Title == "" {
			tab.Title = trim
			tab.Metadata[model.MetaKeyTitle] = trim
			continue
		}
	}
	return scanLimit
}

// normalizeTabBPM copies tempo metadata into the canonical bpm key.
func normalizeTabBPM(tab *model.Tab) {
	if tab == nil {
		return
	}
	if tab.Metadata == nil {
		tab.Metadata = map[string]string{}
	}
	if strings.TrimSpace(tab.Metadata[model.MetaKeyBPM]) != "" {
		return
	}
	if tempo := strings.TrimSpace(tab.Metadata[model.MetaKeyTempo]); tempo != "" {
		tab.Metadata[model.MetaKeyBPM] = tempo
	}
}
