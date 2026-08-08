package browser

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"fretboard/internal/export"
	"fretboard/internal/library"
	"fretboard/internal/model"
	"fretboard/internal/ui/kit"
	"fretboard/internal/ui/msgs"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	store          *library.Store
	tabs           []library.TabRow
	filtered       []library.TabRow
	cursor         int
	searchActive   bool
	searchInput    string
	favOnly        bool
	sortMode       SortMode
	viewport       viewport.Model
	width          int
	height         int
	loaded         bool
	loading        bool
	errMsg         string
	autoImportWarn string
	confirmDelete  *library.TabRow
	editing        bool
	editField      int // 1 = title, 2 = artist
	editInput      textinput.Model
	editRow        *library.TabRow
	preview        string
	previewTitle   string
	previewTabID   int64
	previewGen     int
}

// Preview panel layout: the browser splits into list + preview when the
// terminal is wide enough for both to stay usable.
const (
	previewPanelWidth = 42
	splitMinWidth     = 60 + 2 + previewPanelWidth + 2
)

// NewBrowserModel creates a browser bound to a library store.
func NewBrowserModel(store *library.Store) BrowserModel {
	vp := viewport.New(80, 20)
	ti := textinput.New()
	ti.CharLimit = 120
	return BrowserModel{
		store:     store,
		viewport:  vp,
		editInput: ti,
		width:     80,
		height:    24,
	}
}

