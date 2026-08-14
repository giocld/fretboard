package viewer

import (
	"strconv"
	"strings"

	"fretboard/internal/ui/kit"
	tea "github.com/charmbracelet/bubbletea"
)

// searchMatch is one in-tab search hit: a bar index and the column where
// the pattern starts.
type searchMatch struct{ bar, col int }

// computeSearchMatches re-runs the in-tab search for the current query. A
// plain bar number jumps straight to that bar; otherwise the query is
// matched against each string's fret-digit sequence.
func (m *ViewerModel) computeSearchMatches() {
	m.searchMatches = nil
	m.searchIdx = 0
	q := strings.TrimSpace(m.searchInput)
	if q == "" || m.tab == nil {
		return
	}
	if n, err := strconv.Atoi(q); err == nil && n >= 1 && n <= len(m.tab.Bars) {
		m.searchMatches = []searchMatch{{bar: n - 1, col: 0}}
		return
	}
	for bi, bar := range m.tab.Bars {
		// Section names are searchable: "verse" jumps to the verse.
		if sec := strings.TrimSpace(bar.Section); sec != "" && strings.Contains(strings.ToLower(sec), q) {
			m.searchMatches = append(m.searchMatches, searchMatch{bar: bi, col: 0})
			continue
		}
		for _, sl := range bar.Strings {
			var digits strings.Builder
			var cols []int
			for _, seg := range sl.Segments {
				if seg.Char >= '0' && seg.Char <= '9' {
					digits.WriteRune(seg.Char)
					cols = append(cols, seg.Position)
				}
			}
			seq := digits.String()
			if idx := strings.Index(seq, q); idx >= 0 && idx < len(cols) {
				m.searchMatches = append(m.searchMatches, searchMatch{bar: bi, col: cols[idx]})
				break
			}
		}
	}
}

// jumpToMatch moves the cursor to the current search match and wraps.
func (m *ViewerModel) jumpToMatch(delta int) {
	if len(m.searchMatches) == 0 || m.tab == nil {
		return
	}
	m.searchIdx = (m.searchIdx + delta + len(m.searchMatches)) % len(m.searchMatches)
	match := m.searchMatches[m.searchIdx]
	m.cursorBar = match.bar
	m.cursorCol = match.col
	m.follow = false
	m.ensureCursorVisible()
	m.refresh()
}

// handleKey routes keys to the audio picker, the search box, or the practice
// and navigation key handlers.
func (m ViewerModel) handleKey(msg tea.KeyMsg) (ViewerModel, tea.Cmd) {
	if m.showAudioPicker {
		return m.handleAudioPickerKey(msg)
	}
	if m.searchActive {
		return m.handleSearchKey(msg)
	}
	if s := msg.String(); len(s) == 1 && s[0] >= '0' && s[0] <= '9' {
		m.jumpBuffer += s
		return m, nil
	}
	switch msg.String() {
	case kit.KeyQuit, kit.KeyQuit2, "b", "H", "a", " ", "p", "+", "=", "-", "_", "P", "y", "m", "C", "s", "S", "i", "u", "x", ">", "<", "r", "w", "W", "f9", "[", "{", "]", "}", ",", ".", "o", "esc":
		return m.handleKeyPractice(msg)
	default:
		return m.handleKeyNav(msg)
	}
}

// clampTranspose keeps the session transpose within a playable range.
func clampTranspose(n int) int {
	if n > 12 {
		return 12
	}
	if n < -12 {
		return -12
	}
	return n
}

// handleSearchKey drives the in-tab search box (`/`): printable keys edit
// the query and recompute matches, Enter jumps to the first match, n/N cycle
// while typing too, Esc closes.
func (m ViewerModel) handleSearchKey(msg tea.KeyMsg) (ViewerModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.searchActive = false
		m.searchInput = ""
		m.searchMatches = nil
		m.searchIdx = 0
		m.refresh()
		return m, nil
	case "enter":
		if len(m.searchMatches) > 0 {
			m.jumpToMatch(0)
		}
		m.searchActive = false
		m.refresh()
		return m, nil
	case "n":
		m.jumpToMatch(1)
		return m, nil
	case "N":
		m.jumpToMatch(-1)
		return m, nil
	case "backspace":
		r := []rune(m.searchInput)
		if len(r) > 0 {
			m.searchInput = string(r[:len(r)-1])
			m.computeSearchMatches()
			m.refresh()
		}
		return m, nil
	default:
		// Printable characters only (single keys and pastes); arrow/ctrl
		// sequences must never land in the query.
		s := msg.String()
		printable := s != ""
		for _, r := range s {
			if r < 32 || r >= 127 {
				printable = false
				break
			}
		}
		if printable {
			m.searchInput += s
			m.computeSearchMatches()
			m.refresh()
		}
	}
	return m, nil
}

// keyFromMouse builds a key message from a mouse-driven action name.
func keyFromMouse(k string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}
