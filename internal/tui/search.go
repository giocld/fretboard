package tui

import (
	"fmt"
	"strings"

	"github.com/YOUR_USERNAME/fretboard/internal/model"
	"github.com/YOUR_USERNAME/fretboard/internal/scraper"
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
}

// NewSearchModel creates an online search view.
func NewSearchModel(client *scraper.Client) SearchModel {
	ti := textinput.New()
	ti.Placeholder = "Search Ultimate Guitar + Songsterr…"
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
	}
}

// Reset clears search state when (re)entering the screen.
func (m *SearchModel) Reset() {
	m.results = nil
	m.cursor = 0
	m.errMsg = ""
	m.loading = false
	m.importing = false
	m.reqGen++
	m.input.SetValue("")
	m.inputActive = true
	m.input.Focus()
	m.refresh()
}

// Init focuses the search input.
func (m SearchModel) Init() tea.Cmd {
	return textinput.Blink
}

// SearchPerformedMsg is sent when a search completes.
type SearchPerformedMsg struct {
	Results []scraper.SearchResult
	Err     error
	Gen     int
}

// TabFetchedMsg is sent when an online tab has been fetched and parsed.
type TabFetchedMsg struct {
	Tab    *model.Tab
	Source scraper.SearchResult
	Gen    int
}

// SearchBackMsg is sent when the user leaves online search.
type SearchBackMsg struct{}

// TabImportErrorMsg is sent when fetching an online tab fails.
type TabImportErrorMsg struct {
	Err error
	Gen int
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
	case tea.KeyMsg:
		key := msg.String()
		switch key {
		case KeyQuit:
			if !m.inputActive {
				return m, tea.Quit
			}
		case KeyQuit2:
			return m, tea.Quit
		case "esc":
			if m.loading {
				m.loading = false
				m.importing = false
				m.reqGen++
				m.errMsg = ""
				m.focusQuery()
				m.refresh()
				return m, nil
			}
			if m.inputActive && m.input.Value() != "" {
				m.input.SetValue("")
				m.errMsg = ""
				m.refresh()
				return m, nil
			}
			if !m.inputActive {
				m.focusQuery()
				return m, nil
			}
			if len(m.results) > 0 || m.errMsg != "" {
				m.results = nil
				m.errMsg = ""
				m.cursor = 0
				m.focusQuery()
				m.refresh()
				return m, nil
			}
			return m, func() tea.Msg { return SearchBackMsg{} }
		case "enter":
			if m.inputActive {
				if m.client == nil {
					m.errMsg = "online search is not configured"
					m.refresh()
					return m, nil
				}
				q := strings.TrimSpace(m.input.Value())
				if q == "" {
					m.errMsg = "Type a song or artist, then press Enter"
					m.focusQuery()
					m.refresh()
					return m, nil
				}
				m.loading = true
				m.importing = false
				m.errMsg = ""
				m.reqGen++
				gen := m.reqGen
				return m, m.searchCmd(q, gen)
			}
			if m.cursor >= 0 && m.cursor < len(m.results) {
				m.loading = true
				m.importing = true
				m.errMsg = ""
				m.reqGen++
				gen := m.reqGen
				return m, m.importCmd(m.results[m.cursor], gen)
			}
		case "tab", "down":
			if m.inputActive && len(m.results) > 0 {
				m.focusResults()
				return m, nil
			}
			if !m.inputActive {
				m.moveCursor(1)
			}
		case "up":
			if m.inputActive && len(m.results) > 0 {
				m.focusResults()
				if m.cursor > 0 {
					m.moveCursor(-1)
				}
				return m, nil
			}
			if !m.inputActive {
				if m.cursor == 0 {
					m.focusQuery()
				} else {
					m.moveCursor(-1)
				}
			}
		case "j":
			if m.inputActive && len(m.results) > 0 {
				m.focusResults()
				return m, nil
			}
			if !m.inputActive {
				m.moveCursor(1)
			}
		case "k":
			if m.inputActive && len(m.results) > 0 {
				m.focusResults()
				if m.cursor > 0 {
					m.moveCursor(-1)
				}
				return m, nil
			}
			if !m.inputActive {
				m.moveCursor(-1)
			}
		case "/", "i":
			m.focusQuery()
		case "h":
			if !m.inputActive && m.input.Value() == "" && len(m.results) == 0 && m.errMsg == "" {
				return m, func() tea.Msg { return SearchBackMsg{} }
			}
			if !m.inputActive {
				m.focusQuery()
			}
		}
	case SearchPerformedMsg:
		if msg.Gen != m.reqGen {
			return m, nil
		}
		m.loading = false
		m.importing = false
		if msg.Err != nil {
			m.errMsg = msg.Err.Error()
			m.focusQuery()
		} else {
			m.results = msg.Results
			m.cursor = 0
			if len(m.results) == 0 {
				m.errMsg = "No results — try a different query"
				m.focusQuery()
			} else {
				m.focusResults()
				m.viewport.SetYOffset(0)
			}
		}
		m.refresh()
	case TabImportErrorMsg:
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
	case TabFetchedMsg:
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
		case "pgdown", "ctrl+d":
			m.viewport, cmd = m.viewport.Update(msg)
		case "pgup", "ctrl+u":
			m.viewport, cmd = m.viewport.Update(msg)
		}
	}
	return m, cmd
}

