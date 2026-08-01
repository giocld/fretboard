package kit

import (
	"fmt"
	"strings"

	"fretboard/internal/model"
	"github.com/charmbracelet/lipgloss"
)

// TabCursor marks the playhead position in the viewer.
type TabCursor struct {
	Bar     int
	Col     int
	Playing bool
}

// RenderTab renders a parsed tab as a styled string for the terminal.
// It is exported so the e2e test module can verify rendering output.
func RenderTab(tab *model.Tab) string {
	return RenderTabGrid(tab, defaultGridWidth, 0, nil)
}

// RenderTabPreview renders the first few lines of a tab for dashboard previews.
func RenderTabPreview(tab *model.Tab, maxLines int) string {
	if tab == nil || len(tab.Bars) == 0 {
		return MutedStyle.Render("No preview available.")
	}
	full := RenderTabGrid(tab, defaultGridWidth, 0, nil)
	lines := strings.Split(full, "\n")
	if len(lines) <= maxLines {
		return full
	}
	return strings.Join(lines[:maxLines], "\n") + "\n" + MutedStyle.Render("…")
}

// RenderTabWithOffset renders a tab starting at the given horizontal column offset.
func RenderTabWithOffset(tab *model.Tab, offset int) string {
	return RenderTabGrid(tab, defaultGridWidth, offset, nil)
}

// RenderTabWithCursor renders a tab and optionally draws a vertical playhead.
// It uses a page layout: bars flow left-to-right and wrap into rows that fill
// the available width (like TuxGuitar's page layout or alphaTab's reflow).
func RenderTabWithCursor(tab *model.Tab, offset int, cur *TabCursor) string {
	return RenderTabGrid(tab, defaultGridWidth, offset, cur)
}

// Default grid layout constants.
const (
	defaultGridWidth = 120
	minBarWidth      = 18
	maxBarsPerRow    = 6
)

// BarGridMetrics describes the page layout for a tab at a given width.
type BarGridMetrics struct {
	BarsPerRow int
	RowHeight  int // lines occupied by one row of bars (header + strings + blank)
}

// BarGridLayout computes the page-layout metrics for a tab at availWidth.
// Bars are packed side by side to fill the terminal; if any bar is wider than
// the per-bar column, the column widens to fit it (rows shrink accordingly).
func BarGridLayout(tab *model.Tab, availWidth int) BarGridMetrics {
	if tab == nil || len(tab.Bars) == 0 {
		return BarGridMetrics{BarsPerRow: 1, RowHeight: 3}
	}
	if availWidth < minBarWidth+4 {
		availWidth = minBarWidth + 4
	}
	natural := maxNaturalBarWidth(tab)
	barsPerRow := availWidth / minBarWidth
	if barsPerRow < 1 {
		barsPerRow = 1
	}
	if barsPerRow > maxBarsPerRow {
		barsPerRow = maxBarsPerRow
	}
	barWidth := availWidth / barsPerRow
	if natural > barWidth {
		barWidth = natural
		barsPerRow = availWidth / barWidth
		if barsPerRow < 1 {
			barsPerRow = 1
		}
	}
	rows := (len(tab.Bars) + barsPerRow - 1) / barsPerRow
	rowHeight := 3 // header + blank
	for r := 0; r < rows; r++ {
		for b := r * barsPerRow; b < (r+1)*barsPerRow && b < len(tab.Bars); b++ {
			if h := len(tab.Bars[b].Strings) + 2; h > rowHeight {
				rowHeight = h
			}
		}
	}
	return BarGridMetrics{BarsPerRow: barsPerRow, RowHeight: rowHeight}
}

func maxNaturalBarWidth(tab *model.Tab) int {
	max := 0
	for _, bar := range tab.Bars {
		if c := maxBarCols(bar); c > max {
			max = c
		}
	}
	return max
}

