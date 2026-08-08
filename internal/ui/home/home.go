package home

import (
	"fmt"
	"sort"
	"strings"

	"fretboard/internal/library"
	"fretboard/internal/player"
	"fretboard/internal/ui/kit"
	"fretboard/internal/ui/msgs"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const homeActionCount = 3

// HomeModel is the landing screen shown when the app starts.
type HomeModel struct {
	store          *library.Store
	tabs           []library.TabRow
	cursor         int
	loaded         bool
	showImportHelp bool
	preview        string
	errMsg         string
	autoImportWarn string
	width          int
	height         int
}

// NewHomeModel creates the landing page.
func NewHomeModel(store *library.Store) HomeModel {
	return HomeModel{
		store:  store,
		width:  80,
		height: 24,
	}
}

// Init loads library stats for the dashboard widgets.
func (m HomeModel) Init() tea.Cmd {
	return func() tea.Msg {
		if m.store == nil {
			return msgs.TabsLoadedMsg{Tabs: nil}
		}
		tabs, err := m.store.List()
		if err != nil {
			return msgs.TabsLoadErrorMsg{Err: err}
		}
		return msgs.TabsLoadedMsg{Tabs: tabs}
	}
}

// Update handles landing page input.
func (m HomeModel) Update(msg tea.Msg) (HomeModel, tea.Cmd) {
	switch msg := msg.(type) {
	case msgs.TabsLoadedMsg:
		m.tabs = msg.Tabs
		m.loaded = true
		m.errMsg = ""
		m.clampCursor()
		m.preview = m.loadPreview()
	case msgs.AutoImportWarnMsg:
		m.autoImportWarn = msg.Msg
	case msgs.TabsLoadErrorMsg:
		m.loaded = true
		if msg.Err != nil {
			m.errMsg = "Could not load library: " + msg.Err.Error()
		} else {
			m.errMsg = "Could not load library"
		}
		m.preview = ""
		m.clampCursor()
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.loaded {
			m.preview = m.loadPreview()
		}
	case tea.KeyMsg:
		switch msg.String() {
		case kit.KeyQuit, kit.KeyQuit2:
			return m, tea.Quit
		case "esc":
			if m.showImportHelp {
				m.showImportHelp = false
				return m, nil
			}
		case "j", "down":
			m.moveCursor(1)
		case "k", "up":
			m.moveCursor(-1)
		case "l":
			return m, func() tea.Msg { return msgs.HomeLibraryMsg{} }
		case "o":
			return m, func() tea.Msg { return msgs.HomeSearchMsg{} }
		case "i":
			m.showImportHelp = !m.showImportHelp
		case "enter":
			if m.cursor == 2 {
				m.showImportHelp = !m.showImportHelp
				return m, nil
			}
			return m, m.activate()
		}
	}
	return m, nil
}

func (m *HomeModel) clampCursor() {
	max := m.maxCursor()
	if m.cursor > max {
		m.cursor = max
	}
}

func (m *HomeModel) moveCursor(delta int) {
	max := m.maxCursor()
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor > max {
		m.cursor = max
	}
	if m.loaded {
		m.preview = m.loadPreview()
	}
}

func (m HomeModel) maxCursor() int {
	n := homeActionCount - 1
	if len(m.recentTabs()) > 0 {
		n = homeActionCount + len(m.recentTabs()) - 1
	}
	return n
}

func (m HomeModel) activate() tea.Cmd {
	if m.cursor < homeActionCount {
		switch m.cursor {
		case 0:
			return func() tea.Msg { return msgs.HomeLibraryMsg{} }
		case 1:
			return func() tea.Msg { return msgs.HomeSearchMsg{} }
		case 2:
			return nil // import help toggled via showImportHelp in Update
		case 3:
			return func() tea.Msg { return msgs.HomeSettingsMsg{} }
		}
	}
	recent := m.recentTabs()
	idx := m.cursor - homeActionCount
	if idx >= 0 && idx < len(recent) {
		id := recent[idx].ID
		return func() tea.Msg { return msgs.TabSelectedMsg{ID: id} }
	}
	return nil
}

func (m HomeModel) recentTabs() []library.TabRow {
	if len(m.tabs) == 0 {
		return nil
	}
	sorted := make([]library.TabRow, len(m.tabs))
	copy(sorted, m.tabs)
	sort.Slice(sorted, func(i, j int) bool {
		return library.MoreRecentlyUsed(sorted[i], sorted[j])
	})
	n := 3
	if len(sorted) < n {
		n = len(sorted)
	}
	return sorted[:n]
}

func (m *HomeModel) loadPreview() string {
	recent := m.recentTabs()
	if len(recent) == 0 || m.store == nil {
		return ""
	}
	if m.cursor < homeActionCount {
		return ""
	}
	idx := m.cursor - homeActionCount
	if idx < 0 || idx >= len(recent) {
		return ""
	}
	row := recent[idx]
	tab, err := m.store.Get(row.ID)
	if err != nil || tab == nil {
		return ""
	}
	title := row.Title
	if title == "" {
		title = "Preview"
	}
	return kit.RenderPanel(m.width-4, "Preview · "+title, kit.RenderTabPreview(tab, 10))
}

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

// View renders the landing page.
func (m HomeModel) View() string {
	footer := kit.RenderFooter(m.width, []kit.KeyHint{
		{Key: "j/k", Label: "navigate"},
		{Key: "Enter", Label: "select"},
		{Key: "l", Label: "library"},
		{Key: "o", Label: "search"},
		{Key: "i", Label: "import"},
		{Key: "?", Label: "help"},
		{Key: "t", Label: "theme"},
		{Key: "q", Label: "quit"},
	})
	return kit.LayoutScreen(m.width, m.height, kit.FormatBreadcrumb("home"), m.renderBody(), footer)
}

// SetAutoImportWarn updates the auto-import warning banner shown on home.
func (m *HomeModel) SetAutoImportWarn(msg string) {
	m.autoImportWarn = msg
}