// Init loads tabs from the library on startup.
func (m BrowserModel) Init() tea.Cmd {
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

// Update handles key events and library messages.
func (m BrowserModel) Update(msg tea.Msg) (BrowserModel, tea.Cmd) {
	switch msg := msg.(type) {
	case msgs.TabsLoadedMsg:
		m.tabs = msg.Tabs
		m.loaded = true
		m.loading = false
		m.errMsg = ""
		return m, m.apply()
	case msgs.BrowserPreviewMsg:
		if msg.Gen != m.previewGen {
			return m, nil
		}
		if msg.Err != nil || msg.Preview == "" {
			m.preview = ""
			m.previewTitle = ""
			m.previewTabID = 0
		} else {
			m.preview = msg.Preview
			m.previewTitle = msg.Title
			m.previewTabID = msg.TabID
		}
		m.refresh()
		return m, nil
	case msgs.AutoImportWarnMsg:
		m.autoImportWarn = msg.Msg
	case msgs.TabsLoadErrorMsg:
		m.loaded = true
		m.loading = false
		if msg.Err != nil {
			m.errMsg = "Could not load library: " + msg.Err.Error()
		} else {
			m.errMsg = "Could not load library"
		}
		return m, m.apply()
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
	case tea.MouseMsg:
		// Wheel scrolls the library list like j/k.
		switch msg.Type {
		case tea.MouseWheelUp:
			return m, m.moveCursor(-1)
		case tea.MouseWheelDown:
			return m, m.moveCursor(1)
		}
	case tea.KeyMsg:
		if m.editing {
			return m.handleEditKey(msg)
		}
		if m.searchActive {
			return m.handleSearchKey(msg)
		}
		return m.handleNormalKey(msg)
	}
	return m, nil
}

// handleEditKey drives the two-step title/artist editor (`e`). The input
// starts empty with the current value as placeholder — type to replace,
// Enter saves and moves to the next field, empty Enter keeps the old value,
// Esc cancels. The note warns that a later file re-import overwrites edits.
func (m BrowserModel) handleEditKey(msg tea.KeyMsg) (BrowserModel, tea.Cmd) {
	if m.editRow == nil {
		m.editing = false
		return m, nil
	}
	switch msg.String() {
	case "esc":
		m.editing = false
		m.editRow = nil
		m.editInput.Blur()
		m.errMsg = ""
		m.refresh()
		return m, nil
	case "enter":
		val := strings.TrimSpace(m.editInput.Value())
		if m.editField == 1 {
			if val != "" {
				if err := m.store.UpdateMeta(m.editRow.ID, val, m.editRow.Artist); err != nil {
					m.errMsg = "Save failed: " + err.Error()
					m.editing = false
					m.editRow = nil
					m.refresh()
					return m, nil
				}
				m.editRow.Title = val
			}
			m.editField = 2
			m.editInput.SetValue("")
			m.editInput.Placeholder = m.editRow.Artist
			m.editInput.Focus()
			m.refresh()
			return m, nil
		}
		if val != "" {
			if err := m.store.UpdateMeta(m.editRow.ID, m.editRow.Title, val); err != nil {
				m.errMsg = "Save failed: " + err.Error()
			} else {
				m.editRow.Artist = val
				m.errMsg = "Saved — re-importing the file will overwrite this edit"
			}
		}
		rowID := m.editRow.ID
		title, artist := m.editRow.Title, m.editRow.Artist
		m.editing = false
		m.editRow = nil
		m.editInput.Blur()
		m.updateTabRow(rowID, func(r *library.TabRow) { r.Title = title; r.Artist = artist })
		return m, m.apply()
	}
	var cmd tea.Cmd
	m.editInput, cmd = m.editInput.Update(msg)
	m.refresh()
	return m, cmd
}

// exportRow writes the selected tab to "<Title>.txt" in the working
// directory and copies it to the clipboard when a tool is available.
func (m BrowserModel) exportRow(row library.TabRow) (BrowserModel, tea.Cmd) {
	tab, err := m.store.Get(row.ID)
	if err != nil || tab == nil {
		m.errMsg = "Export failed: could not load tab"
		m.refresh()
		return m, nil
	}
	_, msg := export.Tab(tab)
	m.errMsg = msg
	m.refresh()
	return m, nil
}

func (m BrowserModel) handleSearchKey(msg tea.KeyMsg) (BrowserModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if m.searchInput != "" {
			m.searchInput = ""
			return m, m.apply()
		} else {
			m.searchActive = false
			return m, func() tea.Msg { return msgs.GoHomeMsg{} }
		}
	case "o":
		m.searchActive = false
		m.searchInput = ""
		return m, func() tea.Msg { return msgs.HomeSearchMsg{} }
	case "enter":
		if m.cursor >= 0 && m.cursor < len(m.filtered) {
			m.searchActive = false
			m.searchInput = ""
			return m, func() tea.Msg {
				return msgs.TabSelectedMsg{ID: m.filtered[m.cursor].ID}
			}
		}
		// Stay in search mode so the user can keep typing.
		return m, m.apply()
	case "down":
		return m, m.moveCursor(1)
	case "up":
		return m, m.moveCursor(-1)
	case "backspace":
		r := []rune(m.searchInput)
		if len(r) > 0 {
			m.searchInput = string(r[:len(r)-1])
			m.cursor = 0
			return m, m.apply()
		}
	case kit.KeyQuit2:
		return m, tea.Quit
	default:
		if len(msg.String()) == 1 && msg.String()[0] >= 32 && msg.String()[0] < 127 {
			m.searchInput += msg.String()
			m.cursor = 0
			return m, m.apply()
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
					return m, m.apply()
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
	case kit.KeyQuit, kit.KeyQuit2:
		return m, tea.Quit
	case "down", kit.KeyDown:
		return m, m.moveCursor(1)
	case "up", kit.KeyUp:
		return m, m.moveCursor(-1)
	case "enter":
		if m.cursor >= 0 && m.cursor < len(m.filtered) {
			return m, func() tea.Msg {
				return msgs.TabSelectedMsg{ID: m.filtered[m.cursor].ID}
			}
		}
	case "f":
		if m.store != nil && m.cursor >= 0 && m.cursor < len(m.filtered) {
			row := m.filtered[m.cursor]
			newFav := !row.Favorite
			if err := m.store.SetFavorite(row.ID, newFav); err == nil {
				m.errMsg = ""
				m.updateTabRow(row.ID, func(r *library.TabRow) { r.Favorite = newFav })
				return m, m.apply()
			} else if err != nil {
				m.errMsg = "Favorite failed: " + err.Error()
				m.refresh()
			}
		}
	case "F":
		m.favOnly = !m.favOnly
		m.cursor = 0
		return m, m.apply()
	case "e":
		if m.store != nil && m.cursor >= 0 && m.cursor < len(m.filtered) {
			row := m.filtered[m.cursor]
			copy := row
			m.editRow = &copy
			m.editField = 1
			m.editing = true
			m.editInput.SetValue("")
			m.editInput.Placeholder = row.Title
			m.editInput.Focus()
			m.errMsg = ""
			m.refresh()
		}
	case "x":
		if m.store != nil && m.cursor >= 0 && m.cursor < len(m.filtered) {
			return m.exportRow(m.filtered[m.cursor])
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
		return m, m.apply()
	case "/":
		m.searchActive = true
		m.cursor = 0
		m.refresh()
	case "esc", "h":
		if m.searchInput != "" {
			m.searchInput = ""
			m.searchActive = false
			m.cursor = 0
			return m, m.apply()
		}
		if m.searchActive {
			m.searchActive = false
			return m, m.apply()
		}
		return m, func() tea.Msg { return msgs.GoHomeMsg{} }
	}
	return m, nil
}

func (m *BrowserModel) moveCursor(delta int) tea.Cmd {
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if len(m.filtered) > 0 && m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}
	m.ensureCursorVisible()
	m.refresh()
	return m.requestPreview()
}

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

// apply recomputes the filtered/sorted list and reloads the preview for the
// selected row if it changed.
func (m *BrowserModel) apply() tea.Cmd {
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
	return m.requestPreview()
}

func (m BrowserModel) filterAndSort(rows []library.TabRow) []library.TabRow {
	out := make([]library.TabRow, len(rows))
	copy(out, rows)

	if m.favOnly {
		var favs []library.TabRow
		for _, r := range out {
			if r.Favorite {
				favs = append(favs, r)
			}
		}
		out = favs
	}

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

// splitActive reports whether the preview panel is shown beside the list.
func (m BrowserModel) splitActive() bool {
	return m.preview != "" && m.width >= splitMinWidth
}

func (m *BrowserModel) refresh() {
	// The list viewport narrows when the preview panel shares the row.
	if m.splitActive() {
		m.viewport.Width = m.width - previewPanelWidth - 8
	} else {
		m.viewport.Width = m.width - 4
	}
	m.viewport.SetContent(m.renderList())
}

func (m BrowserModel) renderList() string {
	if m.loading {
		return kit.InfoStyle.Render("⠋ Reloading library…")
	}
	if !m.loaded {
		return kit.InfoStyle.Render("Loading library…")
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
	case SortRecent:
		header = fmt.Sprintf("%-3s %-34s %-24s %s", " ", "TITLE", "ARTIST", "TUNING")
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
			star = "★"
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

func (m *BrowserModel) resetFilter() {
	m.searchActive = false
	m.searchInput = ""
	m.errMsg = ""
	m.apply()
}

// ResetFilter clears the search filter and returns to the full list.
func (m *BrowserModel) ResetFilter() { m.resetFilter() }

// SetAutoImportWarn updates the auto-import warning banner.
func (m *BrowserModel) SetAutoImportWarn(msg string) { m.autoImportWarn = msg }

// IsSearchActive reports whether the filter is being typed.
func (m *BrowserModel) IsSearchActive() bool { return m.searchActive }

// SetSearchActive forces the filter on/off (used by the router's tests).
func (m *BrowserModel) SetSearchActive(v bool) { m.searchActive = v }

// FilterValue returns the current filter text.
func (m *BrowserModel) FilterValue() string { return m.searchInput }
