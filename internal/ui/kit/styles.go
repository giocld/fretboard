package kit

import "github.com/charmbracelet/lipgloss"

var (
	currentTheme = DefaultTheme

	BarNumberStyle lipgloss.Style
	FretDigitStyle lipgloss.Style
	RestStyle      lipgloss.Style
	TechniqueStyle lipgloss.Style
	CursorStyle    lipgloss.Style
	PlayheadStyle  lipgloss.Style
	StatusBarStyle lipgloss.Style
	ListNormal     lipgloss.Style
	ListSelected   lipgloss.Style
	StringLabel    lipgloss.Style
	ErrorStyle     lipgloss.Style

	// Chrome / layout styles
	HeaderStyle         lipgloss.Style
	LogoStyle           lipgloss.Style
	BreadcrumbStyle     lipgloss.Style
	PanelStyle          lipgloss.Style
	PanelTitleStyle     lipgloss.Style
	FooterKeyStyle      lipgloss.Style
	FooterHintStyle     lipgloss.Style
	StatValueStyle      lipgloss.Style
	StatLabelStyle      lipgloss.Style
	ActionTitleStyle    lipgloss.Style
	ActionDescStyle     lipgloss.Style
	ActionSelectedStyle lipgloss.Style
	MutedStyle          lipgloss.Style
	SuccessStyle        lipgloss.Style
	WarningStyle        lipgloss.Style
	InfoStyle           lipgloss.Style
	TooSmallStyle       lipgloss.Style
)

func init() {
	applyTheme(DefaultTheme)
}

// SetTheme changes the active theme by name.
func SetTheme(name string) {
	applyTheme(getTheme(name))
}

// applyTheme rebuilds all package-level styles from the given theme.
func applyTheme(t Theme) {
	currentTheme = t
	BarNumberStyle = lipgloss.NewStyle().Foreground(t.Secondary).Bold(true).Width(5)
	FretDigitStyle = lipgloss.NewStyle().Foreground(t.Emphasis).Bold(true)
	RestStyle = lipgloss.NewStyle().Foreground(t.Dimmed)
	TechniqueStyle = lipgloss.NewStyle().Foreground(t.Accent).Italic(true)
	CursorStyle = lipgloss.NewStyle().Reverse(true)
	PlayheadStyle = lipgloss.NewStyle().Foreground(t.Accent).Bold(true)
	StatusBarStyle = lipgloss.NewStyle().Background(t.StatusBG).Foreground(t.Primary).Padding(0, 1)
	ListNormal = lipgloss.NewStyle().Padding(0, 1).Foreground(t.Primary)
	ListSelected = ListNormal.Copy().Background(t.Overlay).Foreground(t.Emphasis).Bold(true)
	StringLabel = lipgloss.NewStyle().Foreground(t.Secondary).Width(3).Align(lipgloss.Right)
	ErrorStyle = lipgloss.NewStyle().Foreground(t.Error).Bold(true)
	HeaderStyle = lipgloss.NewStyle().Foreground(t.Primary).Bold(true).Padding(0, 1)
	LogoStyle = lipgloss.NewStyle().Foreground(t.Accent).Bold(true)
	BreadcrumbStyle = lipgloss.NewStyle().Foreground(t.Dimmed)
	PanelStyle = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(t.Accent).
		Padding(0, 1)
	PanelTitleStyle = lipgloss.NewStyle().Foreground(t.Emphasis).Bold(true)
	FooterKeyStyle = lipgloss.NewStyle().Foreground(t.Accent).Bold(true)
	FooterHintStyle = lipgloss.NewStyle().Foreground(t.Dimmed)
	StatValueStyle = lipgloss.NewStyle().Foreground(t.Emphasis).Bold(true)
	StatLabelStyle = lipgloss.NewStyle().Foreground(t.Dimmed)
	ActionTitleStyle = lipgloss.NewStyle().Foreground(t.Emphasis).Bold(true)
	ActionDescStyle = lipgloss.NewStyle().Foreground(t.Dimmed)
	ActionSelectedStyle = lipgloss.NewStyle().
		Foreground(t.Emphasis).
		Background(t.Overlay).
		Bold(true).
		Padding(0, 1)
	MutedStyle = lipgloss.NewStyle().Foreground(t.Dimmed)
	SuccessStyle = lipgloss.NewStyle().Foreground(t.Success)
	WarningStyle = lipgloss.NewStyle().Foreground(t.Warning)
	InfoStyle = lipgloss.NewStyle().Foreground(t.Info)
	TooSmallStyle = lipgloss.NewStyle().
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
