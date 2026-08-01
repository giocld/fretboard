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
			"Resize to at least " + itoa(minTermWidth) + "×" + itoa(minTermHeight) +
			" (currently " + itoa(width) + "×" + itoa(height) + ")",
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

// RenderAppHeader renders the top chrome bar with logo and breadcrumb.
func RenderAppHeader(width int, breadcrumb string) string {
	logo := LogoStyle.Render("♪ fretboard")
	theme := MutedStyle.Render(CurrentTheme().Name)
	crumb := BreadcrumbStyle.Render(breadcrumb)

	gap := width - lipgloss.Width(logo) - lipgloss.Width(theme) - lipgloss.Width(crumb) - 4
	if gap < 1 {
		gap = 1
	}
	line := logo + strings.Repeat(" ", gap) + crumb + "  " + theme
	return HeaderStyle.Render(line)
}

// RenderFooter renders a contextual shortcut bar.
func RenderFooter(width int, hints []KeyHint) string {
	if len(hints) == 0 {
		return ""
	}
	var parts []string
	for _, h := range hints {
		parts = append(parts, FooterKeyStyle.Render("["+h.Key+"]")+FooterHintStyle.Render(h.Label))
	}
	content := strings.Join(parts, "  ")
	return StatusBarStyle.Render(content)
}

// RenderPanel wraps content in a titled bordered panel.
func RenderPanel(width int, title, content string) string {
	innerW := width - 4
	if innerW < 10 {
		innerW = 10
	}
	titleLine := PanelTitleStyle.Render(title)
	body := lipgloss.NewStyle().MaxWidth(innerW).Render(content)
	return PanelStyle.Width(width).Render(titleLine + "\n" + body)
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

// RenderStatBox renders a compact dashboard stat cell.
func RenderStatBox(width int, label, value string) string {
	inner := StatLabelStyle.Render(label) + "\n" + StatValueStyle.Render(value)
	return PanelStyle.Width(width).Render(inner)
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
