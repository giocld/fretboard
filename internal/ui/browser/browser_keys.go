package browser

import (
	"strings"

	"fretboard/internal/export"
	"fretboard/internal/library"
	"fretboard/internal/ui/kit"
	"fretboard/internal/ui/msgs"
	tea "github.com/charmbracelet/bubbletea"
)

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
		}
		m.searchActive = false
		return m, func() tea.Msg { return msgs.GoHomeMsg{} }
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
			} else {
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
