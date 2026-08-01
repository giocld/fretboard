package tui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/YOUR_USERNAME/fretboard/internal/library"
	"github.com/YOUR_USERNAME/fretboard/internal/model"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/sahilm/fuzzy"
)

// SortMode controls how library rows are ordered.
type SortMode int

const (
	SortRecent SortMode = iota
	SortAlpha
	SortArtist
	SortPlays
)

// BrowserModel is the library tab list.
type BrowserModel struct {
	store        *library.Store
	tabs         []library.TabRow
	filtered     []library.TabRow
	cursor       int
	searchActive bool
	searchInput  string
	sortMode     SortMode
	viewport     viewport.Model
	width        int
	height       int
	loaded       bool
	loading      bool
	errMsg         string
	autoImportWarn string
	confirmDelete  *library.TabRow
}

// NewBrowserModel creates a browser bound to a library store.
func NewBrowserModel(store *library.Store) BrowserModel {
	vp := viewport.New(80, 20)
	return BrowserModel{
		store:    store,
		viewport: vp,
		width:    80,
		height:   24,
	}
}

// Init loads tabs from the library on startup.
func (m BrowserModel) Init() tea.Cmd {
	return func() tea.Msg {
		if m.store == nil {
			return TabsLoadedMsg{Tabs: nil}
		}
		tabs, err := m.store.List()
		if err != nil {
			return TabsLoadErrorMsg{Err: err}
		}
		return TabsLoadedMsg{Tabs: tabs}
	}
}

// TabsLoadedMsg is sent when the library list has been loaded.
type TabsLoadedMsg struct {
	Tabs []library.TabRow
}

// TabsLoadErrorMsg is sent when the library list fails to load.
type TabsLoadErrorMsg struct {
	Err error
}

// TabSelectedMsg is sent when the user opens a tab.
type TabSelectedMsg struct {
	ID int64
}

// GoHomeMsg navigates back to the landing page.
type GoHomeMsg struct{}

// Update handles key events and library messages.
func (m BrowserModel) Update(msg tea.Msg) (BrowserModel, tea.Cmd) {
	switch msg := msg.(type) {
	case TabsLoadedMsg:
		m.tabs = msg.Tabs
		m.loaded = true
		m.loading = false
		m.errMsg = ""
		m.apply()
	case AutoImportWarnMsg:
		m.autoImportWarn = msg.Msg
	case TabsLoadErrorMsg:
		m.loaded = true
		m.loading = false
		if msg.Err != nil {
			m.errMsg = "Could not load library: " + msg.Err.Error()
		} else {
			m.errMsg = "Could not load library"
		}
		m.apply()
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = msg.Width - 4
		bodyH := msg.Height - 5
		if bodyH < 3 {
			bodyH = 3
		}
		m.viewport.Height = bodyH
		m.refresh()
	case tea.KeyMsg:
		if m.searchActive {
			return m.handleSearchKey(msg)
		}
		return m.handleNormalKey(msg)
	}
	return m, nil
}

func (m BrowserModel) handleSearchKey(msg tea.KeyMsg) (BrowserModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if m.searchInput != "" {
			m.searchInput = ""
			m.apply()
		} else {
			m.searchActive = false
			return m, func() tea.Msg { return GoHomeMsg{} }
		}
	case "o":
		m.searchActive = false
		m.searchInput = ""
		return m, func() tea.Msg { return HomeSearchMsg{} }
	case "enter":
		if m.cursor >= 0 && m.cursor < len(m.filtered) {
			m.searchActive = false
			m.searchInput = ""
			return m, func() tea.Msg {
				return TabSelectedMsg{ID: m.filtered[m.cursor].ID}
			}
		}
		// Stay in search mode so the user can keep typing.
		m.apply()
	case "down":
		m.moveCursor(1)
	case "up":
		m.moveCursor(-1)
	case "backspace":
		r := []rune(m.searchInput)
		if len(r) > 0 {
			m.searchInput = string(r[:len(r)-1])
			m.cursor = 0
			m.apply()
		}
	case KeyQuit2:
		return m, tea.Quit
	default:
		if len(msg.String()) == 1 && msg.String()[0] >= 32 && msg.String()[0] < 127 {
			m.searchInput += msg.String()
			m.cursor = 0
			m.apply()
		}
	}
	return m, nil
}

