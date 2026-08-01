package parser

import (
	"strings"

	"github.com/YOUR_USERNAME/fretboard/internal/model"
)

func extractBars(region []string, stringsPerColumn int) []model.Bar {
	if stringsPerColumn < 1 {
		return nil
	}

	var bars []model.Bar
	barNum := 1
	i := 0
	for i < len(region) {
		for i < len(region) && strings.TrimSpace(region[i]) == "" {
			i++
		}
		if i >= len(region) {
			break
		}

		var rhythmLine string
		if looksLikeRhythmLine(region[i]) {
			rhythmLine = region[i]
			i++
		}

		col := make([]string, 0, stringsPerColumn)
		for j := 0; j < stringsPerColumn && i+j < len(region); j++ {
			if strings.TrimSpace(region[i+j]) == "" {
				break
			}
			if !looksLikeStringLine(region[i+j]) {
				break
			}
			col = append(col, region[i+j])
		}
		if len(col) == 0 {
			i++
			continue
		}

		rhythmMarks := parseRhythmMarks(rhythmLine)
		chunkBars := barsFromColumn(col, stringsPerColumn, barNum, rhythmMarks)
		bars = append(bars, chunkBars...)
		barNum += len(chunkBars)
		i += len(col)
	}
	return bars
}

func parseRhythmMarks(line string) []model.RhythmMark {
	if line == "" {
		return nil
	}
	parsed := parseRhythmLine(line)
	out := make([]model.RhythmMark, len(parsed))
	for i, r := range parsed {
		out[i] = model.RhythmMark{Position: r.Position, Ticks: r.Ticks}
	}
	return out
}

func barsFromColumn(col []string, stringsPerColumn, startNum int, rhythm []model.RhythmMark) []model.Bar {
	var bars []model.Bar
	barNum := startNum
	positions := pipePositions(col[0])
	if len(positions) < 2 {
		bar := model.Bar{Number: barNum, Strings: reverseAndParse(col), Rhythm: rhythm}
		return []model.Bar{bar}
	}
	for j := 0; j < len(positions)-1; j++ {
		start := positions[j] + 1
		end := positions[j+1]
		bar := model.Bar{Number: barNum, Strings: make([]model.StringLine, stringsPerColumn), Rhythm: rhythm}
		sliced := make([]string, len(col))
		for s, line := range col {
			if start >= len(line) {
				sliced[s] = ""
				continue
			}
			stop := end
			if stop > len(line) {
				stop = len(line)
			}
			sliced[s] = line[start:stop]
		}
		bar.Strings = reverseAndParse(sliced)
		bars = append(bars, bar)
		barNum++
	}
	return bars
}

// reverseAndParse reverses the slice of string lines so that the lowest-
// pitched ASCII line (conventionally the last line) becomes index 0, then
// parses each one into a model.StringLine. This makes Bar.Strings align with
// model.Tuning, where index 0 is the lowest-pitched string.
func reverseAndParse(lines []string) []model.StringLine {
	out := make([]model.StringLine, len(lines))
	for i, line := range lines {
		out[len(lines)-1-i] = parseBarContent(line)
	}
	return out
}

// pipePositions returns the byte indices of bar boundaries (`|` characters)
// in the line, collapsing adjacent pipes (`||`) into a single boundary.
func pipePositions(s string) []int {
	var out []int
	for i := 0; i < len(s); i++ {
		if s[i] == '|' {
			out = append(out, i)
			for i+1 < len(s) && s[i+1] == '|' {
				i++
			}
		}
	}
	return out
}
