// Package parser converts ASCII guitar tablature into model.Tab structures.
//
// The parser is forgiving: real-world tabs are messy. It runs in two passes:
//
//	Pass 1: locate the tab region in the file, extract metadata (title,
//	        artist, tuning, capo) from the lines above it.
//	Pass 2: walk the tab region string by string, splitting on `|` bar
//	        delimiters, and emit model.Segment for each meaningful character.
package parser

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/YOUR_USERNAME/fretboard/internal/model"
)

// Parse reads a tab from r and returns a structured model.Tab.
func Parse(r io.Reader) (*model.Tab, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 8*1024*1024) // up to 8MB lines
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	return parseLines(lines)
}

// ParseFile opens a file and parses it as an ASCII tab.
func ParseFile(path string) (*model.Tab, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Parse(f)
}

func parseLines(lines []string) (*model.Tab, error) {
	cleaned := stripCommentsAndBlanks(lines)
	tab := &model.Tab{Metadata: map[string]string{}}

	// Pass 1: extract metadata from header lines.
	metaEnd := extractMetadata(cleaned, tab)

	// Locate the tab region (the block of consecutive string lines).
	tabStart, tabEnd := findTabRegion(cleaned, metaEnd)
	if tabStart < 0 || tabEnd <= tabStart {
		// No tab region found. Return what we have.
		tab.Tuning = model.Standard
		return tab, nil
	}

	// Infer tuning from the number of consecutive string lines in a row.
	stringsPerColumn := countStrings(cleaned, tabStart, tabEnd)
	tab.Tuning = inferTuning(tab, stringsPerColumn)

	// Pass 2: split the tab region into bar chunks and extract segments.
	tab.Bars = extractBars(cleaned[tabStart:tabEnd], stringsPerColumn)
	normalizeTabBPM(tab)
	return tab, nil
}

func stripCommentsAndBlanks(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed == "" {
			out = append(out, "")
			continue
		}
		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") {
			continue
		}
		out = append(out, l)
	}
	return out
}


// ParsePath dispatches to the ASCII parser or Guitar Pro converter based on
// file extension.
func ParsePath(path string) (*model.Tab, error) {
	if IsGpFile(path) {
		return ParseGPFile(path)
	}
	return ParseFile(path)
}