func (m *SearchModel) focusQuery() {
	m.inputActive = true
	m.input.Focus()
}

func (m *SearchModel) focusResults() {
	m.inputActive = false
	m.input.Blur()
}

func (m *SearchModel) searchCmd(query string, gen int) tea.Cmd {
	return func() tea.Msg {
		res, err := m.client.Search(query)
		return SearchPerformedMsg{Results: res, Err: err, Gen: gen}
	}
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

func (m *SearchModel) importCmd(result scraper.SearchResult, gen int) tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return TabImportErrorMsg{Err: fmt.Errorf("online search is not configured"), Gen: gen}
		}
		tab, err := m.client.Fetch(result)
		if err != nil {
			return TabImportErrorMsg{Err: err, Gen: gen}
		}
		return TabFetchedMsg{Tab: tab, Source: result, Gen: gen}
	}
}

// OnlineTabPath returns a stable library key for an online search result.
func OnlineTabPath(r scraper.SearchResult) string {
	return fmt.Sprintf("online://%s/%d", r.Source, r.ID)
}

func (m *SearchModel) refresh() {
	m.viewport.SetContent(m.renderResults())
}

func (m SearchModel) renderResults() string {
	if m.loading {
		if m.importing {
			return infoStyle.Render("⠋ Fetching tab…")
		}
		return infoStyle.Render("⠋ Searching…")
	}
	if m.errMsg != "" {
		return errorStyle.Render(m.errMsg)
	}
	if len(m.results) == 0 {
		hint := "Type a query and press Enter to search."
		if m.inputActive {
			hint += "  Tab/j/k move through results after a search."
		}
		return mutedStyle.Render(hint)
	}
	var b strings.Builder
	for i, r := range m.results {
		line := formatResult(r)
		if i == m.cursor {
			b.WriteString(listSelected.Render("▸ " + line))
		} else {
			b.WriteString(listNormal.Render("  " + line))
		}
		b.WriteString("\n")
	}
	if !m.inputActive {
		b.WriteString("\n")
		b.WriteString(mutedStyle.Render("/ or i — edit query   Enter — open tab"))
	}
	return b.String()
}

func formatResult(r scraper.SearchResult) string {
	badge := infoStyle.Render("[UG]")
	if r.Source == scraper.SourceSongsterr {
		badge = successStyle.Render("[ST]")
	}
	meta := mutedStyle.Render(r.Type)
	return badge + " " + r.SongName + " — " + r.ArtistName + "  " + meta
}

// View renders the search screen.
func (m SearchModel) View() string {
	queryTitle := "Query"
	if m.inputActive {
		queryTitle += successStyle.Render("  ●")
	}
	searchBox := RenderPanel(m.width-2, queryTitle, m.input.View())
	resultsTitle := "Results"
	if !m.inputActive && len(m.results) > 0 {
		resultsTitle += successStyle.Render("  ●")
	}
	results := RenderPanel(m.width-2, resultsTitle, m.viewport.View())
	body := "\n" + searchBox + "\n" + results
	footer := RenderFooter(m.width, []KeyHint{
		{Key: "Enter", Label: "search/open"},
		{Key: "Tab", Label: "results"},
		{Key: "/", Label: "query"},
		{Key: "j/k", Label: "move"},
		{Key: "Esc", Label: "back"},
		{Key: "q", Label: "quit"},
	})
	return LayoutScreen(m.width, m.height, FormatBreadcrumb("home", "search"), body, footer)
}
