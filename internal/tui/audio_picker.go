package tui

import (
	"fmt"
	"strings"

	"github.com/YOUR_USERNAME/fretboard/internal/model"
	"github.com/YOUR_USERNAME/fretboard/internal/player"
	tea "github.com/charmbracelet/bubbletea"
)

// AudioCatalogMsg delivers ranked audio options for the current tab.
type AudioCatalogMsg struct {
	Catalog player.AudioCatalog
	Err     error
	Artist  string
	Title   string
	TabID   int64
	TabPath string
}

func fetchAudioCatalogCmd(tab *model.Tab, tabPath string, tabID int64, audioDirs []string, allowOnline bool) tea.Cmd {
	return func() tea.Msg {
		if tab == nil {
			return AudioCatalogMsg{}
		}
		cat, err := player.BuildAudioCatalog(tab, tabPath, audioDirs, allowOnline)
		return AudioCatalogMsg{
			Catalog: cat,
			Err:     err,
			Artist:  tab.Artist,
			Title:   tab.Title,
			TabID:   tabID,
			TabPath: tabPath,
		}
	}
}

// RenderAudioPicker draws the source selection overlay.
func RenderAudioPicker(width int, catalog player.AudioCatalog, cursor int, fetching bool) string {
	title := "Audio source"
	if fetching {
		title += "  … searching"
	}
	body := renderAudioPickerBody(catalog, cursor, fetching)
	return "\n" + RenderPanel(width-2, title, body)
}

func renderAudioPickerBody(catalog player.AudioCatalog, cursor int, fetching bool) string {
	if fetching && len(catalog.Sources) <= 1 {
		return mutedStyle.Render("Searching for matching recordings…")
	}
	if len(catalog.Sources) == 0 {
		return mutedStyle.Render("No audio sources found. Press Esc to use MIDI.")
	}
	var lines []string
	for i, src := range catalog.Sources {
		prefix := "  "
		if i == cursor {
			prefix = "▸ "
		}
		kind := string(src.Kind)
		line := fmt.Sprintf("%s[%s] %s", prefix, kind, src.Label)
		if src.Detail != "" {
			line += mutedStyle.Render("  · " + src.Detail)
		}
		if src.Score != 0 && src.Kind == player.SourceOnline {
			line += mutedStyle.Render(fmt.Sprintf("  · match %d", src.Score))
		}
		if i == cursor {
			line = listSelected.Render(line)
		} else {
			line = listNormal.Render(line)
		}
		lines = append(lines, line)
	}
	lines = append(lines, "")
	lines = append(lines, mutedStyle.Render("j/k move  Enter select  r refresh  Esc cancel"))
	return strings.Join(lines, "\n")
}
