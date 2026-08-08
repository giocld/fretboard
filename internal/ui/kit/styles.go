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
	LoopBarStyle   lipgloss.Style
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
	PanelDividerStyle   lipgloss.Style
	FooterKeyStyle      lipgloss.Style
	FooterHintStyle     lipgloss.Style
	FooterStatusStyle   lipgloss.Style
	StatValueStyle      lipgloss.Style
	StatLabelStyle      lipgloss.Style
	TableHeaderStyle    lipgloss.Style
	ActionTitleStyle    lipgloss.Style
	ActionDescStyle     lipgloss.Style
	ActionSelectedStyle lipgloss.Style
	MutedStyle          lipgloss.Style
	SuccessStyle        lipgloss.Style
	WarningStyle        lipgloss.Style
	InfoStyle           lipgloss.Style
	HighlightStyle      lipgloss.Style
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
	BarNumberStyle = lipgloss.NewStyle().Foreground(t.Secondary).Bold(true)
	FretDigitStyle = lipgloss.NewStyle().Foreground(t.Emphasis).Bold(true)
	RestStyle = lipgloss.NewStyle().Foreground(t.Dimmed)
	TechniqueStyle = lipgloss.NewStyle().Foreground(t.Accent).Italic(true)
	CursorStyle = lipgloss.NewStyle().Reverse(true)
	PlayheadStyle = lipgloss.NewStyle().Foreground(t.Accent).Bold(true)
	LoopBarStyle = lipgloss.NewStyle().Foreground(t.Success).Bold(true)
	StatusBarStyle = lipgloss.NewStyle().Background(t.StatusBG).Foreground(t.Primary).Padding(0, 1)
	ListNormal = lipgloss.NewStyle().Padding(0, 1).Foreground(t.Primary)
	ListSelected = ListNormal.Copy().Background(t.Overlay).Foreground(t.Emphasis).Bold(true)
	StringLabel = lipgloss.NewStyle().Foreground(t.Secondary).Width(3).Align(lipgloss.Right)
	ErrorStyle = lipgloss.NewStyle().Foreground(t.Error).Bold(true)
	// The header is plain text: weight and position carry it, no background.
	HeaderStyle = lipgloss.NewStyle().Padding(0, 1)
	LogoStyle = lipgloss.NewStyle().Foreground(t.Accent).Bold(true)
	BreadcrumbStyle = lipgloss.NewStyle().Foreground(t.Dimmed)
	PanelStyle = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(t.Accent).
		Padding(0, 1)
	PanelTitleStyle = lipgloss.NewStyle().Foreground(t.Emphasis).Bold(true)
	PanelDividerStyle = lipgloss.NewStyle().Foreground(t.Dimmed)
	FooterKeyStyle = lipgloss.NewStyle().Foreground(t.Accent).Bold(true)
	FooterHintStyle = lipgloss.NewStyle().Foreground(t.Dimmed)
	FooterStatusStyle = lipgloss.NewStyle().Foreground(t.Secondary)
	StatValueStyle = lipgloss.NewStyle().Foreground(t.Emphasis).Bold(true)
	StatLabelStyle = lipgloss.NewStyle().Foreground(t.Dimmed).Bold(true)
	TableHeaderStyle = lipgloss.NewStyle().Foreground(t.Dimmed).Bold(true)
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
	HighlightStyle = lipgloss.NewStyle().Foreground(t.Highlight)
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
