package search

import (
	"fmt"
	"strings"

	"github.com/YOUR_USERNAME/fretboard/internal/scraper"
	"github.com/YOUR_USERNAME/fretboard/internal/ui/kit"
	"github.com/YOUR_USERNAME/fretboard/internal/ui/msgs"
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
		case kit.KeyQuit:
			if !m.inputActive {
				return m, tea.Quit
			}
		case kit.KeyQuit2:
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
			return m, func() tea.Msg { return msgs.SearchBackMsg{} }
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
				return m, func() tea.Msg { return msgs.SearchBackMsg{} }
			}
			if !m.inputActive {
				m.focusQuery()
			}
		}
	case msgs.SearchPerformedMsg:
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
		return msgs.SearchPerformedMsg{Results: res, Err: err, Gen: gen}
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
			return msgs.TabImportErrorMsg{Err: fmt.Errorf("online search is not configured"), Gen: gen}
		}
		tab, err := m.client.Fetch(result)
		if err != nil {
			return msgs.TabImportErrorMsg{Err: err, Gen: gen}
		}
		return msgs.TabFetchedMsg{Tab: tab, Source: result, Gen: gen}
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
			return kit.InfoStyle.Render("⠋ Fetching tab…")
		}
		return kit.InfoStyle.Render("⠋ Searching…")
	}
	if m.errMsg != "" {
		return kit.ErrorStyle.Render(m.errMsg)
	}
	if len(m.results) == 0 {
		hint := "Type a query and press Enter to search."
		if m.inputActive {
			hint += "  Tab/j/k move through results after a search."
		}
		return kit.MutedStyle.Render(hint)
	}
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
		b.WriteString(kit.MutedStyle.Render("/ or i — edit query   Enter — open tab"))
	}
	return b.String()
}

func formatResult(r scraper.SearchResult) string {
	badge := kit.InfoStyle.Render("[UG]")
	if r.Source == scraper.SourceSongsterr {
		badge = kit.SuccessStyle.Render("[ST]")
	}
	meta := kit.MutedStyle.Render(r.Type)
	return badge + " " + r.SongName + " — " + r.ArtistName + "  " + meta
}

// View renders the search screen.
func (m SearchModel) View() string {
	queryTitle := "Query"
	if m.inputActive {
		queryTitle += kit.SuccessStyle.Render("  ●")
	}
	searchBox := kit.RenderPanel(m.width-2, queryTitle, m.input.View())
	resultsTitle := "Results"
	if !m.inputActive && len(m.results) > 0 {
		resultsTitle += kit.SuccessStyle.Render("  ●")
	}
	results := kit.RenderPanel(m.width-2, resultsTitle, m.viewport.View())
	body := "\n" + searchBox + "\n" + results
	footer := kit.RenderFooter(m.width, []kit.KeyHint{
		{Key: "Enter", Label: "search/open"},
		{Key: "Tab", Label: "results"},
		{Key: "/", Label: "query"},
		{Key: "j/k", Label: "move"},
		{Key: "Esc", Label: "back"},
		{Key: "q", Label: "quit"},
	})
	return kit.LayoutScreen(m.width, m.height, kit.FormatBreadcrumb("home", "search"), body, footer)
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
