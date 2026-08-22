package search

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"fretboard/internal/model"
	"fretboard/internal/scraper"
	"fretboard/internal/ui/kit"
	"fretboard/internal/ui/msgs"
	tea "github.com/charmbracelet/bubbletea"
)

// handleKey processes a key press from Update: history recall with up/down,
// tab/j/k result navigation, enter to search or import, esc to clear or go
// back, m to load more, and / or i to refocus the query. Keys the switch does
// not consume fall through to the input (when typing) or, outside the query
// box, to the viewport scroll keys (pgdown/ctrl+d/pgup/ctrl+u) — the same
// tail Update used to run.
func (m SearchModel) handleKey(msg tea.KeyMsg) (SearchModel, tea.Cmd) {
	var cmd tea.Cmd
	switch msg.String() {
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
			m.cacheSavedAt = time.Time{}
			m.cacheStale = false
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

	if m.inputActive {
		m.input, cmd = m.input.Update(msg)
	} else {
		switch msg.String() {
		case "pgdown", "ctrl+d", "pgup", "ctrl+u":
			m.viewport, cmd = m.viewport.Update(msg)
		}
	}
	return m, cmd
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

func (m *SearchModel) importCmd(result scraper.SearchResult, gen int) tea.Cmd {
	query := m.lastQuery
	return func() tea.Msg {
		// FetchBest falls back to a community copy when the selected result
		// is UG Pro/official-only and its direct fetch hits the paywall;
		// the reason is carried on the result for the viewer to surface.
		tab, reason, err := m.client.FetchBest(result, query)
		if err != nil {
			return msgs.TabImportErrorMsg{Err: err, Gen: gen}
		}
		if reason != "" {
			result.PickReason = reason
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
		// Provenance for the viewer header/badges: pro/reconstructed flags,
		// the source page URL (g binds it), and the pick fallback reason.
		if result.Pro {
			tab.Metadata["pro"] = "1"
		}
		if result.Reconstructed {
			tab.Metadata["reconstructed"] = "1"
		}
		if result.SourceURL != "" {
			tab.Metadata["source_url"] = result.SourceURL
		}
		if result.PickReason != "" {
			tab.Metadata["pick_reason"] = result.PickReason
		}
		return msgs.TabFetchedMsg{Tab: tab, Source: result, Gen: gen}
	}
}
