package browser

import (
	"encoding/json"
	"fmt"
	"strings"

	"fretboard/internal/model"
	"fretboard/internal/ui/kit"
	"fretboard/internal/ui/msgs"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Preview panel layout: the browser splits into list + preview when the
// terminal is wide enough for both to stay usable.
const (
	previewPanelWidth = 42
	splitMinWidth     = 60 + 2 + previewPanelWidth + 2
)

// requestPreview returns a command that renders the selected row's tab for
// the right-side preview panel. Rows already previewed are skipped.
func (m *BrowserModel) requestPreview() tea.Cmd {
	if m.store == nil || m.cursor < 0 || m.cursor >= len(m.filtered) {
		return nil
	}
	row := m.filtered[m.cursor]
	if m.previewTabID == row.ID && m.preview != "" {
		return nil
	}
	m.previewGen++
	gen := m.previewGen
	m.previewTabID = row.ID
	m.previewTitle = row.Title
	return func() tea.Msg {
		tab, err := m.store.Get(row.ID)
		if err != nil || tab == nil {
			return msgs.BrowserPreviewMsg{Gen: gen, TabID: row.ID, Err: err}
		}
		return msgs.BrowserPreviewMsg{Gen: gen, TabID: row.ID, Title: row.Title, Preview: kit.RenderTabPreview(tab, 12)}
	}
}

// splitActive reports whether the preview panel is shown beside the list.
func (m BrowserModel) splitActive() bool {
	return m.preview != "" && m.width >= splitMinWidth
}

func (m BrowserModel) renderList() string {
	if m.loading {
		return kit.InfoStyle.Render("⠋ Reloading library...")
	}
	if !m.loaded {
		return kit.InfoStyle.Render("Loading library...")
	}
	if len(m.tabs) == 0 {
		return "\n\n  " + kit.WarningStyle.Render("No tabs in your library") + "\n\n  " +
			kit.MutedStyle.Render("Import one from your shell:") + "\n  " +
			kit.SuccessStyle.Render("fretboard import <file-or-directory>") + "\n"
	}
	var b strings.Builder
	if m.searchActive {
		b.WriteString(kit.InfoStyle.Render("Search: ") + m.searchInput + kit.MutedStyle.Render("_"))
		b.WriteString("\n")
	} else if m.searchInput != "" {
		b.WriteString(kit.MutedStyle.Render("Filter: " + m.searchInput))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	if len(m.filtered) == 0 {
		b.WriteString("  " + kit.WarningStyle.Render("No matches for \""+m.searchInput+"\""))
		b.WriteString("\n  " + kit.MutedStyle.Render("Press Esc to clear the filter."))
		b.WriteString("\n")
		return b.String()
	}

	// Table header with a sort indicator on the active column.
	header := fmt.Sprintf("%-3s %-34s %-24s %s", " ", "TITLE", "ARTIST", "TUNING")
	switch m.sortMode {
	case SortAlpha:
		header = fmt.Sprintf("%-3s %-34s %-24s %s", " ", "TITLE ▼", "ARTIST", "TUNING")
	case SortArtist:
		header = fmt.Sprintf("%-3s %-34s %-24s %s", " ", "TITLE", "ARTIST ▼", "TUNING")
	case SortPlays:
		header = fmt.Sprintf("%-3s %-34s %-24s %s", " ", "TITLE", "ARTIST", "PLAYS ▼")
	}
	b.WriteString(kit.TableHeaderStyle.Render(header))
	b.WriteString("\n")
	b.WriteString(kit.PanelDividerStyle.Render(strings.Repeat("─", 3+34+24+10)))
	b.WriteString("\n")
	for i, row := range m.filtered {
		star := " "
		if row.Favorite {
			star = "*"
		}
		tuning := formatRowTuning(row.Tuning)
		if row.SourceBadge != "" {
			// Provenance beats a standard tuning label for online tabs: it
			// tells the user which source and how well rated the tab is.
			tuning = kit.MutedStyle.Render(row.SourceBadge)
		}
		line := fmt.Sprintf("%-3s %-34s %-24s %s", star, kit.Truncate(row.Title, 34), kit.Truncate(row.Artist, 24), tuning)
		if m.sortMode == SortPlays {
			line = fmt.Sprintf("%-3s %-34s %-24s %d", star, kit.Truncate(row.Title, 34), kit.Truncate(row.Artist, 24), row.PlayCount)
		}
		if i == m.cursor {
			b.WriteString(kit.ListSelected.Render("▸ " + line[2:]))
		} else {
			b.WriteString(kit.ListNormal.Render(line))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// View renders the browser.
func (m BrowserModel) View() string {
	status := fmt.Sprintf("%d tabs · %s", len(m.filtered), sortLabel(m.sortMode))
	if m.favOnly {
		status = fmt.Sprintf("%d favs · %s", len(m.filtered), sortLabel(m.sortMode))
	}
	var body string
	if m.editing && m.editRow != nil {
		label := "Title"
		if m.editField == 2 {
			label = "Artist"
		}
		body += "\n" + kit.RenderPanel(m.width-2, "Edit "+label, m.editInput.View()) + "\n"
	}
	if m.splitActive() {
		listW := m.width - 2 - previewPanelWidth - 2
		panel := kit.RenderPanel(listW, "", m.viewport.View())
		previewBody := m.preview
		// Pad the preview to the list panel's height so both borders line up.
		if pad := strings.Count(m.viewport.View(), "\n") - strings.Count(previewBody, "\n"); pad > 0 {
			previewBody += strings.Repeat("\n", pad)
		}
		title := "Preview"
		if m.previewTitle != "" {
			title += " · " + kit.Truncate(m.previewTitle, previewPanelWidth-6)
		}
		previewPanel := kit.RenderPanel(previewPanelWidth, title, previewBody)
		body = "\n" + lipgloss.JoinHorizontal(lipgloss.Top, panel, "  ", previewPanel)
	} else {
		body = "\n" + kit.RenderPanel(m.width-2, "", m.viewport.View())
	}
	if m.confirmDelete != nil {
		body += "\n" + kit.WarningStyle.Render(fmt.Sprintf(
			"Delete %q? [y]es [n]o (irreversible)", m.confirmDelete.Title))
	}
	if m.autoImportWarn != "" {
		body += "\n" + kit.WarningStyle.Render(m.autoImportWarn)
	}
	if m.errMsg != "" {
		body += "\n" + kit.ErrorStyle.Render(m.errMsg)
	}
	hints := []kit.KeyHint{
		{Key: "j/k", Label: "move"},
		{Key: "Enter", Label: "open"},
		{Key: "/", Label: "filter"},
		{Key: "s", Label: "sort"},
		{Key: "f", Label: "fav"},
		{Key: "F", Label: "favs"},
		{Key: "e", Label: "edit"},
		{Key: "x", Label: "export"},
		{Key: "d", Label: "delete"},
		{Key: "o", Label: "online"},
		{Key: "Esc", Label: "home"},
		{Key: "q", Label: "quit"},
	}
	if m.confirmDelete != nil {
		hints = []kit.KeyHint{
			{Key: "y", Label: "delete"},
			{Key: "n/Esc", Label: "cancel"},
		}
	} else if m.editing {
		hints = []kit.KeyHint{
			{Key: "type", Label: "value"},
			{Key: "Enter", Label: "save/next"},
			{Key: "Esc", Label: "cancel"},
		}
	} else if m.searchActive {
		hints = []kit.KeyHint{
			{Key: "type", Label: "filter"},
			{Key: "↑/↓", Label: "move"},
			{Key: "Enter", Label: "open"},
			{Key: "Esc", Label: "clear/home"},
			{Key: "q", Label: "quit"},
		}
	}
	footer := kit.RenderFooterWithStatus(m.width, status, hints)
	return kit.LayoutScreen(m.width, m.height, kit.FormatBreadcrumb("home", "library"), body, footer)
}

func sortLabel(s SortMode) string {
	switch s {
	case SortRecent:
		return "recent"
	case SortAlpha:
		return "alpha"
	case SortArtist:
		return "artist"
	case SortPlays:
		return "plays"
	}
	return "?"
}

func formatRowTuning(raw string) string {
	if raw == "" {
		return ""
	}
	var t model.Tuning
	if err := json.Unmarshal([]byte(raw), &t); err != nil {
		return raw
	}
	if len(t) == 0 {
		return "" // "null"/"[]" must not render as literal text
	}
	if label := t.Label(); label != "" {
		return label
	}
	return raw
}
