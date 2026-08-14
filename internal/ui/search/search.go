package search

import (
	"fmt"
	"strconv"
	"strings"

	"fretboard/internal/model"
	"fretboard/internal/scraper"
	"fretboard/internal/ui/kit"
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
				m.lastQuery = q
				m.cacheNote = ""
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
		case "m":
			// Load more results from the next page, merged with the current
			// list (only meaningful in results mode).
			if !m.inputActive && m.lastQuery != "" && m.client != nil && !m.loading {
				m.loading = true
				m.errMsg = ""
				m.reqGen++
				gen := m.reqGen
				return m, m.searchMoreCmd(m.lastQuery, gen)
			}
		case "tab", "j":
			if m.inputActive && len(m.results) > 0 {
				m.focusResults()
				return m, nil
			}
			if !m.inputActive {
				m.moveCursor(1)
			}
		case "up":
			if m.inputActive {
				// Recall the query history: from an empty box, or continuing
				// from the query the previous up filled in.
				cur := m.input.Value()
				if m.histIdx < len(m.history) && (cur == "" || cur == m.history[m.histIdx-1]) {
					m.histIdx++
					m.input.SetValue(m.history[m.histIdx-1])
					m.refresh()
					return m, nil
				}
				if len(m.results) > 0 {
					m.focusResults()
					if m.cursor > 0 {
						m.moveCursor(-1)
					}
					return m, nil
				}
				return m, nil
			}
			if m.cursor == 0 {
				m.focusQuery()
			} else {
				m.moveCursor(-1)
			}
		case "down":
			if m.inputActive {
				if m.histIdx > 0 {
					m.histIdx--
					if m.histIdx == 0 {
						m.input.SetValue("")
					} else {
						m.input.SetValue(m.history[m.histIdx-1])
					}
					m.refresh()
					return m, nil
				}
				if len(m.results) > 0 {
					m.focusResults()
					return m, nil
				}
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

func (m *SearchModel) searchCmd(query string, gen int) tea.Cmd {
	return func() tea.Msg {
		res, err := m.client.Search(query)
		return msgs.SearchPerformedMsg{Results: res, Err: err, Gen: gen}
	}
}

// searchMoreCmd fetches the next page of results for the current query.
func (m *SearchModel) searchMoreCmd(query string, gen int) tea.Cmd {
	return func() tea.Msg {
		res, err := m.client.SearchPage(query, 2)
		return msgs.SearchPerformedMsg{Results: res, Err: err, Gen: gen, More: true}
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
		tab, err := m.client.Fetch(result)
		if err != nil {
			return msgs.TabImportErrorMsg{Err: err, Gen: gen}
		}
		// Carry provenance into the library: where the tab came from and how
		// well it is rated, so the browser and viewer can show it later.
		if tab.Metadata == nil {
			tab.Metadata = map[string]string{}
		}
		tab.Metadata[model.MetaKeySourceBadge] = scraper.SourceBadge(result)
		if result.Rating > 0 {
			tab.Metadata["source_rating"] = fmt.Sprintf("%.1f", result.Rating)
		}
		if result.Votes > 0 {
			tab.Metadata["source_votes"] = strconv.FormatInt(result.Votes, 10)
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
