package kit

import (
	"fmt"
	"strings"

	"fretboard/internal/model"
	"github.com/charmbracelet/lipgloss"
)

func maxBarCols(bar model.Bar) int {
	m := 0
	for _, str := range bar.Strings {
		for _, seg := range str.Segments {
			m = max(m, seg.Position+seg.Width)
		}
	}
	return m
}

func renderTabGrid(tab *model.Tab, width int, offset int, cur *TabCursor, withHeader bool) string {
	if tab == nil || len(tab.Bars) == 0 {
		return "No tab loaded."
	}

	var b strings.Builder
	if withHeader && tab.Title != "" {
		b.WriteString(FretDigitStyle.Render(tab.Title))
		if tab.Artist != "" {
			b.WriteString("  " + RestStyle.Render(tab.Artist))
		}
		if tab.Tuning.Label() != "" {
			b.WriteString("  " + InfoStyle.Render(tab.Tuning.Label()))
		}
		if bpm := tab.Metadata[model.MetaKeyBPM]; bpm != "" {
			b.WriteString("  " + MutedStyle.Render(bpm+" BPM"))
		}
		b.WriteString("\n\n")
	}

	metrics := BarGridLayout(tab, width)
	barWidth := metrics.BarWidth

	for rowStart := 0; rowStart < len(tab.Bars); rowStart += metrics.BarsPerRow {
		rowEnd := min(rowStart+metrics.BarsPerRow, len(tab.Bars))
		renderBarRow(&b, tab, rowStart, rowEnd, barWidth, offset, cur)
		b.WriteString("\n")
	}
	return b.String()
}

func renderBarRow(b *strings.Builder, tab *model.Tab, start, end, barWidth, offset int, cur *TabCursor) {
	for barIdx := start; barIdx < end; barIdx++ {
		bar := tab.Bars[barIdx]
		highlight := cur != nil && barIdx == cur.Bar

		// Repeat structure markers: "|:" opens a repeat, ":|" closes one,
		// "1."/"2." label endings — mirrored on the bar header so the
		// player sees the same structure the playback follows.
		open, close, ending := barMarkers(bar)
		header := sectionHeaderFor(tab, barIdx, open, ending, close, barWidth)
		headerStyle := MutedStyle
		switch {
		case highlight:
			headerStyle = CursorStyle
		case cur.InLoop(barIdx):
			headerStyle = LoopBarStyle
		case cur.InSearch(barIdx):
			headerStyle = SearchBarStyle
		}
		b.WriteString(headerStyle.Render(padToWidth(header, barWidth)))
	}
	b.WriteString("\n")

	for s := 0; s < stringsPerRow(tab, start, end); s++ {
		for barIdx := start; barIdx < end; barIdx++ {
			bar := tab.Bars[barIdx]
			label := tab.Tuning.NoteName(s)
			if label == "" {
				label = "?"
			}
			style := StringLabel.Copy().Foreground(StringColor(s))
			prefix := MutedStyle.Render("│") + style.Render(label[:1]) + MutedStyle.Render("│")
			var line model.StringLine
			if s < len(bar.Strings) {
				line = bar.Strings[s]
			}
			highlight := cur != nil && barIdx == cur.Bar
			content := renderStringContent(line, s, offset, curCol(cur, highlight), highlight, tab, cur)
			// Pad the content to the bar column width so string rows line up
			// under the bar headers; the prefix width is measured because the
			// string label style has its own fixed width.
			b.WriteString(prefix + padToWidth(content, barWidth-lipgloss.Width(prefix)))
		}
		b.WriteString("\n")
	}
}

func stringsPerRow(tab *model.Tab, start, end int) int {
	m := 0
	for i := start; i < end && i < len(tab.Bars); i++ {
		m = max(m, len(tab.Bars[i].Strings))
	}
	if m == 0 {
		return 6
	}
	return m
}

func padToWidth(s string, width int) string {
	pad := width - lipgloss.Width(s)
	if pad <= 0 {
		return s
	}
	return s + strings.Repeat(" ", pad)
}

func curCol(cur *TabCursor, highlight bool) int {
	if !highlight || cur == nil {
		return -1
	}
	return cur.Col
}

func renderStringContent(line model.StringLine, stringIdx, offset, cursorCol int, highlight bool, tab *model.Tab, cur *TabCursor) string {
	maxCol := 0
	for _, seg := range line.Segments {
		maxCol = max(maxCol, seg.Position+seg.Width)
	}
	// The vertical playhead must appear on every string of the highlighted
	// bar, even rows whose content ends before the cursor column — a line
	// with a short rest still crosses the current beat.
	if highlight {
		maxCol = max(maxCol, cursorCol+1)
	}

	var b strings.Builder
	for col := offset; col < maxCol; {
		if highlight && col == cursorCol {
			b.WriteString(PlayheadStyle.Render("┊"))
			col++
			continue
		}
		seg, ok := segmentAt(line, col)
		if !ok {
			b.WriteString(" ")
			col++
			continue
		}
		rendered := renderSegment(seg, stringIdx, tab, cur)
		if highlight && cursorCol >= seg.Position && cursorCol < seg.Position+seg.Width {
			rendered = CursorStyle.Render(rendered)
		}
		b.WriteString(rendered)
		col = seg.Position + seg.Width
	}
	return b.String()
}

func segmentAt(line model.StringLine, col int) (model.Segment, bool) {
	for _, seg := range line.Segments {
		if col >= seg.Position && col < seg.Position+seg.Width {
			return seg, true
		}
	}
	return model.Segment{}, false
}

func renderSegment(seg model.Segment, stringIdx int, tab *model.Tab, cur *TabCursor) string {
	str := string(seg.Char)
	if seg.Char >= '0' && seg.Char <= '9' {
		// Note-name view: show the pitch instead of the fret number, using
		// the same column width so the grid stays aligned.
		if cur != nil && cur.ShowNotes && tab != nil && tab.Tuning != nil && seg.Value >= 0 {
			name := tab.Tuning.NoteNameAt(stringIdx, seg.Value)
			if name != "" {
				str = fmt.Sprintf("%-*s", seg.Width, name)
			} else {
				str = fmt.Sprintf("%-*d", seg.Width, seg.Value)
			}
		} else {
			str = fmt.Sprintf("%-*d", seg.Width, seg.Value)
		}
	} else if seg.Width > 1 {
		str = fmt.Sprintf("%-*s", seg.Width, str)
	}
	base := lipgloss.NewStyle().Foreground(StringColor(stringIdx))
	switch {
	case seg.Char >= '0' && seg.Char <= '9':
		return FretDigitStyle.Render(str)
	case seg.Char == '-':
		return base.Render(str)
	case seg.Char == 'h', seg.Char == 'p', seg.Char == 'b', seg.Char == '/', seg.Char == '\\', seg.Char == '~', seg.Char == 'x', seg.Char == 's', seg.Char == 'u':
		return TechniqueStyle.Render(str)
	default:
		return base.Render(str)
	}
}