func (m BrowserModel) handleNormalKey(msg tea.KeyMsg) (BrowserModel, tea.Cmd) {
	if m.confirmDelete != nil {
		switch msg.String() {
		case "y", "d", "enter":
			row := *m.confirmDelete
			m.confirmDelete = nil
			if m.store != nil {
				if err := m.store.Delete(row.ID); err == nil {
					m.errMsg = ""
					m.removeTabRow(row.ID)
					m.apply()
				} else {
					m.errMsg = "Delete failed: " + err.Error()
					m.refresh()
				}
			}
		case "n", "esc":
			m.confirmDelete = nil
		}
		return m, nil
	}

	switch msg.String() {
	case KeyQuit, KeyQuit2:
		return m, tea.Quit
	case "down", KeyDown:
		m.moveCursor(1)
	case "up", KeyUp:
		m.moveCursor(-1)
	case "enter":
		if m.cursor >= 0 && m.cursor < len(m.filtered) {
			return m, func() tea.Msg {
				return TabSelectedMsg{ID: m.filtered[m.cursor].ID}
			}
		}
	case "f":
		if m.store != nil && m.cursor >= 0 && m.cursor < len(m.filtered) {
			row := m.filtered[m.cursor]
			newFav := !row.Favorite
			if err := m.store.SetFavorite(row.ID, newFav); err == nil {
				m.errMsg = ""
				m.updateTabRow(row.ID, func(r *library.TabRow) { r.Favorite = newFav })
				m.apply()
			} else if err != nil {
				m.errMsg = "Favorite failed: " + err.Error()
				m.refresh()
			}
		}
	case "d":
		if m.store != nil && m.cursor >= 0 && m.cursor < len(m.filtered) {
			row := m.filtered[m.cursor]
			copy := row
			m.confirmDelete = &copy
		}
	case "r":
		if m.store != nil {
			m.loading = true
			m.errMsg = ""
			m.refresh()
			return m, m.Init()
		}
	case "s":
		m.sortMode = (m.sortMode + 1) % 4
		m.apply()
	case "/":
		m.searchActive = true
		m.cursor = 0
		m.refresh()
	case "esc", "h":
		if m.searchInput != "" {
			m.searchInput = ""
			m.searchActive = false
			m.cursor = 0
			m.apply()
			return m, nil
		}
		if m.searchActive {
			m.searchActive = false
			m.apply()
			return m, nil
		}
		return m, func() tea.Msg { return GoHomeMsg{} }
	}
	return m, nil
}

func (m *BrowserModel) moveCursor(delta int) {
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if len(m.filtered) > 0 && m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}
	m.ensureCursorVisible()
	m.refresh()
}

func (m *BrowserModel) ensureCursorVisible() {
	offset := 0
	if m.searchActive || m.searchInput != "" {
		offset = 2
	}
	target := offset + m.cursor
	if target < m.viewport.YOffset {
		m.viewport.SetYOffset(target)
	} else if target >= m.viewport.YOffset+m.viewport.Height {
		m.viewport.SetYOffset(target - m.viewport.Height + 1)
	}
}

func (m *BrowserModel) updateTabRow(id int64, fn func(*library.TabRow)) {
	for i := range m.tabs {
		if m.tabs[i].ID == id {
			fn(&m.tabs[i])
			return
		}
	}
}

func (m *BrowserModel) removeTabRow(id int64) {
	out := m.tabs[:0]
	for _, r := range m.tabs {
		if r.ID != id {
			out = append(out, r)
		}
	}
	m.tabs = out
}

func (m *BrowserModel) apply() {
	m.filtered = m.filterAndSort(m.tabs)
	if len(m.filtered) == 0 {
		m.cursor = 0
	} else {
		if m.cursor >= len(m.filtered) {
			m.cursor = len(m.filtered) - 1
		}
		if m.cursor < 0 {
			m.cursor = 0
		}
	}
	m.ensureCursorVisible()
	m.refresh()
}

