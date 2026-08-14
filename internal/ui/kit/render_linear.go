package kit

import (
	"fmt"
	"strings"

	"fretboard/internal/model"
	"github.com/charmbracelet/lipgloss"
)

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
		header := barHeaderWithMarkers(tab, i)
		if cur.InSearch(i) {
			header = SearchBarStyle.Render(header)
		}
		sb.WriteString(header)
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
			sb.WriteString(renderStringContent(bar.Strings[si], si, offset, curCol(cur, highlight), highlight, tab, cur))
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// barWidthInLinear returns a bar's natural content width in the linear layout.
func barWidthInLinear(bar model.Bar, tab *model.Tab) int {
	return maxBarCols(bar) + 4
}

// sectionLabel returns the section name for a bar when it starts a new
// section (the previous bar belongs to a different or no section).
func sectionLabel(tab *model.Tab, barIdx int) string {
	if tab == nil || barIdx < 0 || barIdx >= len(tab.Bars) {
		return ""
	}
	s := strings.TrimSpace(tab.Bars[barIdx].Section)
	if s == "" {
		return ""
	}
	if barIdx > 0 && tab.Bars[barIdx-1].Section == s {
		return ""
	}
	return s
}

// sectionHeaderFor renders a bar header that fits the section name (when
// the bar starts a section) inside the bar column width, with the dash fill
// after it and the repeat-close marker at the end.
func sectionHeaderFor(tab *model.Tab, barIdx int, open, ending, close string, barWidth int) string {
	name := sectionLabel(tab, barIdx)
	num := barIdx + 1 // display bar number (1-based, mirrors Bar.Number)
	if barIdx >= 0 && barIdx < len(tab.Bars) {
		num = tab.Bars[barIdx].Number
	}
	head := fmt.Sprintf("%s %d %s", open, num, ending)
	if name != "" {
		head += " "
		room := max(barWidth-lipgloss.Width(head)-lipgloss.Width(close)-1, 2)
		head += Truncate(name, room)
	}
	fill := max(barWidth-lipgloss.Width(head)-lipgloss.Width(close), 1)
	return head + strings.Repeat("─", fill) + close
}

// barMarkers computes a bar's repeat-open, repeat-close, and ending header
// markers from its structure.
func barMarkers(bar model.Bar) (open, close, ending string) {
	open = "│"
	if bar.RepeatStart {
		open = "│:"
	}
	if bar.RepeatEnd {
		close = ":│"
	}
	if bar.Ending == 1 || bar.Ending == 2 {
		ending = fmt.Sprintf("%d.", bar.Ending)
	}
	return open, close, ending
}

// barHeaderWithMarkers renders the linear-layout bar header including repeat
// structure markers ("|:", ":|", "1."/"2.") and the section name when the
// bar starts a new section.
func barHeaderWithMarkers(tab *model.Tab, barIdx int) string {
	bar := tab.Bars[barIdx]
	open, close, ending := barMarkers(bar)
	width := barWidthInLinear(bar, tab)
	return BarNumberStyle.Render(sectionHeaderFor(tab, barIdx, open, ending, close, width))
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
		left += "  │  "
	}
	playStr := "Space:play"
	if info.Playing {
		playStr = "Space:pause"
	}
	right := fmt.Sprintf("j/k:scroll  %s  /:search  q:quit", playStr)
	fill := max(width-lipgloss.Width(left)-lipgloss.Width(right)-2, 0)
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
	return b.String() + "..."
}
