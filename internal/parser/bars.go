package parser

import (
	"strings"

	"fretboard/internal/model"
)

func extractBars(region []string, stringsPerColumn int) []model.Bar {
	if stringsPerColumn < 1 {
		return nil
	}

	var bars []model.Bar
	barNum := 1
	i := 0
	currentSection := ""
	for i < len(region) {
		for i < len(region) && strings.TrimSpace(region[i]) == "" {
			i++
		}
		if i >= len(region) {
			break
		}

		// A section header ("[Verse]", "Chorus:") names the bars that
		// follow until the next header.
		if sec := sectionHeader(region[i]); sec != "" {
			currentSection = sec
			i++
			continue
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
		for k := range chunkBars {
			chunkBars[k].Section = currentSection
		}
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
		bar := model.Bar{Number: barNum, Strings: make([]model.StringLine, stringsPerColumn), Rhythm: rhythmForBar(rhythm, start, end)}
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
			// A "1."/"2." ending marker is notation, not a note: strip it
			// from the content so it never plays as a fret.
			if idx := leadingEndingIndex(sliced[s]); idx >= 0 {
				sliced[s] = sliced[s][:idx] + sliced[s][idx+2:]
			}
		}
		bar.Strings = reverseAndParse(sliced)
		bar.RepeatStart, bar.RepeatEnd, bar.Ending = repeatMarkers(col, start, end)
		bars = append(bars, bar)
		barNum++
	}
	return bars
}

// repeatMarkers inspects the raw chunk lines for one bar's pipe-delimited
// range and reports its repeat structure: a "|" followed by ":" opens a
// repeat ("|:--0--|"), a ":" before the closing "|" closes one
// ("--0--:|"), and a leading "1." / "2." marks a first/second ending. The
// markers usually sit on the top string line, but every line is checked so
// tabs that repeat the marker on each line work too.
func repeatMarkers(col []string, start, end int) (repeatStart, repeatEnd bool, ending int) {
	for _, line := range col {
		if start < len(line) && line[start] == ':' {
			repeatStart = true
		}
		if end > 0 && end-1 < len(line) && line[end-1] == ':' {
			repeatEnd = true
		}
		if n := leadingEndingNumber(line, start, end); n > 0 {
			ending = n
		}
	}
	return repeatStart, repeatEnd, ending
}

// leadingEndingNumber looks for a "1." or "2." ending marker at the start
// of a bar's content (after optional spaces), e.g. "|1.---|" or "| 2.--|".
// String lines in a column can be shorter than the bar range taken from the
// top line, so the slice is bounds-guarded (this used to panic on real
// fetched tabs with ragged line lengths).
func leadingEndingNumber(line string, start, end int) int {
	if start < 0 || start >= len(line) {
		return 0
	}
	if end > len(line) {
		end = len(line)
	}
	i := leadingEndingIndex(line[start:end])
	if i < 0 {
		return 0
	}
	return int(line[start+i] - '0')
}

// leadingEndingIndex returns the index of the "N." ending marker within a
// bar's content slice, or -1 when there is none.
func leadingEndingIndex(content string) int {
	if content == "" {
		return -1
	}
	i := 0
	for i < len(content) && content[i] == ' ' {
		i++
	}
	if i+1 < len(content) && content[i] >= '1' && content[i] <= '2' && content[i+1] == '.' {
		return i
	}
	return -1
}

// sectionHeader recognizes a song-section header line: "[Verse 1]",
// "Chorus:", "[SOLO]", etc. Bracket form accepts any bracketed label;
// colon form requires a known section keyword so lyric-like lines in the
// tab region are never mistaken for headers.
func sectionHeader(line string) string {
	trim := strings.TrimSpace(line)
	if len(trim) > 40 {
		return ""
	}
	if len(trim) >= 3 && trim[0] == '[' && trim[len(trim)-1] == ']' {
		inner := strings.TrimSpace(trim[1 : len(trim)-1])
		if inner != "" && !strings.ContainsAny(inner, "|-") {
			return inner
		}
		return ""
	}
	colon := strings.IndexByte(trim, ':')
	if colon <= 0 || colon > 30 || strings.TrimSpace(trim[colon+1:]) != "" {
		return ""
	}
	name := strings.TrimSpace(trim[:colon])
	lower := strings.ToLower(name)
	for _, kw := range []string{
		"intro", "verse", "chorus", "bridge", "solo", "outro",
		"interlude", "pre-chorus", "prechorus", "riff", "instrumental",
		"middle 8", "coda", "breakdown", "tag", "ending",
	} {
		if strings.HasPrefix(lower, kw) {
			return name
		}
	}
	return ""
}

// rhythmForBar filters rhythm marks to the pipe-delimited range [start, end)
// of one bar and rebases them to bar-relative positions. Rhythm rows span the
// whole chunk (e.g. "| q  e  | h  q  |"); without rebasing, every bar in the
// chunk would be timed with the chunk-wide positions of the first bar. The
// string line's pipe grid defines the bars (and the content slicing), so the
// same [start, end) range is used for the marks, whose columns share the
// string line's origin.
func rhythmForBar(rhythm []model.RhythmMark, start, end int) []model.RhythmMark {
	if len(rhythm) == 0 {
		return nil
	}
	var out []model.RhythmMark
	for _, r := range rhythm {
		if r.Position >= start && r.Position < end {
			out = append(out, model.RhythmMark{Position: r.Position - start, Ticks: r.Ticks})
		}
	}
	return out
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