func (m BrowserModel) filterAndSort(rows []library.TabRow) []library.TabRow {
	out := make([]library.TabRow, len(rows))
	copy(out, rows)

	if m.searchInput != "" {
		var targets []string
		for _, r := range rows {
			targets = append(targets, r.Title+" "+r.Artist+" "+formatRowTuning(r.Tuning))
		}
		matches := fuzzy.Find(m.searchInput, targets)
		var filtered []library.TabRow
		seen := make(map[int]bool)
		for _, m := range matches {
			if !seen[m.Index] {
				seen[m.Index] = true
				filtered = append(filtered, rows[m.Index])
			}
		}
		out = filtered
	}

	switch m.sortMode {
	case SortRecent:
		sort.Slice(out, func(i, j int) bool {
			return library.MoreRecentlyUsed(out[i], out[j])
		})
	case SortAlpha:
		sort.Slice(out, func(i, j int) bool {
			return strings.ToLower(out[i].Title) < strings.ToLower(out[j].Title)
		})
	case SortArtist:
		sort.Slice(out, func(i, j int) bool {
			return strings.ToLower(out[i].Artist) < strings.ToLower(out[j].Artist)
		})
	case SortPlays:
		sort.Slice(out, func(i, j int) bool {
			return out[i].PlayCount > out[j].PlayCount
		})
	}
	return out
}

func (m *BrowserModel) refresh() {
	m.viewport.SetContent(m.renderList())
}

func (m BrowserModel) renderList() string {
	if m.loading {
		return infoStyle.Render("⠋ Reloading library…")
	}
	if !m.loaded {
		return infoStyle.Render("Loading library…")
	}
	if len(m.tabs) == 0 {
		return warningStyle.Render("No tabs in library.") + "\n" +
			mutedStyle.Render("Import one with: fretboard import <file>")
	}
	var b strings.Builder
	if m.searchActive {
		b.WriteString(infoStyle.Render("Search: ") + m.searchInput + mutedStyle.Render("_"))
		b.WriteString("\n\n")
	} else if m.searchInput != "" {
		b.WriteString(mutedStyle.Render("Filter: " + m.searchInput))
		b.WriteString("\n\n")
	}
	if len(m.filtered) == 0 {
		b.WriteString(warningStyle.Render("No matches."))
		b.WriteString("\n")
		return b.String()
	}
	for i, row := range m.filtered {
		star := mutedStyle.Render(" ")
		if row.Favorite {
			star = successStyle.Render("★")
		}
		meta := mutedStyle.Render(formatRowTuning(row.Tuning))
		line := fmt.Sprintf("%s  %s — %s", star, row.Title, row.Artist)
		if i == m.cursor {
			b.WriteString(listSelected.Render("▸ "+line) + "  " + meta)
		} else {
			b.WriteString(listNormal.Render(line) + "  " + meta)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// View renders the browser.
func (m BrowserModel) View() string {
	count := fmt.Sprintf("%d tabs · sort: %s", len(m.filtered), sortLabel(m.sortMode))
	panel := RenderPanel(m.width-2, count, m.viewport.View())
	body := "\n" + panel
	if m.confirmDelete != nil {
		body += "\n" + warningStyle.Render(fmt.Sprintf(
			"Delete %q? [y]es [n]o (irreversible)", m.confirmDelete.Title))
	}
	if m.autoImportWarn != "" {
		body += "\n" + warningStyle.Render(m.autoImportWarn)
	}
	if m.errMsg != "" {
		body += "\n" + errorStyle.Render(m.errMsg)
	}
	hints := []KeyHint{
		{Key: "j/k", Label: "move"},
		{Key: "Enter", Label: "open"},
		{Key: "/", Label: "filter"},
		{Key: "s", Label: "sort"},
		{Key: "f", Label: "favorite"},
		{Key: "d", Label: "delete"},
		{Key: "o", Label: "online"},
		{Key: "Esc", Label: "home"},
		{Key: "?", Label: "help"},
		{Key: "q", Label: "quit"},
	}
	if m.confirmDelete != nil {
		hints = []KeyHint{
			{Key: "y", Label: "delete"},
			{Key: "n/Esc", Label: "cancel"},
		}
	} else if m.searchActive {
		hints = []KeyHint{
			{Key: "type", Label: "filter"},
			{Key: "↑/↓", Label: "move"},
			{Key: "Enter", Label: "open"},
			{Key: "Esc", Label: "clear/home"},
			{Key: "o", Label: "online"},
			{Key: "q", Label: "quit"},
		}
	}
	footer := RenderFooter(m.width, hints)
	return LayoutScreen(m.width, m.height, FormatBreadcrumb("home", "library"), body, footer)
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
	if label := t.Label(); label != "" {
		return label
	}
	return raw
}

func (m *BrowserModel) resetFilter() {
	m.searchActive = false
	m.searchInput = ""
	m.errMsg = ""
	m.apply()
}
