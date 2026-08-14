package search

import (
	"fmt"
	"strings"

	"fretboard/internal/scraper"
	"fretboard/internal/ui/kit"
)

func (m SearchModel) renderResults() string {
	if m.loading {
		if m.importing {
			return kit.InfoStyle.Render("⠋ Fetching tab...")
		}
		return kit.InfoStyle.Render("⠋ Searching...")
	}
	if m.errMsg != "" {
		return kit.ErrorStyle.Render(m.errMsg)
	}
	if m.cacheNote != "" {
		return kit.WarningStyle.Render(m.cacheNote) + "\n\n" + m.renderResultList()
	}
	if len(m.results) == 0 {
		hint := "Type a query and press Enter to search."
		if m.inputActive {
			hint += "  Tab/j/k move through results after a search."
		}
		return kit.MutedStyle.Render(hint)
	}
	return m.renderResultList()
}

func (m SearchModel) renderResultList() string {
	var b strings.Builder
	for i, r := range m.results {
		line := formatResult(r)
		if i == m.cursor {
			b.WriteString(kit.ListSelected.Render("▸ " + line))
		} else {
			b.WriteString(kit.ListNormal.Render("  " + line))
		}
		b.WriteString("\n")
	}
	if !m.inputActive {
		b.WriteString("\n")
		b.WriteString(kit.MutedStyle.Render("/ or i — edit query   Enter — open tab   m — more results"))
	}
	return b.String()
}

func formatResult(r scraper.SearchResult) string {
	badge := kit.InfoStyle.Render("[UG]")
	switch r.Source {
	case scraper.SourceSongsterr:
		badge = kit.SuccessStyle.Render("[ST]")
	case scraper.SourceGuitarTabs:
		badge = kit.WarningStyle.Render("[GT]")
	case scraper.SourceGuitareTab:
		badge = kit.HighlightStyle.Render("[GR]")
	}
	// Performance type: tabs are what most queries want; chord sheets and
	// bass parts are labeled so they are recognizable before fetching.
	typeBadge := ""
	switch strings.ToLower(r.Type) {
	case "tabs", "tab", "tab pro", "pro", "bass", "bass tabs":
		typeBadge = kit.MutedStyle.Render("TAB")
	case "chords", "chord":
		typeBadge = kit.WarningStyle.Render("CHD")
	}
	// Rating + vote count from the source (UG), plus a top-rated marker for
	// strongly-voted tabs — the official version is recognizable at a glance.
	rating := ""
	if r.Rating > 0 {
		rating = kit.SuccessStyle.Render(fmt.Sprintf("*%.1f", r.Rating))
		if r.Votes > 0 {
			rating = kit.SuccessStyle.Render(fmt.Sprintf("*%.1f · %s", r.Rating, shortVotes(r.Votes)))
		}
	}
	top := ""
	if scraper.IsTopRated(r) {
		top = kit.SuccessStyle.Render("top")
	}
	parts := []string{badge + " " + r.SongName + " — " + r.ArtistName}
	if typeBadge != "" {
		parts = append(parts, typeBadge)
	}
	if rating != "" {
		parts = append(parts, rating)
	}
	if top != "" {
		parts = append(parts, top)
	}
	return strings.Join(parts, "  ")
}

// shortVotes renders a vote count compactly: 2100 -> "2.1k", 500 -> "500".
func shortVotes(v int64) string {
	if v >= 1000 {
		return fmt.Sprintf("%.1fk", float64(v)/1000.0)
	}
	return fmt.Sprintf("%d", v)
}

// View renders the search screen as a single panel: query line, divider,
// results — one surface instead of two stacked boxes. Focus is shown on the
// query line itself (prompt + ●), never by a second border.
func (m SearchModel) View() string {
	queryLine := m.input.View()
	if m.inputActive {
		queryLine += kit.SuccessStyle.Render("  ●")
	}
	innerW := m.width - 6
	content := queryLine + "\n" + kit.RenderDivider(innerW) + "\n" + m.viewport.View()
	panel := kit.RenderPanel(m.width-2, "", content)
	footer := kit.RenderFooter(m.width, []kit.KeyHint{
		{Key: "Enter", Label: "search/open"},
		{Key: "Tab", Label: "results"},
		{Key: "/", Label: "query"},
		{Key: "j/k", Label: "move"},
		{Key: "Esc", Label: "back"},
		{Key: "q", Label: "quit"},
	})
	return kit.LayoutScreen(m.width, m.height, kit.FormatBreadcrumb("home", "search"), "\n"+panel, footer)
}