func maxBarCols(bar model.Bar) int {
	max := 0
	for _, str := range bar.Strings {
		for _, seg := range str.Segments {
			if end := seg.Position + seg.Width; end > max {
				max = end
			}
		}
	}
	return max
}

// RenderTabGrid renders a tab in a page layout at the given width, starting at
// the given horizontal column offset, optionally highlighting the playhead bar.
func RenderTabGrid(tab *model.Tab, width int, offset int, cur *TabCursor) string {
	if tab == nil || len(tab.Bars) == 0 {
		return "No tab loaded."
	}

	var b strings.Builder
	if tab.Title != "" {
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
	barWidth := width / metrics.BarsPerRow

	for rowStart := 0; rowStart < len(tab.Bars); rowStart += metrics.BarsPerRow {
		rowEnd := rowStart + metrics.BarsPerRow
		if rowEnd > len(tab.Bars) {
			rowEnd = len(tab.Bars)
		}
		renderBarRow(&b, tab, rowStart, rowEnd, barWidth, offset, cur)
		b.WriteString("\n")
	}
	return b.String()
}

func renderBarRow(b *strings.Builder, tab *model.Tab, start, end, barWidth, offset int, cur *TabCursor) {
	for barIdx := start; barIdx < end; barIdx++ {
		bar := tab.Bars[barIdx]
		highlight := cur != nil && barIdx == cur.Bar

		header := fmt.Sprintf("│ %d ", bar.Number)
		header += strings.Repeat("─", 12)
		headerStyle := MutedStyle
		if highlight {
			headerStyle = CursorStyle
		}
		// pad the header to the bar column width
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
			content := renderStringContent(line, s, offset, curCol(cur, highlight), highlight)
			b.WriteString(prefix + padToWidth(content, barWidth-4))
		}
		b.WriteString("\n")
	}
}

func stringsPerRow(tab *model.Tab, start, end int) int {
	max := 0
	for i := start; i < end && i < len(tab.Bars); i++ {
		if n := len(tab.Bars[i].Strings); n > max {
			max = n
		}
	}
	if max == 0 {
		return 6
	}
	return max
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

func renderStringContent(line model.StringLine, stringIdx, offset, cursorCol int, highlight bool) string {
	maxCol := 0
	for _, seg := range line.Segments {
		if end := seg.Position + seg.Width; end > maxCol {
			maxCol = end
		}
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
		rendered := renderSegment(seg, stringIdx)
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

func renderSegment(seg model.Segment, stringIdx int) string {
	str := string(seg.Char)
	if seg.Char >= '0' && seg.Char <= '9' {
		str = fmt.Sprintf("%-*d", seg.Width, seg.Value)
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

// StatusInfo is the metadata shown in the status bar.
type StatusInfo struct {
	Filename  string
	Tuning    string
	BPM       int
	Playing   bool
	LoopStart int
	LoopEnd   int
}

// RenderStatusBar renders a full-width status bar.
func RenderStatusBar(width int, info StatusInfo) string {
	if info.BPM <= 0 {
		info.BPM = 120
	}
	left := fmt.Sprintf("%s  │  %s  │  BPM: %d", info.Filename, info.Tuning, info.BPM)
	if info.Playing {
		left += "  │  ▶"
	}
	playStr := "Space:play"
	if info.Playing {
		playStr = "Space:pause"
	}
	right := fmt.Sprintf("j/k:scroll  %s  /:search  q:quit", playStr)
	fill := width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if fill < 0 {
		fill = 0
	}
	content := left + strings.Repeat(" ", fill) + right
	return StatusBarStyle.Width(width).Render(content)
}

// Truncate shortens s to fit max display columns, appending an ellipsis.
func Truncate(s string, max int) string {
	if max < 4 || lipgloss.Width(s) <= max {
		return s
	}
	limit := max - 1
	var b strings.Builder
	for _, r := range s {
		w := lipgloss.Width(string(r))
		if w > limit {
			break
		}
		b.WriteRune(r)
		limit -= w
	}
	return b.String() + "…"
}
