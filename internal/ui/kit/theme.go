// Package kit provides the shared presentational layer for the fretboard TUI:
// themes, styles, layout chrome, tab rendering, and key definitions.
package kit

import "github.com/charmbracelet/lipgloss"

// Theme is a complete semantic color palette for the TUI.
type Theme struct {
	Name         string
	Primary      lipgloss.AdaptiveColor // fg.default
	Secondary    lipgloss.AdaptiveColor // secondary labels
	Dimmed       lipgloss.AdaptiveColor // fg.muted
	Accent       lipgloss.AdaptiveColor // accent.primary
	Highlight    lipgloss.AdaptiveColor // accent.secondary
	Emphasis     lipgloss.AdaptiveColor // fg.emphasis
	Error        lipgloss.Color         // status.error
	Success      lipgloss.Color         // status.success
	Warning      lipgloss.Color         // status.warning
	Info         lipgloss.Color         // status.info
	Base         lipgloss.Color         // bg.base
	Surface      lipgloss.Color         // bg.surface
	Overlay      lipgloss.Color         // bg.overlay
	StatusBG     lipgloss.Color         // footer background
	// Staff colors the tab's string lines: the muted color blended toward
	// the accent, so it reads as the theme's own tint -- brighter than
	// plain grey, without competing with the fret numbers for attention.
	Staff lipgloss.AdaptiveColor
}

var (
	DefaultTheme = Theme{
		Name:      "default",
		Primary:   lipgloss.AdaptiveColor{Light: "#1a1b26", Dark: "#c0caf5"},
		Secondary: lipgloss.AdaptiveColor{Light: "#565f89", Dark: "#7aa2f7"},
		Dimmed:    lipgloss.AdaptiveColor{Light: "#9aa5ce", Dark: "#565f89"},
		Accent:    lipgloss.AdaptiveColor{Light: "#2e3c64", Dark: "#7aa2f7"},
		Highlight: lipgloss.AdaptiveColor{Light: "#3d59a1", Dark: "#bb9af7"},
		Emphasis:  lipgloss.AdaptiveColor{Light: "#1a1b26", Dark: "#e0e0e0"},
		Error:     lipgloss.Color("#f7768e"),
		Success:   lipgloss.Color("#9ece6a"),
		Warning:   lipgloss.Color("#e0af68"),
		Info:      lipgloss.Color("#7dcfff"),
		Base:      lipgloss.Color("#1a1b26"),
		Surface:   lipgloss.Color("#24283b"),
		Overlay:   lipgloss.Color("#414868"),
		StatusBG:  lipgloss.Color("#16161e"),
		Staff: lipgloss.AdaptiveColor{Light: "#5f6b94", Dark: "#6a84c6"},
	}

	OneDarkTheme = Theme{
		Name:      "onedark",
		Primary:   lipgloss.AdaptiveColor{Light: "#282C34", Dark: "#ABB2BF"},
		Secondary: lipgloss.AdaptiveColor{Light: "#5C6370", Dark: "#61AFEF"},
		Dimmed:    lipgloss.AdaptiveColor{Light: "#ABB2BF", Dark: "#5C6370"},
		Accent:    lipgloss.AdaptiveColor{Light: "#C678DD", Dark: "#61AFEF"},
		Highlight: lipgloss.AdaptiveColor{Light: "#E06C75", Dark: "#C678DD"},
		Emphasis:  lipgloss.AdaptiveColor{Light: "#282C34", Dark: "#ECEFF4"},
		Error:     lipgloss.Color("#E06C75"),
		Success:   lipgloss.Color("#98C379"),
		Warning:   lipgloss.Color("#E5C07B"),
		Info:      lipgloss.Color("#56B6C2"),
		Base:      lipgloss.Color("#282C34"),
		Surface:   lipgloss.Color("#2C323C"),
		Overlay:   lipgloss.Color("#3B4048"),
		StatusBG:  lipgloss.Color("#21252B"),
		Staff: lipgloss.AdaptiveColor{Light: "#ba92d0", Dark: "#5f8db6"},
	}

	DraculaTheme = Theme{
		Name:      "dracula",
		Primary:   lipgloss.AdaptiveColor{Light: "#282A36", Dark: "#F8F8F2"},
		Secondary: lipgloss.AdaptiveColor{Light: "#6272A4", Dark: "#BD93F9"},
		Dimmed:    lipgloss.AdaptiveColor{Light: "#F8F8F2", Dark: "#6272A4"},
		Accent:    lipgloss.AdaptiveColor{Light: "#FF79C6", Dark: "#BD93F9"},
		Highlight: lipgloss.AdaptiveColor{Light: "#F1FA8C", Dark: "#FF79C6"},
		Emphasis:  lipgloss.AdaptiveColor{Light: "#282A36", Dark: "#F8F8F2"},
		Error:     lipgloss.Color("#FF5555"),
		Success:   lipgloss.Color("#50FA7B"),
		Warning:   lipgloss.Color("#F1FA8C"),
		Info:      lipgloss.Color("#8BE9FD"),
		Base:      lipgloss.Color("#282A36"),
		Surface:   lipgloss.Color("#313442"),
		Overlay:   lipgloss.Color("#44475A"),
		StatusBG:  lipgloss.Color("#21222C"),
		Staff: lipgloss.AdaptiveColor{Light: "#b876b7", Dark: "#9484d3"},
	}
)

// Themes is the registry of available themes by name.
var Themes = map[string]Theme{
	DefaultTheme.Name: DefaultTheme,
	OneDarkTheme.Name: OneDarkTheme,
	DraculaTheme.Name: DraculaTheme,
}

// themeOrder preserves declaration order for deterministic theme cycling.
var themeOrder = []string{DefaultTheme.Name, OneDarkTheme.Name, DraculaTheme.Name}

// getTheme returns a theme by name, falling back to the default.
func getTheme(name string) Theme {
	if t, ok := Themes[name]; ok {
		return t
	}
	return DefaultTheme
}

// ThemeNames returns all available theme names in a stable order.
func ThemeNames() []string {
	return append([]string(nil), themeOrder...)
}
