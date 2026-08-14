package viewer

import (
	"strconv"
	"time"

	"fretboard/internal/export"
	tea "github.com/charmbracelet/bubbletea"
)

// handleKeyNav applies the navigation, search, transpose and display keys.
func (m ViewerModel) handleKeyNav(msg tea.KeyMsg) (ViewerModel, tea.Cmd) {
	switch msg.String() {
	case "/":
		m.jumpBuffer = ""
		m.searchActive = true
		m.searchInput = ""
		m.searchMatches = nil
		m.searchIdx = 0
		m.refresh()
	case "n":
		m.jumpToMatch(1)
	case "N":
		m.jumpToMatch(-1)
	case "T":
		m.transpose = clampTranspose(m.transpose + 1)
		m.jumpBuffer = ""
		m.refresh()
	case "Z":
		m.transpose = clampTranspose(m.transpose - 1)
		m.jumpBuffer = ""
		m.refresh()
	case "R":
		m.transpose = 0
		m.jumpBuffer = ""
		m.refresh()
	case "e":
		m.showNotes = !m.showNotes
		m.jumpBuffer = ""
		m.refresh()
	case "g":
		m.stopPlaybackForNav()
		m.follow = false
		if m.lastKey == "g" && time.Since(m.lastKeyAt) < 500*time.Millisecond {
			m.cursorBar = 0
			m.cursorCol = 0
			m.panOffset = 0
			m.ensureCursorVisible()
			m.lastKey = ""
			m.refresh()
		} else {
			m.lastKey = "g"
			m.lastKeyAt = time.Now()
		}
		m.jumpBuffer = ""
	case "G":
		m.stopPlaybackForNav()
		m.follow = false
		if m.tab != nil && len(m.tab.Bars) > 0 {
			m.cursorBar = len(m.tab.Bars) - 1
			m.cursorCol = 0
		}
		m.panOffset = 0
		m.ensureCursorVisible()
		m.jumpBuffer = ""
		m.refresh()
	case "enter":
		if m.jumpBuffer != "" && m.tab != nil {
			m.stopPlaybackForNav()
			if n, err := strconv.Atoi(m.jumpBuffer); err == nil && n > 0 && n <= len(m.tab.Bars) {
				m.follow = false
				m.cursorBar = n - 1
				m.cursorCol = 0
				m.panOffset = 0
				m.ensureCursorVisible()
				m.refresh()
			}
		}
		m.jumpBuffer = ""
	case "h":
		if m.panOffset > 0 {
			m.panOffset--
			m.refresh()
		}
	case "v":
		m.linear = !m.linear
		m.ensureCursorVisible()
		m.refresh()
	case "f":
		m.follow = !m.follow
		m.refresh()
	case "l":
		if m.panOffset < m.maxPanOffset() {
			m.panOffset++
			m.refresh()
		}
	case "X":
		m.jumpBuffer = ""
		if m.tab != nil {
			_, msg := export.Tab(m.tab)
			m.infoMsg = msg
			m.refresh()
		}
	case "j", "down":
		m.jumpBuffer = ""
		m.stopPlaybackForNav()
		m.follow = false
		if m.tab != nil && m.cursorBar < len(m.tab.Bars)-1 {
			m.cursorBar++
			m.ensureCursorVisible()
			m.refresh()
			return m, nil
		}
	case "k", "up":
		m.jumpBuffer = ""
		m.stopPlaybackForNav()
		m.follow = false
		if m.tab != nil && m.cursorBar > 0 {
			m.cursorBar--
			m.ensureCursorVisible()
			m.refresh()
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}
