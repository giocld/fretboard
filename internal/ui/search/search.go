package search

import (
	"fmt"

	"fretboard/internal/scraper"
	"fretboard/internal/ui/msgs"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

// SearchModel is the online tab search view.
type SearchModel struct {
	input       textinput.Model
	results     []scraper.SearchResult
	cursor      int
	viewport    viewport.Model
	client      *scraper.Client
	loading     bool
	importing   bool
	errMsg      string
	width       int
	height      int
	inputActive bool // true = typing query; false = navigating results
	reqGen      int
	history     []string // persisted query history, newest first
	histIdx     int      // position while recalling history with up/down
	cacheNote   string   // "offline cache" note when results came from cache
	lastQuery   string   // the query of the current result set
}

// NewSearchModel creates an online search view.
func NewSearchModel(client *scraper.Client) SearchModel {
	ti := textinput.New()
	ti.Placeholder = "Search Ultimate Guitar, Songsterr, GuitarTabs.cc..."
	ti.Prompt = "› "
	ti.CharLimit = 120
	ti.Focus()
	return SearchModel{
		input:       ti,
		viewport:    viewport.New(80, 20),
		client:      client,
		width:       80,
		height:      24,
		inputActive: true,
		history:     loadHistory(),
	}
}

// Reset clears search state when (re)entering the screen.
func (m *SearchModel) Reset() {
	m.results = nil
	m.cursor = 0
	m.errMsg = ""
	m.cacheNote = ""
	m.loading = false
	m.importing = false
	m.reqGen++
	m.input.SetValue("")
	m.inputActive = true
	m.input.Focus()
	m.history = loadHistory()
	m.histIdx = 0
	m.refresh()
}

// Init focuses the search input.
func (m SearchModel) Init() tea.Cmd {
	return textinput.Blink
}

// Update handles input, search, and result selection.
func (m SearchModel) Update(msg tea.Msg) (SearchModel, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = msg.Width - 4
		bodyH := msg.Height - 7
		if bodyH < 3 {
			bodyH = 3
		}
		m.viewport.Height = bodyH
	case tea.MouseMsg:
		// Wheel scrolls the result list (results mode only).
		if !m.inputActive {
			switch msg.Type {
			case tea.MouseWheelUp:
				m.moveCursor(-1)
			case tea.MouseWheelDown:
				m.moveCursor(1)
			}
		}
	case tea.KeyMsg:
		return m.handleKey(msg)
	case msgs.SearchPerformedMsg:
		if msg.Gen != m.reqGen {
			return m, nil
		}
		m.loading = false
		m.importing = false
		if msg.More {
			// Load-more pass: merge the next page into the current list.
			if msg.Err != nil {
				m.errMsg = "Could not load more: " + msg.Err.Error()
			} else {
				prev := len(m.results)
				merged := scraper.MergeResults(m.results, msg.Results)
				m.results = merged
				if n := len(merged) - prev; n > 0 {
					m.errMsg = fmt.Sprintf("Loaded %d more results", n)
				} else {
					m.errMsg = "No more results"
				}
			}
			m.refresh()
			return m, nil
		}
		if msg.Err != nil {
			m.errMsg = msg.Err.Error()
			// Offline fallback: serve the cached result set for this query
			// instead of an empty dead end.
			if cached, ok := loadCache(m.lastQuery); ok {
				m.results = cached
				m.cursor = 0
				m.cacheNote = "offline cache — " + msg.Err.Error()
				m.errMsg = ""
				m.focusResults()
				m.viewport.SetYOffset(0)
			} else {
				m.focusQuery()
			}
		} else {
			m.results = msg.Results
			m.cursor = 0
			m.cacheNote = ""
			m.history = addHistory(m.history, m.lastQuery)
			saveHistory(m.history)
			saveCache(m.lastQuery, msg.Results)
			if len(m.results) == 0 {
				m.errMsg = "No results — try a different query"
				m.focusQuery()
			} else {
				m.focusResults()
				m.viewport.SetYOffset(0)
			}
		}
		m.refresh()
	case msgs.TabImportErrorMsg:
		if msg.Gen != m.reqGen {
			return m, nil
		}
		m.loading = false
		m.importing = false
		if msg.Err != nil {
			m.errMsg = "Could not fetch tab: " + msg.Err.Error()
		} else {
			m.errMsg = "Could not fetch tab"
		}
		m.focusResults()
		m.refresh()
	case msgs.TabFetchedMsg:
		if msg.Gen != m.reqGen {
			return m, nil
		}
		m.loading = false
		m.importing = false
	}

	if m.inputActive {
		m.input, cmd = m.input.Update(msg)
	} else if key, ok := msg.(tea.KeyMsg); ok {
		// Scroll results when not editing the query.
		switch key.String() {
		case "pgdown", "ctrl+d", "pgup", "ctrl+u":
			m.viewport, cmd = m.viewport.Update(msg)
		}
	}
	return m, cmd
}

func (m *SearchModel) focusQuery() {
	m.inputActive = true
	m.input.Focus()
	m.refresh()
}

func (m *SearchModel) focusResults() {
	m.inputActive = false
	m.input.Blur()
	m.refresh()
}

func (m *SearchModel) moveCursor(delta int) {
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if len(m.results) > 0 && m.cursor >= len(m.results) {
		m.cursor = len(m.results) - 1
	}
	m.ensureCursorVisible()
	m.refresh()
}

func (m *SearchModel) ensureCursorVisible() {
	if len(m.results) == 0 {
		return
	}
	target := m.cursor
	if target < m.viewport.YOffset {
		m.viewport.SetYOffset(target)
	} else if target >= m.viewport.YOffset+m.viewport.Height {
		m.viewport.SetYOffset(target - m.viewport.Height + 1)
	}
}

// OnlineTabPath returns a stable library key for an online search result.
func OnlineTabPath(r scraper.SearchResult) string {
	return fmt.Sprintf("online://%s/%d", r.Source, r.ID)
}

func (m *SearchModel) refresh() {
	m.viewport.SetContent(m.renderResults())
}

// HasClient reports whether an online search client is configured.
func (m *SearchModel) HasClient() bool { return m.client != nil }

// IsInputActive reports whether the query box has focus.
func (m *SearchModel) IsInputActive() bool { return m.inputActive }

// QueryValue returns the current query text.
func (m *SearchModel) QueryValue() string { return m.input.Value() }

// AcceptsGen reports whether msg generation matches the current request.
func (m *SearchModel) AcceptsGen(gen int) bool { return gen == m.reqGen }

// SetBusy updates the loading/importing flags after a search or import completes.
func (m *SearchModel) SetBusy(loading, importing bool) {
	m.loading = loading
	m.importing = importing
}
