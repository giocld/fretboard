package kit

import (
	"fmt"
	"strconv"
	"strings"

	"fretboard/internal/model"
	"github.com/charmbracelet/lipgloss"
)

// TabCursor marks the playhead position in the viewer.
type TabCursor struct {
	Bar     int
	Col     int
	Playing bool
	// LoopStartBar/LoopEndBar mark the A-B loop region as a half-open
	// 0-based bar range (end exclusive). 0/0 means no loop.
	LoopStartBar int
	LoopEndBar   int
}

// InLoop reports whether barIdx falls inside the cursor's A-B loop region.
func (c *TabCursor) InLoop(barIdx int) bool {
	return c != nil && c.LoopEndBar > c.LoopStartBar && barIdx >= c.LoopStartBar && barIdx < c.LoopEndBar
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

// RenderTabPlain renders a tab as plain, uncolored ASCII — the classic
// "|--0--3--|" text form — suitable for exporting to a file or copying to
// the clipboard. Repeat markers and endings are preserved so the exported
// text re-imports cleanly.
func RenderTabPlain(tab *model.Tab) string {
	if tab == nil {
		return ""
	}
	var b strings.Builder
	if tab.Title != "" {
		b.WriteString(tab.Title)
		b.WriteString("\n")
	}
	if tab.Artist != "" {
		b.WriteString(tab.Artist)
		b.WriteString("\n")
	}
	if tab.Tuning != nil && len(tab.Tuning) > 0 {
		b.WriteString("Tuning: ")
		b.WriteString(tab.Tuning.Label())
		b.WriteString("\n\n")
	}
	for _, bar := range tab.Bars {
		width := 0
		for _, sl := range bar.Strings {
			for _, seg := range sl.Segments {
				if w := seg.Position + seg.Width; w > width {
					width = w
				}
			}
		}
		if width == 0 {
			continue
		}
		lines := make([]string, len(bar.Strings))
		for s, sl := range bar.Strings {
			row := []byte(strings.Repeat("-", width))
			for _, seg := range sl.Segments {
				var digits string
				switch {
				case seg.Value > 0:
					digits = strconv.Itoa(seg.Value)
				case seg.Char == '0':
					digits = "0" // open string
				default:
					continue // rest or technique character
				}
				for i := 0; i < len(digits) && seg.Position+i < width; i++ {
					row[seg.Position+i] = digits[i]
				}
			}
			lines[s] = string(row)
		}
		open := "|"
		if bar.RepeatStart {
			open = "|:"
		}
		closeP := "|"
		if bar.RepeatEnd {
			closeP = ":|"
		}
		for s, line := range lines {
			ending := ""
			if s == 0 && (bar.Ending == 1 || bar.Ending == 2) {
				ending = fmt.Sprintf("%d.", bar.Ending)
			}
			b.WriteString(open + ending + line + closeP + "\n")
		}
	}
	return b.String()
}

// RenderTabWithOffset renders a tab starting at the given horizontal column offset.
func RenderTabWithOffset(tab *model.Tab, offset int) string {
	return RenderTabGrid(tab, defaultGridWidth, offset, nil)
}

// RenderTabLinear renders the tab as a vertical strip of bars (TuxGuitar's
// linear layout): each bar block holds the bar number, a playhead ruler for
// the highlighted bar, the string lines, and a blank separator.
func RenderTabLinear(tab *model.Tab, offset int, cur *TabCursor) string {
	return renderTabLinear(tab, offset, cur, true)
}

// RenderTabLinearBody is RenderTabLinear without the tab's header block.
func RenderTabLinearBody(tab *model.Tab, offset int, cur *TabCursor) string {
	return renderTabLinear(tab, offset, cur, false)
}

func renderTabLinear(tab *model.Tab, offset int, cur *TabCursor, withHeader bool) string {
	if tab == nil {
		return ""
	}
	var sb strings.Builder
	if withHeader && tab.Title != "" {
		sb.WriteString(PanelTitleStyle.Render(tab.Title))
		sb.WriteString("\n")
		if tab.Tuning != nil {
			sb.WriteString(PanelTitleStyle.Render(tab.Tuning.Label()))
			sb.WriteString("\n")
		}
	}
	for i, bar := range tab.Bars {
		if i > 0 {
			sb.WriteString("\n")
		}
		highlight := cur != nil && cur.Bar == i
		sb.WriteString(barHeaderWithMarkers(bar))
		sb.WriteString("\n")
		// String rows prefix their content with a 3-wide right-aligned label
		// plus 2 spaces; the ruler must use the same 5-column prefix or the
		// playhead floats left of the notes' own ┊ markers.
		ruler := strings.Repeat(" ", 5)
		column := 0
		if highlight && cur != nil && cur.Col >= offset {
			column = cur.Col - offset
		}
		ruler += strings.Repeat(" ", column) + CursorStyle.Render("┊")
		sb.WriteString(ruler)
		sb.WriteString("\n")
		for si := 0; si < len(bar.Strings); si++ {
			label := ""
			if tab.Tuning != nil {
				label = tab.Tuning.NoteName(si)
			}
			sb.WriteString(StringLabel.Render(label))
			sb.WriteString(strings.Repeat(" ", 2))
			sb.WriteString(renderStringContent(bar.Strings[si], si, offset, curCol(cur, highlight), highlight))
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// barWidthInLinear returns a bar's natural content width in the linear layout.
func barWidthInLinear(bar model.Bar, tab *model.Tab) int {
	return maxBarCols(bar) + 4
}

// barHeader renders the "│ N ───" bar-number line.
func barHeader(number int, width int) string {
	return BarNumberStyle.Render(fmt.Sprintf("│ %d %s", number, strings.Repeat("─", width)))
}

// barHeaderWithMarkers renders the linear-layout bar header including repeat
// structure markers ("|:", ":|", "1."/"2.") when present.
func barHeaderWithMarkers(bar model.Bar) string {
	open := "│"
	if bar.RepeatStart {
		open = "│:"
	}
	close := ""
	if bar.RepeatEnd {
		close = ":│"
	}
	ending := ""
	if bar.Ending == 1 || bar.Ending == 2 {
		ending = fmt.Sprintf("%d.", bar.Ending)
	}
	return BarNumberStyle.Render(fmt.Sprintf("%s %d %s%s%s", open, bar.Number, ending, strings.Repeat("─", barWidthInLinear(bar, nil)), close))
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
	BarWidth   int // width of each bar column (padded; ≥ widest bar's content)
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
	return BarGridMetrics{BarsPerRow: barsPerRow, BarWidth: barWidth, RowHeight: rowHeight}
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

// GridBarLineOffsets returns the content line where each bar's row begins in
// the page layout, matching RenderTabGrid exactly. Rows are priced at their
// own actual height (header + row string count + blank), so mixed string
// counts across rows do not overshoot the playhead line.
func GridBarLineOffsets(tab *model.Tab, availWidth int) []int {
	if tab == nil || len(tab.Bars) == 0 {
		return nil
	}
	offset := 0
	if tab.Title != "" {
		offset += 2
	}
	metrics := BarGridLayout(tab, availWidth)
	out := make([]int, len(tab.Bars))
	for rowStart := 0; rowStart < len(tab.Bars); rowStart += metrics.BarsPerRow {
		rowEnd := rowStart + metrics.BarsPerRow
		if rowEnd > len(tab.Bars) {
			rowEnd = len(tab.Bars)
		}
		for b := rowStart; b < rowEnd; b++ {
			out[b] = offset
		}
		offset += 1 + stringsPerRow(tab, rowStart, rowEnd) + 1
	}
	return out
}

// LinearBarLineOffsets returns the content line where each bar block begins in
// the linear layout, matching RenderTabLinear exactly: title (2 lines) plus,
// per bar, header + ruler + string lines, with one blank separator line
// between blocks.
func LinearBarLineOffsets(tab *model.Tab) []int {
	if tab == nil || len(tab.Bars) == 0 {
		return nil
	}
	offset := 0
	if tab.Title != "" {
		offset += 2
	}
	out := make([]int, len(tab.Bars))
	for i, bar := range tab.Bars {
		out[i] = offset
		offset += 3 + len(bar.Strings) // header + ruler + strings, then one blank line
	}
	return out
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
// The tab's own header (title/artist/tuning) is included.
func RenderTabGrid(tab *model.Tab, width int, offset int, cur *TabCursor) string {
	return renderTabGrid(tab, width, offset, cur, true)
}

// RenderTabGridBody is RenderTabGrid without the tab's header block — used by
// the viewer, which draws its own title/status chrome above the panel.
func RenderTabGridBody(tab *model.Tab, width int, offset int, cur *TabCursor) string {
	return renderTabGrid(tab, width, offset, cur, false)
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

		// Repeat structure markers: "|:" opens a repeat, ":|" closes one,
		// "1."/"2." label endings — mirrored on the bar header so the
		// player sees the same structure the playback follows.
		open := "│"
		if bar.RepeatStart {
			open = "│:"
		}
		close := ""
		if bar.RepeatEnd {
			close = ":│"
		}
		ending := ""
		if bar.Ending == 1 || bar.Ending == 2 {
			ending = fmt.Sprintf("%d.", bar.Ending)
		}
		header := fmt.Sprintf("%s %d %s%s%s ", open, bar.Number, ending, strings.Repeat("─", 12), close)
		headerStyle := MutedStyle
		switch {
		case highlight:
			headerStyle = CursorStyle
		case cur.InLoop(barIdx):
			headerStyle = LoopBarStyle
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
			// Pad the content to the bar column width so string rows line up
			// under the bar headers; the prefix width is measured because the
			// string label style has its own fixed width.
			b.WriteString(prefix + padToWidth(content, barWidth-lipgloss.Width(prefix)))
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
	// The vertical playhead must appear on every string of the highlighted
	// bar, even rows whose content ends before the cursor column — a line
	// with a short rest still crosses the current beat.
	if highlight && cursorCol+1 > maxCol {
		maxCol = cursorCol + 1
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
