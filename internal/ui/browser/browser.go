package browser

import (
	"sort"
	"strings"

	"fretboard/internal/library"
	"fretboard/internal/ui/msgs"
	"github.com/charmbracelet/bubbles/textinput"
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
	} else if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	} else if m.cursor < 0 {
		m.cursor = 0
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
		filtered := make([]library.TabRow, 0, len(matches))
		for _, match := range matches {
			filtered = append(filtered, rows[match.Index])
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
	// The list viewport narrows when the preview panel shares the row.
	if m.splitActive() {
		m.viewport.Width = m.width - previewPanelWidth - 8
	} else {
		m.viewport.Width = m.width - 4
	}
	m.viewport.SetContent(m.renderList())
}

// ResetFilter clears the search filter and returns to the full list.
func (m *BrowserModel) ResetFilter() {
	m.searchActive = false
	m.searchInput = ""
	m.errMsg = ""
	m.apply()
}

// SetAutoImportWarn updates the auto-import warning banner.
func (m *BrowserModel) SetAutoImportWarn(msg string) { m.autoImportWarn = msg }

// IsSearchActive reports whether the filter is being typed.
func (m *BrowserModel) IsSearchActive() bool { return m.searchActive }

// SetSearchActive forces the filter on/off (used by the router's tests).
func (m *BrowserModel) SetSearchActive(v bool) { m.searchActive = v }

// FilterValue returns the current filter text.
func (m *BrowserModel) FilterValue() string { return m.searchInput }
