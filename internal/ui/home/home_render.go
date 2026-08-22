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
		return kit.InfoStyle.Render("⠋ Loading library stats...")
	}

	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(kit.MutedStyle.Render("Guitar tabs in your terminal — browse, play, and search."))
	b.WriteString("\n\n")

	// Stat row: label-above-value columns separated by dim rules. No borders —
	// whitespace and alignment carry the grouping.
	recent := m.recentTabs()
	lastLabel := "—"
	if len(recent) > 0 {
		lastLabel = kit.Truncate(recent[0].Title, 26)
	}
	cols := []struct{ label, value string }{
		{"TABS", fmt.Sprintf("%d", len(m.tabs))},
		{"FAVORITES", fmt.Sprintf("%d *", m.favoriteCount())},
		{"RECENT", lastLabel},
	}
	var colWidths []int
	for _, c := range cols {
		w := lipgloss.Width(c.label)
		if v := lipgloss.Width(c.value); v > w {
			w = v
		}
		if w < 6 {
			w = 6
		}
		colWidths = append(colWidths, w)
	}
	var cells []string
	for i, c := range cols {
		cell := kit.StatLabelStyle.Render(c.label) + "\n" + kit.StatValueStyle.Render(c.value)
		cells = append(cells, lipgloss.NewStyle().Width(colWidths[i]).Render(cell))
		if i < len(cols)-1 {
			// Two-line separator so the value row keeps the column rule.
			cells = append(cells, kit.PanelDividerStyle.Render(" │ \n │ "))
		}
	}
	statLine := lipgloss.JoinHorizontal(lipgloss.Top, cells...)
	if lipgloss.Width(statLine) > m.width-4 {
		// Narrow terminals: fall back to a single-line "label: value" row.
		var flat []string
		for _, c := range cols {
			flat = append(flat, kit.StatLabelStyle.Render(c.label+":")+" "+kit.StatValueStyle.Render(c.value))
		}
		statLine = strings.Join(flat, "  ")
	}
	b.WriteString(statLine)
	b.WriteString("\n")

	if m.autoImportWarn != "" {
		b.WriteString("\n")
		b.WriteString(kit.WarningStyle.Render(m.autoImportWarn))
	}
	if m.errMsg != "" {
		b.WriteString("\n")
		b.WriteString(kit.ErrorStyle.Render(m.errMsg))
	}
	if banner := m.degradedBanner(); banner != "" {
		b.WriteString("\n")
		b.WriteString(kit.WarningStyle.Render(banner))
	}

	b.WriteString("\n\n")
	actions := []struct {
		title string
		desc  string
		key   string
	}{
		{"Library", "Browse and open saved tabs", "l"},
		{"Online Search", "Search Ultimate Guitar + Songsterr", "o"},
		{"Import", "Add tabs from your filesystem", "i"},
	}
	descW := 0
	for _, a := range actions {
		if w := lipgloss.Width(a.desc); w > descW {
			descW = w
		}
	}
	for i, a := range actions {
		line := fmt.Sprintf("%s  %s", a.title, a.desc)
		pad := descW - lipgloss.Width(a.desc)
		if i == m.cursor {
			b.WriteString(kit.ActionSelectedStyle.Render("▸ "+line) + strings.Repeat(" ", pad) + kit.MutedStyle.Render("  ["+a.key+"]"))
		} else {
			b.WriteString("  " + kit.ActionTitleStyle.Render(a.title) + "  " + kit.ActionDescStyle.Render(a.desc) + strings.Repeat(" ", pad))
		}
		b.WriteString("\n")
	}

	if len(recent) > 0 {
		b.WriteString("\n")
		b.WriteString(kit.StatLabelStyle.Render("RECENT TABS"))
		b.WriteString("\n")
		b.WriteString(kit.PanelDividerStyle.Render(strings.Repeat("─", min(m.width-4, 60))))
		b.WriteString("\n")
		for i, row := range recent {
			idx := homeActionCount + i
			star := " "
			if row.Favorite {
				star = "*"
			}
			line := fmt.Sprintf("  %s %s — %s", star, row.Title, row.Artist)
			if m.cursor == idx {
				b.WriteString(kit.ListSelected.Render("▸ "+line) + "\n")
			} else {
				b.WriteString(kit.ListNormal.Render(line) + "\n")
			}
		}
	} else if len(m.tabs) == 0 {
		b.WriteString("\n")
		b.WriteString(kit.WarningStyle.Render("No tabs yet"))
		b.WriteString("\n\n")
		b.WriteString(kit.MutedStyle.Render("Import one from your shell:"))
		b.WriteString("\n")
		b.WriteString(kit.SuccessStyle.Render("  fretboard import samples/sultans.txt"))
		b.WriteString("\n")
	}

	if m.showImportHelp {
		b.WriteString("\n")
		b.WriteString(kit.RenderPanel(m.width-4, "Import tabs", kit.InfoStyle.Render("Run from your shell:")+"\n"+
			kit.SuccessStyle.Render("  fretboard import path/to/tab.txt")+"\n"+
			kit.MutedStyle.Render("  fretboard import path/to/tabs/")+"\n\n"+
			kit.InfoStyle.Render("Backing tracks (optional):")+"\n"+
			kit.MutedStyle.Render("  ~/.config/fretboard/audio/Artist - Title.mp3")+"\n"+
			kit.MutedStyle.Render("  or beside the tab file: layla.mp3")))
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
