package tui

import "github.com/charmbracelet/lipgloss"

var (
	currentTheme = DefaultTheme

	barNumberStyle lipgloss.Style
	fretDigitStyle lipgloss.Style
	restStyle      lipgloss.Style
	techniqueStyle lipgloss.Style
	cursorStyle    lipgloss.Style
	playheadStyle  lipgloss.Style
	statusBarStyle lipgloss.Style
	listNormal     lipgloss.Style
	listSelected   lipgloss.Style
	stringLabel    lipgloss.Style
	errorStyle     lipgloss.Style

	// Chrome / layout styles
	headerStyle         lipgloss.Style
	logoStyle           lipgloss.Style
	breadcrumbStyle     lipgloss.Style
	panelStyle          lipgloss.Style
	panelTitleStyle     lipgloss.Style
	footerKeyStyle      lipgloss.Style
	footerHintStyle     lipgloss.Style
	statValueStyle      lipgloss.Style
	statLabelStyle      lipgloss.Style
	actionTitleStyle    lipgloss.Style
	actionDescStyle     lipgloss.Style
	actionSelectedStyle lipgloss.Style
	mutedStyle          lipgloss.Style
	successStyle        lipgloss.Style
	warningStyle        lipgloss.Style
	infoStyle           lipgloss.Style
	tooSmallStyle       lipgloss.Style
)

func init() {
	ApplyTheme(DefaultTheme)
}

// SetTheme changes the active theme by name.
func SetTheme(name string) {
	ApplyTheme(GetTheme(name))
}

// ApplyTheme rebuilds all package-level styles from the given theme.
func ApplyTheme(t Theme) {
	currentTheme = t
	barNumberStyle = lipgloss.NewStyle().Foreground(t.Secondary).Bold(true).Width(5)
	fretDigitStyle = lipgloss.NewStyle().Foreground(t.Emphasis).Bold(true)
	restStyle = lipgloss.NewStyle().Foreground(t.Dimmed)
	techniqueStyle = lipgloss.NewStyle().Foreground(t.Accent).Italic(true)
	cursorStyle = lipgloss.NewStyle().Reverse(true)
	playheadStyle = lipgloss.NewStyle().Foreground(t.Accent).Bold(true)
	statusBarStyle = lipgloss.NewStyle().Background(t.StatusBG).Foreground(t.Primary).Padding(0, 1)
	listNormal = lipgloss.NewStyle().Padding(0, 1).Foreground(t.Primary)
	listSelected = listNormal.Copy().Background(t.Overlay).Foreground(t.Emphasis).Bold(true)
	stringLabel = lipgloss.NewStyle().Foreground(t.Secondary).Width(3).Align(lipgloss.Right)
	errorStyle = lipgloss.NewStyle().Foreground(t.Error).Bold(true)
	headerStyle = lipgloss.NewStyle().Foreground(t.Primary).Bold(true).Padding(0, 1)
	logoStyle = lipgloss.NewStyle().Foreground(t.Accent).Bold(true)
	breadcrumbStyle = lipgloss.NewStyle().Foreground(t.Dimmed)
	panelStyle = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(t.Accent).
		Padding(0, 1)
	panelTitleStyle = lipgloss.NewStyle().Foreground(t.Emphasis).Bold(true)
	footerKeyStyle = lipgloss.NewStyle().Foreground(t.Accent).Bold(true)
	footerHintStyle = lipgloss.NewStyle().Foreground(t.Dimmed)
	statValueStyle = lipgloss.NewStyle().Foreground(t.Emphasis).Bold(true)
	statLabelStyle = lipgloss.NewStyle().Foreground(t.Dimmed)
	actionTitleStyle = lipgloss.NewStyle().Foreground(t.Emphasis).Bold(true)
	actionDescStyle = lipgloss.NewStyle().Foreground(t.Dimmed)
	actionSelectedStyle = lipgloss.NewStyle().
		Foreground(t.Emphasis).
		Background(t.Overlay).
		Bold(true).
		Padding(0, 1)
	mutedStyle = lipgloss.NewStyle().Foreground(t.Dimmed)
	successStyle = lipgloss.NewStyle().Foreground(t.Success)
	warningStyle = lipgloss.NewStyle().Foreground(t.Warning)
	infoStyle = lipgloss.NewStyle().Foreground(t.Info)
	tooSmallStyle = lipgloss.NewStyle().
		Foreground(t.Warning).
		Background(t.Surface).
		Padding(1, 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Warning)
}

// CurrentTheme returns the active theme.
func CurrentTheme() Theme {
	return currentTheme
}

// StringColor returns the color for a given string index.
func StringColor(idx int) lipgloss.AdaptiveColor {
	if idx < 0 || idx >= len(currentTheme.StringColors) {
		return currentTheme.Primary
	}
	return currentTheme.StringColors[idx]
}
