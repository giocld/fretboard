package parser

import (
	"strings"
	"unicode"
)

// looksLikeStringLine returns true if the line has a high density of
// tab-relevant characters (digits, hyphens, pipes, slashes, etc.).
func looksLikeStringLine(l string) bool {
	if !strings.Contains(l, "-") {
		return false
	}
	tally := 0
	for _, r := range l {
		switch {
		case r == '-' || r == '|' || r == '/' || r == '\\':
			tally++
		case r >= '0' && r <= '9':
			tally++
		case unicode.IsLetter(r):
			if r == 'e' || r == 'B' || r == 'G' || r == 'D' || r == 'A' || r == 'E' || r == 'C' || r == 'F' {
				tally++
			}
		}
	}
	nonSpace := 0
	for _, r := range l {
		if !unicode.IsSpace(r) {
			nonSpace++
		}
	}
	if nonSpace == 0 {
		return false
	}
	return tally*2 >= nonSpace
}

// findTabRegion returns [start, end) indices spanning all tab blocks in the file.
// UG tabs often repeat a 6-line block per measure separated by blank lines.
func findTabRegion(lines []string, startFrom int) (int, int) {
	bestStart, bestEnd := -1, -1
	i := startFrom
	for i < len(lines) {
		for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
			i++
		}
		runStart := i
		count := 0
		for i < len(lines) && looksLikeStringLine(lines[i]) {
			count++
			i++
		}
		if count > 0 {
			actualStart := runStart
			prev := actualStart - 1
			for prev >= startFrom && strings.TrimSpace(lines[prev]) == "" {
				prev--
			}
			// A rhythm row or a section header directly above the block
			// belongs to it — pull it into the region.
			if prev >= startFrom && looksLikeRhythmLine(lines[prev]) {
				actualStart = prev
			} else if prev >= startFrom && sectionHeader(lines[prev]) != "" {
				actualStart = prev
			}
			if bestStart < 0 {
				bestStart = actualStart
				bestEnd = i
			} else {
				bestEnd = i
			}
		}
		if count == 0 {
			i++
		}
	}
	return bestStart, bestEnd
}

// countStrings returns the most common run-length of consecutive string lines.
func countStrings(lines []string, start, end int) int {
	best := 0
	i := start
	for i < end {
		for i < end && strings.TrimSpace(lines[i]) == "" {
			i++
		}
		count := 0
		for i < end && looksLikeStringLine(lines[i]) {
			count++
			i++
		}
		if count > best {
			best = count
		}
		if count == 0 {
			i++
		}
	}
	return best
}
