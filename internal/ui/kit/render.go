package kit

import (
	"fmt"
	"strconv"
	"strings"

	"fretboard/internal/model"
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
	// ShowNotes renders fret numbers as note names instead of digits.
	ShowNotes bool
	// SearchBar/SearchCol mark the current in-tab search match
	// (0-based; -1/-1 means no active match).
	SearchBar int
	SearchCol int
}

// InLoop reports whether barIdx falls inside the cursor's A-B loop region.
func (c *TabCursor) InLoop(barIdx int) bool {
	return c != nil && c.LoopEndBar > c.LoopStartBar && barIdx >= c.LoopStartBar && barIdx < c.LoopEndBar
}

// InSearch reports whether barIdx is the current search match.
func (c *TabCursor) InSearch(barIdx int) bool {
	return c != nil && c.SearchBar == barIdx
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
	return strings.Join(lines[:maxLines], "\n") + "\n" + MutedStyle.Render("...")
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
		width := maxBarCols(bar)
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
		availWidth = max(availWidth, minBarWidth+4)
	}
	natural := maxNaturalBarWidth(tab)
	barsPerRow := availWidth / minBarWidth
	barsPerRow = min(max(barsPerRow, 1), maxBarsPerRow)
	barWidth := availWidth / barsPerRow
	if natural > barWidth {
		barWidth = natural
		barsPerRow = availWidth / barWidth
		barsPerRow = max(barsPerRow, 1)
	}
	rows := (len(tab.Bars) + barsPerRow - 1) / barsPerRow
	rowHeight := 3 // header + blank
	for r := 0; r < rows; r++ {
		for b := r * barsPerRow; b < (r+1)*barsPerRow && b < len(tab.Bars); b++ {
			rowHeight = max(rowHeight, len(tab.Bars[b].Strings)+2)
		}
	}
	return BarGridMetrics{BarsPerRow: barsPerRow, BarWidth: barWidth, RowHeight: rowHeight}
}

func maxNaturalBarWidth(tab *model.Tab) int {
	m := 0
	for _, bar := range tab.Bars {
		m = max(m, maxBarCols(bar))
	}
	return m
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
