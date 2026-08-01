package kit

import (
	"fmt"
	"strings"

	"github.com/YOUR_USERNAME/fretboard/internal/model"
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
	return RenderTabWithOffset(tab, 0)
}

// RenderTabPreview renders the first few lines of a tab for dashboard previews.
func RenderTabPreview(tab *model.Tab, maxLines int) string {
	if tab == nil || len(tab.Bars) == 0 {
		return MutedStyle.Render("No preview available.")
	}
	full := RenderTabWithOffset(tab, 0)
	lines := strings.Split(full, "\n")
	if len(lines) <= maxLines {
		return full
	}
	return strings.Join(lines[:maxLines], "\n") + "\n" + MutedStyle.Render("…")
}

// RenderTabWithOffset renders a tab starting at the given horizontal column offset.
func RenderTabWithOffset(tab *model.Tab, offset int) string {
	return RenderTabWithCursor(tab, offset, nil)
}

// RenderTabWithCursor renders a tab and optionally draws a vertical playhead.
func RenderTabWithCursor(tab *model.Tab, offset int, cur *TabCursor) string {
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

	for barIdx, bar := range tab.Bars {
		b.WriteString(BarNumberStyle.Render(fmt.Sprintf("│ %d ", bar.Number)))
		b.WriteString(MutedStyle.Render(strings.Repeat("─", 12)))
		b.WriteString("\n")

		highlight := cur != nil && barIdx == cur.Bar
		if highlight && cur.Col >= offset {
			b.WriteString(renderPlayheadRuler(cur.Col, offset))
		}

		for s, line := range bar.Strings {
			label := tab.Tuning.NoteName(s)
			if label == "" {
				label = "?"
			}
			style := StringLabel.Copy().Foreground(StringColor(s))
			b.WriteString(MutedStyle.Render("│"))
			b.WriteString(style.Render(label[:1]))
			b.WriteString(MutedStyle.Render("│"))
			b.WriteString(renderStringContent(line, s, offset, curCol(cur, highlight), highlight))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	return b.String()
}

func curCol(cur *TabCursor, highlight bool) int {
	if !highlight || cur == nil {
		return -1
	}
	return cur.Col
}

func renderPlayheadRuler(col, offset int) string {
	if col < offset {
		return ""
	}
	pad := col - offset
	return "   " + strings.Repeat(" ", pad) + PlayheadStyle.Render("┊") + "\n"
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
