package parser

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/YOUR_USERNAME/fretboard/internal/model"
)

const techniqueChars = "hpb/\\~xsu"

func parseBarContent(s string) model.StringLine {
	var segs []model.Segment
	pos := 0
	runes := []rune(s)
	i := 0
	for i < len(runes) {
		r := runes[i]
		switch {
		case r >= '0' && r <= '9':
			numStr := string(r)
			j := i + 1
			for j < len(runes) && runes[j] >= '0' && runes[j] <= '9' {
				numStr += string(runes[j])
				j++
			}
			n, _ := strconv.Atoi(numStr)
			segs = append(segs, model.Segment{
				Char:     r,
				Value:    n,
				Position: pos,
				Width:    j - i,
			})
			pos += j - i
			i = j
		case r == '-':
			segs = append(segs, model.Segment{Char: '-', Value: 0, Position: pos, Width: 1})
			pos++
			i++
		case strings.ContainsRune(techniqueChars, r):
			segs = append(segs, model.Segment{Char: r, Value: 0, Position: pos, Width: 1})
			pos++
			i++
		case unicode.IsSpace(r) || r == '|':
			// Skip spaces and pipes (already handled at higher level).
			pos++
			i++
		default:
			// Unknown character: skip but advance position.
			pos++
			i++
		}
	}
	return model.StringLine{Segments: segs}
}
