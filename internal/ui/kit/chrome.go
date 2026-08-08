package kit

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	minTermWidth  = 60
	minTermHeight = 20
)

// KeyHint is a single footer shortcut.
type KeyHint struct {
	Key   string
	Label string
}

// TermTooSmall reports whether the terminal is below the usable minimum.
func TermTooSmall(width, height int) bool {
	return width < minTermWidth || height < minTermHeight
}

// RenderTooSmall shows a resize prompt when the terminal is too small.
func RenderTooSmall(width, height int) string {
	msg := TooSmallStyle.Render(
		"Terminal too small\n\n" +
			"Resize to at least " + itoa(minTermWidth) + "x" + itoa(minTermHeight) +
			" (currently " + itoa(width) + "x" + itoa(height) + ")",
	)
	return "\n" + lipgloss.Place(width, 8, lipgloss.Center, lipgloss.Center, msg)
}

// FormatBreadcrumb joins navigation segments with a visual separator.
func FormatBreadcrumb(parts ...string) string {
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " › ")
}

// RenderAppHeader renders the top chrome line: logo + breadcrumb, plain text
// (no background bar) so the content remains the loudest element.
func RenderAppHeader(width int, breadcrumb string) string {
	logo := LogoStyle.Render("fretboard")
	crumb := BreadcrumbStyle.Render(breadcrumb)
	gap := width - lipgloss.Width(logo) - lipgloss.Width(crumb) - 4
	if gap < 1 {
		gap = 1
	}
	line := logo + strings.Repeat(" ", gap) + crumb
	return HeaderStyle.Render(line)
}

// RenderDivider renders a full-width dim rule used to separate sections.
func RenderDivider(width int) string {
	if width < 1 {
		return ""
	}
	return PanelDividerStyle.Render(strings.Repeat("─", width))
}

// RenderFooter renders a contextual shortcut bar, keeping it within the
// terminal width. When the hint list is too long it keeps the first and last
// hints (e.g. "q quit") and drops from the middle, so the bar never wraps
// across several lines (which pushed the panel body out of view).
func RenderFooter(width int, hints []KeyHint) string {
	return RenderFooterWithStatus(width, "", hints)
}

// RenderFooterWithStatus renders the footer with an optional left-aligned
// status segment (counts, mode, state) followed by the key hints.
func RenderFooterWithStatus(width int, status string, hints []KeyHint) string {
	status = strings.TrimSpace(status)
	if len(hints) == 0 && status == "" {
		return ""
	}
	render := func(hs []KeyHint) string {
		var parts []string
		for _, h := range hs {
			parts = append(parts, FooterKeyStyle.Render("["+h.Key+"]")+FooterHintStyle.Render(h.Label))
		}
		return strings.Join(parts, "  ")
	}
	content := render(hints)
	if status != "" {
		content = FooterStatusStyle.Render(status) + PanelDividerStyle.Render(" │ ") + content
	}
	fit := width - 2 // StatusBarStyle adds 1 cell of padding on each side
	if lipgloss.Width(content) <= fit || len(hints) <= 1 {
		return StatusBarStyle.Render(content)
	}
	first, last := hints[0], hints[len(hints)-1]
	mid := append([]KeyHint(nil), hints[1:len(hints)-1]...)
	for {
		trimmed := append([]KeyHint{first}, mid...)
		trimmed = append(trimmed, last)
		content = render(trimmed)
		if status != "" {
			content = FooterStatusStyle.Render(status) + PanelDividerStyle.Render(" │ ") + content
		}
		if lipgloss.Width(content) <= fit || len(mid) == 0 {
			return StatusBarStyle.Render(content)
		}
		mid = mid[:len(mid)-1]
	}
}

// RenderPanel wraps content in a bordered panel. With a non-empty title the
// header gets a dim rule underneath; an empty title renders just the border
// and content (used when the caller supplies its own header/table structure).
func RenderPanel(width int, title, content string) string {
	innerW := width - 4
	if innerW < 10 {
		innerW = 10
	}
	body := lipgloss.NewStyle().MaxWidth(innerW).Render(content)
	if title == "" {
		return PanelStyle.Width(width).Render(body)
	}
	titleLine := PanelTitleStyle.Render(title)
	divider := "\n" + PanelDividerStyle.Render(strings.Repeat("─", innerW))
	return PanelStyle.Width(width).Render(titleLine + divider + "\n" + body)
}

// LayoutScreen stacks header, body, and footer into a full-screen view.
func LayoutScreen(width, height int, breadcrumb, body, footer string) string {
	_ = width
	if TermTooSmall(width, height) {
		return RenderTooSmall(width, height)
	}
	header := RenderAppHeader(width, breadcrumb)
	footerBar := footer
	if footerBar == "" {
		footerBar = RenderFooter(width, nil)
	}
	// Do not set Height/Width+Background on the body: lipgloss pads every row
	// with the background color, which turns tall terminals into solid stripes.
	bodyPane := lipgloss.NewStyle().Padding(0, 1).Render(body)
	return lipgloss.JoinVertical(lipgloss.Left, header, bodyPane, footerBar)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
