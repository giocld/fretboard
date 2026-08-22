package home

import (
	"sort"

	"fretboard/internal/library"
	"fretboard/internal/ui/kit"
	"fretboard/internal/ui/msgs"
	tea "github.com/charmbracelet/bubbletea"
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
	missingDeps    []string // critical missing playback deps (diag probe)
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
	m.cursor = min(m.cursor, m.maxCursor())
}

func (m *HomeModel) moveCursor(delta int) {
	limit := m.maxCursor()
	m.cursor += delta
	m.cursor = min(max(m.cursor, 0), limit)
	if m.loaded {
		m.preview = m.loadPreview()
	}
}

func (m HomeModel) maxCursor() int {
	recent := m.recentTabs()
	if len(recent) == 0 {
		return homeActionCount - 1
	}
	return homeActionCount + len(recent) - 1
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
		}
	}
	recent := m.recentTabs()
	idx := m.cursor - homeActionCount
	if idx < len(recent) {
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
	return sorted[:min(3, len(sorted))]
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
	if idx >= len(recent) {
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

// SetAutoImportWarn updates the auto-import warning banner shown on home.
func (m *HomeModel) SetAutoImportWarn(msg string) {
	m.autoImportWarn = msg
}

// SetMissingDeps records critical playback dependencies that the startup
// diag probe found missing (e.g. "fluidsynth/timidity", "mpv/ffplay"); the
// home banner and footer marker surface them (8.2).
func (m *HomeModel) SetMissingDeps(missing []string) {
	m.missingDeps = append([]string(nil), missing...)
}
