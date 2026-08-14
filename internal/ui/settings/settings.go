// Package settings implements the settings screen: a small navigable list of
// preferences (volume, strict audio selection, theme) that are applied live
// and persisted to the config file by the router.
package settings

import (
	"fmt"
	"strings"

	"fretboard/internal/config"
	"fretboard/internal/ui/kit"
	"fretboard/internal/ui/msgs"
	tea "github.com/charmbracelet/bubbletea"
)

type SettingsModel struct {
	cursor   int
	width    int
	height   int
	cfg      config.Config
	themes   []string
	themeIdx int
}

// NewSettingsModel loads the current config and theme list.
func NewSettingsModel() SettingsModel {
	cfg, _ := config.Load()
	themes := kit.ThemeNames()
	idx := 0
	for i, n := range themes {
		if n == kit.CurrentTheme().Name {
			idx = i
			break
		}
	}
	return SettingsModel{cfg: cfg, themes: themes, themeIdx: idx, width: 80, height: 24}
}

// Init is part of the tea.Model interface.
func (m SettingsModel) Init() tea.Cmd { return nil }

func (m SettingsModel) rowCount() int { return 3 }

func (m SettingsModel) Update(msg tea.Msg) (SettingsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "h", "b":
			return m, func() tea.Msg { return msgs.SettingsBackMsg{} }
		case "j", "down":
			if m.cursor < m.rowCount()-1 {
				m.cursor++
			}
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "enter", "left", "right", "-", "_", "+", "=", "t":
			m.adjust(m.cursor, msg.String())
		}
	}
	return m, nil
}

func (m *SettingsModel) adjust(row int, key string) {
	switch row {
	case 0: // volume
		switch key {
		case "left", "-", "_":
			m.cfg.VolumePercent -= 10
		case "right", "+", "=":
			m.cfg.VolumePercent += 10
		case "enter":
			// Enter toggles mute-ish floor: 0 <-> 80
			if m.cfg.VolumePercent == 0 {
				m.cfg.VolumePercent = 80
			} else {
				m.cfg.VolumePercent = 0
			}
		}
		m.cfg.VolumePercent = min(max(m.cfg.VolumePercent, 0), 100)
	case 1: // strict audio selection
		if key == "enter" || key == "left" || key == "right" {
			m.cfg.StrictAudioSelection = !m.cfg.StrictAudioSelection
		}
	case 2: // theme
		if len(m.themes) > 0 {
			if key == "enter" || key == "right" || key == "t" {
				m.themeIdx = (m.themeIdx + 1) % len(m.themes)
			} else if key == "left" {
				m.themeIdx = (m.themeIdx - 1 + len(m.themes)) % len(m.themes)
			}
			m.cfg.ThemeName = m.themes[m.themeIdx]
		}
	}
}

// Config returns the adjusted configuration for the router to apply.
func (m SettingsModel) Config() config.Config { return m.cfg }

func (m SettingsModel) View() string {
	rows := []string{
		fmt.Sprintf("Volume        %3d%%   (left/right adjust, enter mutes)", m.cfg.VolumePercent),
		"Strict audio  " + onOff(m.cfg.StrictAudioSelection) + "   (enter toggles studio-lock auto-pick)",
		"Theme         " + m.themeName(),
	}
	var b strings.Builder
	for i, row := range rows {
		if i == m.cursor {
			b.WriteString(kit.ListSelected.Render("▸ " + row))
		} else {
			b.WriteString(kit.ListNormal.Render("  " + row))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(kit.MutedStyle.Render("j/k move · left/right or enter change · Esc back"))
	body := "\n" + kit.RenderPanel(m.width-2, "Settings", strings.TrimSuffix(b.String(), "\n"))
	footer := kit.RenderFooter(m.width, []kit.KeyHint{
		{Key: "j/k", Label: "move"},
		{Key: "←/->", Label: "change"},
		{Key: "Enter", Label: "toggle"},
		{Key: "Esc", Label: "back"},
	})
	return kit.LayoutScreen(m.width, m.height, kit.FormatBreadcrumb("home", "settings"), body, footer)
}

func onOff(v bool) string {
	if v {
		return kit.SuccessStyle.Render("on")
	}
	return kit.MutedStyle.Render("off")
}

func (m SettingsModel) themeName() string {
	if m.themeIdx >= 0 && m.themeIdx < len(m.themes) {
		return kit.InfoStyle.Render(m.themes[m.themeIdx])
	}
	return "default"
}
