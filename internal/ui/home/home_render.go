package home

import (
	"fmt"
	"strings"

	"fretboard/internal/player"
	"fretboard/internal/ui/kit"
	"github.com/charmbracelet/lipgloss"
)

func (m HomeModel) favoriteCount() int {
	n := 0
	for _, t := range m.tabs {
		if t.Favorite {
			n++
		}
	}
	return n
}

func (m HomeModel) renderBody() string {
	if !m.loaded {
		return kit.MutedStyle.Render("loading library…")
	}

	var b strings.Builder
	b.WriteString("\n")

	// The library at a glance: one dim line, no widgetry. Alignment and
	// whitespace carry the page, like tig's main view.
	stats := fmt.Sprintf("%d tabs · %d favorites", len(m.tabs), m.favoriteCount())
	recent := m.recentTabs()
	if len(recent) > 0 {
		stats += fmt.Sprintf(" · last opened %s", kit.Truncate(recent[0].Title, 30))
	}
	b.WriteString(kit.MutedStyle.Render(stats))
	b.WriteString("\n\n")

	if m.autoImportWarn != "" {
		b.WriteString(kit.WarningStyle.Render(m.autoImportWarn) + "\n")
	}
	if m.errMsg != "" {
		b.WriteString(kit.ErrorStyle.Render(m.errMsg) + "\n")
	}
	if banner := m.degradedBanner(); banner != "" {
		b.WriteString(kit.WarningStyle.Render(banner) + "\n")
	}
	if m.autoImportWarn != "" || m.errMsg != "" || m.degradedBanner() != "" {
		b.WriteString("\n")
	}

	// Actions: plain rows — action name in the left column, dim description
	// next to it, selection via the shared highlight style.
	actions := []struct {
		title string
		desc  string
	}{
		{"Library", "browse and open saved tabs"},
		{"Online search", "Ultimate Guitar · Songsterr · GuitarTabs · GuitareTab"},
		{"Import", "add tabs from your filesystem"},
	}
	titleW := 0
	for _, a := range actions {
		if w := lipgloss.Width(a.title); w > titleW {
			titleW = w
		}
	}
	descW := m.width - titleW - 8
	for i, a := range actions {
		desc := a.desc
		if descW > 0 {
			desc = kit.Truncate(desc, descW)
		}
		row := fmt.Sprintf("  %-*s  %s", titleW, a.title, kit.MutedStyle.Render(desc))
		if i == m.cursor {
			b.WriteString(kit.ListSelected.Render(row))
		} else {
			b.WriteString(kit.ListNormal.Render(row))
		}
		b.WriteString("\n")
	}

	if len(recent) > 0 {
		b.WriteString("\n")
		b.WriteString(kit.MutedStyle.Render("RECENT"))
		b.WriteString("\n")
		for i, row := range recent {
			idx := homeActionCount + i
			star := " "
			if row.Favorite {
				star = "*"
			}
			rowStr := fmt.Sprintf("  %s %s — %s", star, row.Title, row.Artist)
			if m.cursor == idx {
				b.WriteString(kit.ListSelected.Render(rowStr))
			} else {
				b.WriteString(kit.ListNormal.Render(rowStr))
			}
			b.WriteString("\n")
		}
	} else if len(m.tabs) == 0 {
		b.WriteString("\n")
		b.WriteString(kit.MutedStyle.Render("No tabs yet — import one from your shell:"))
		b.WriteString("\n")
		b.WriteString(kit.SuccessStyle.Render("  fretboard import samples/sultans.txt"))
		b.WriteString("\n")
	}

	if m.showImportHelp {
		b.WriteString("\n")
		b.WriteString(kit.MutedStyle.Render("import from your shell:"))
		b.WriteString("\n")
		b.WriteString(kit.SuccessStyle.Render("  fretboard import path/to/tab.txt"))
		b.WriteString("\n")
		b.WriteString(kit.SuccessStyle.Render("  fretboard import path/to/tabs/"))
		b.WriteString("\n")
		b.WriteString(kit.MutedStyle.Render("backing tracks (optional):"))
		b.WriteString("\n")
		b.WriteString(kit.ListNormal.Render("  ~/.config/fretboard/audio/Artist - Title.mp3"))
		b.WriteString("\n")
		b.WriteString(kit.ListNormal.Render("  or beside the tab file: layla.mp3"))
		b.WriteString("\n")
	}

	if m.preview != "" {
		b.WriteString("\n")
		b.WriteString(m.preview)
		b.WriteString("\n")
	}

	if warn := m.audioWarning(); warn != "" {
		b.WriteString("\n")
		b.WriteString(kit.WarningStyle.Render(warn))
		b.WriteString("\n")
	}
	return b.String()
}

func (m HomeModel) audioWarning() string {
	if player.ResolveSoundfont() == "" {
		return "No soundfont found — install soundfont-fluid or set FRETBOARD_SOUNDFONT"
	}
	if !player.SynthAvailable() {
		return "fluidsynth not found — install fluidsynth"
	}
	if player.OnlineAudioAvailable() {
		return ""
	}
	return "yt-dlp not found — install for automatic song audio"
}

// degradedBanner renders the one-line missing-dependency banner: which
// critical tool group is absent and what playback it disables, pointing at
// `fretboard doctor` for the full report (8.2).
func (m HomeModel) degradedBanner() string {
	if len(m.missingDeps) == 0 {
		return ""
	}
	var b strings.Builder
	for _, name := range m.missingDeps {
		impact := "audio playback unavailable"
		if strings.Contains(name, "fluidsynth") || strings.Contains(name, "timidity") {
			impact = "MIDI playback unavailable"
		}
		b.WriteString("missing: " + name + " — " + impact + " (see `fretboard doctor`)")
		b.WriteString("\n")
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// View renders the landing page.
func (m HomeModel) View() string {
	status := ""
	if len(m.missingDeps) > 0 {
		status = "⚠ missing: " + strings.Join(m.missingDeps, ", ")
	}
	footer := kit.RenderFooterWithStatus(m.width, status, []kit.KeyHint{
		{Key: "j/k", Label: "navigate"},
		{Key: "Enter", Label: "select"},
		{Key: "l", Label: "library"},
		{Key: "o", Label: "search"},
		{Key: "i", Label: "import"},
		{Key: "q", Label: "quit"},
	})
	return kit.LayoutScreen(m.width, m.height, kit.FormatBreadcrumb("home"), m.renderBody(), footer)
}
