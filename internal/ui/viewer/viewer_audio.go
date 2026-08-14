package viewer

import (
	"fmt"
	"strings"

	"fretboard/internal/model"
	"fretboard/internal/player"
	"fretboard/internal/ui/kit"
	"fretboard/internal/ui/msgs"
	tea "github.com/charmbracelet/bubbletea"
)

func fetchAudioCatalogCmd(tab *model.Tab, tabPath string, tabID int64, audioDirs []string, allowOnline bool) tea.Cmd {
	return func() tea.Msg {
		if tab == nil {
			return msgs.AudioCatalogMsg{}
		}
		cat, err := player.BuildAudioCatalog(tab, tabPath, audioDirs, allowOnline)
		return msgs.AudioCatalogMsg{
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
func RenderAudioPicker(width int, catalog player.AudioCatalog, cursor int, fetching bool, strict bool, recommended int, rejected map[string]bool) string {
	title := "Audio source"
	if fetching {
		title += "  ... searching"
	}
	body := renderAudioPickerBody(catalog, cursor, fetching, strict, recommended, rejected)
	return "\n" + kit.RenderPanel(width-2, title, body)
}

func categoryBadge(c player.AudioCategory) string {
	switch c {
	case player.CatOfficial:
		return kit.SuccessStyle.Render("[official]")
	case player.CatBacking:
		return kit.InfoStyle.Render("[backing]")
	case player.CatLive:
		return kit.WarningStyle.Render("[live]")
	case player.CatCover:
		return kit.WarningStyle.Render("[cover]")
	case player.CatLesson:
		return kit.MutedStyle.Render("[lesson]")
	case player.CatLocal:
		return kit.MutedStyle.Render("[local]")
	default:
		return kit.MutedStyle.Render("[?]")
	}
}

func renderAudioPickerBody(catalog player.AudioCatalog, cursor int, fetching bool, strict bool, recommended int, rejected map[string]bool) string {
	if fetching && len(catalog.Sources) <= 1 {
		return kit.MutedStyle.Render("Searching for matching recordings...")
	}
	if len(catalog.Sources) == 0 {
		return kit.MutedStyle.Render("No audio sources found. Press Esc to use MIDI.")
	}
	var lines []string
	for i, src := range catalog.Sources {
		prefix := "  "
		if i == cursor {
			prefix = "▸ "
		}
		kind := string(src.Kind)
		notStudio := strict && src.Kind == player.SourceOnline && !src.StrictOK
		userRejected := rejected[src.ID]
		dim := notStudio || userRejected
		line := fmt.Sprintf("%s%s %s%s", prefix, kind, categoryBadge(src.Category), src.Label)
		if i == recommended && !dim {
			line += kit.SuccessStyle.Render("  *")
		}
		if userRejected {
			line += kit.MutedStyle.Render("  rejected")
		} else if notStudio {
			line += kit.MutedStyle.Render("  not studio")
		}
		if src.Detail != "" {
			line += kit.MutedStyle.Render("  · " + src.Detail)
		}
		if src.Score != 0 && src.Kind == player.SourceOnline {
			line += kit.MutedStyle.Render(fmt.Sprintf("  · match %d", src.Score))
		}
		if i == cursor {
			line = kit.ListSelected.Render(line)
		} else if dim {
			line = kit.MutedStyle.Render(line)
		} else {
			line = kit.ListNormal.Render(line)
		}
		lines = append(lines, line)
	}
	lines = append(lines, "")
	lines = append(lines, kit.MutedStyle.Render("j/k move  Enter select  r refresh  Esc cancel"))
	if strict {
		lines = append(lines, kit.MutedStyle.Render("strict on: live/cover/lesson recordings are excluded from auto-pick (* = recommended)"))
	}
	return strings.Join(lines, "\n")
}
