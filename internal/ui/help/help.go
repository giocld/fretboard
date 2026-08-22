// Package help implements the keyboard-reference screen. The reference is
// the full keymap filtered to the screen that was active when help opened
// (the router calls SetSection), and "/" searches the shown lines by
// substring.
package help

import (
	"strings"

	"fretboard/internal/ui/kit"
	"fretboard/internal/ui/msgs"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

// Section names a screen whose keymap block the help screen shows. The
// router maps its current view to one of these when opening help.
type Section string

const (
	SectionHome     Section = "home"
	SectionLibrary  Section = "library"
	SectionViewer   Section = "viewer"
	SectionSearch   Section = "search"
	SectionSettings Section = "settings"
)

// HelpModel shows a scrollable, searchable keybinding help screen.
type HelpModel struct {
	viewport  viewport.Model
	width     int
	height    int
	section   Section // the active screen's keymap block
	filter    string  // "/" search: keep lines containing this substring
	searching bool    // the next typed characters extend the filter
}

// NewHelpModel creates a help screen.
func NewHelpModel() HelpModel {
	vp := viewport.New(80, 20)
	m := HelpModel{viewport: vp, width: 80, height: 24, section: SectionHome}
	m.refresh()
	return m
}

// Init is part of the tea.Model interface.
func (m HelpModel) Init() tea.Cmd {
	return nil
}

// SetSection filters the keymap to the given screen and clears any active
// search filter.
func (m *HelpModel) SetSection(s Section) {
	m.section = s
	m.filter = ""
	m.searching = false
	m.refresh()
}

// FilterValue returns the active search filter (for tests).
func (m HelpModel) FilterValue() string { return m.filter }

// Searching reports whether filter input is active (for tests).
func (m HelpModel) Searching() bool { return m.searching }

// Update is part of the tea.Model interface.
func (m HelpModel) Update(msg tea.Msg) (HelpModel, tea.Cmd) {
	switch msg := msg.(type) {
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
	case tea.KeyMsg:
		switch msg.String() {
		case kit.KeyQuit, kit.KeyQuit2, "?", "esc":
			if m.searching {
				// A second Esc (or ?/q) while typing first backs out of the
				// filter instead of closing the screen.
				m.searching = false
				m.filter = ""
				m.refresh()
				return m, nil
			}
			return m, func() tea.Msg { return msgs.CloseHelpMsg{} }
		case "/":
			m.searching = true
			return m, nil
		case "enter":
			// Enter keeps the current filter and returns to scrolling.
			if m.searching {
				m.searching = false
				return m, nil
			}
		case "backspace":
			if m.searching && len(m.filter) > 0 {
				m.filter = m.filter[:len(m.filter)-1]
				m.refresh()
				return m, nil
			}
		default:
			if m.searching && len(msg.String()) == 1 && msg.String()[0] >= 32 && msg.String()[0] < 127 {
				m.filter += msg.String()
				m.refresh()
				return m, nil
			}
		}
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// refresh repopulates the viewport with the section-filtered keymap.
func (m *HelpModel) refresh() {
	m.viewport.SetContent(m.content())
}

// content renders the keymap for the active section (plus the global keys),
// filtered line-by-line by the active "/" search.
func (m HelpModel) content() string {
	body, ok := sectionBlocks[m.section]
	if !ok {
		body = sectionBlocks[SectionHome]
	}
	text := strings.TrimSpace(body + "\n\n" + globalBlock)
	return filterLines(text, m.filter)
}

// filterLines keeps every line containing the (case-insensitive) filter
// substring; blank lines survive so blocks keep their shape.
func filterLines(text, filter string) string {
	if filter == "" {
		return text
	}
	f := strings.ToLower(filter)
	var keep []string
	for _, line := range strings.Split(text, "\n") {
		if line == "" || strings.Contains(strings.ToLower(line), f) {
			keep = append(keep, line)
		}
	}
	return strings.Join(keep, "\n")
}

// View is part of the tea.Model interface.
func (m HelpModel) View() string {
	title := "Keyboard reference"
	status := string(m.section)
	if m.filter != "" {
		title += " · filtered"
		status += " · filter: " + m.filter
	}
	if m.searching {
		status += " · typing…"
	}
	panel := kit.RenderPanel(m.width-2, title, m.viewport.View())
	body := "\n" + panel
	footer := kit.RenderFooterWithStatus(m.width, status, []kit.KeyHint{
		{Key: "Esc", Label: "close"},
		{Key: "j/k", Label: "scroll"},
		{Key: "/", Label: "filter"},
		{Key: "q", Label: "quit"},
	})
	return kit.LayoutScreen(m.width, m.height, kit.FormatBreadcrumb("help"), body, footer)
}

// sectionBlocks holds the per-screen keymap blocks, keyed by Section.
var sectionBlocks = map[Section]string{
	SectionHome: `Home
  j / ↓      move through actions
  k / ↑      move up
  Enter      select action or recent tab
  l          open library
  o          online search
  S          settings (volume / strict audio / theme)`,
	SectionLibrary: `Library browser
  j / ↓      move down
  k / ↑      move up
  Enter      open selected tab
  /          fuzzy filter (type j/k; ↑/↓ move; Escx2 home)
  F          toggle favorites-only view
  s          cycle sort order
  o          online search
  f          toggle favorite
  e          edit title/artist (re-import overwrites)
  x          export tab to file + clipboard
  d          delete tab (with confirmation)
  r          reload library
  Esc / h    back to home`,
	SectionViewer: `Tab viewer
  a          pick audio source (local / online / MIDI)
  Space / p  play/pause
  + / -      BPM up/down
  > / <      playback speed up/down (r resets)
  m          metronome on/off (MIDI playback)
  C          count-in: 1-2 bars of lead-in clicks before playback
  y          cycle MIDI instrument (steel/nylon/clean/overdrive/bass)
  j / k      prev/next bar (shown in header)
  h / l      pan left/right
  gg         first bar
  G          last bar
  0-9 Enter  jump to bar
  /          search bar number or fret pattern (n/N next/prev)
  T / Z      transpose ±1 semitone (R resets)
  e          toggle note names instead of fret numbers
  v          toggle grid / linear layout
  f          follow-mode auto-scroll
  P          performance mode (hide tab, show section + progress)
  i / u      set loop point A / B (x clears)
  X          export tab to file + clipboard
  [ / ]      nudge audio sync ±0.5s ({ } = ±5s, , . = ±0.1s, o resets)
  s          sync current bar to audio position (during audio playback)
  S          remove the last sync anchor (repeat to remove more)
  o          reset offset (press again to undo the reset)
  Esc        clear/back/stop
  b          back to library
  H          back to home

Syncing with a recording
  1. Pick the studio/official source (a) — in strict mode live,
     cover, and lesson recordings are excluded from auto-pick.
  2. Intros are auto-detected from leading silence (offset +Xs); fine-tune
     with [ ] (0.5 s) or , . (0.1 s).
  3. During playback, jump to a bar you recognize (number + Enter), then
     press s exactly when you hear it. Repeat at 2–3 bars.
  4. Anchors build a tempo map (118->122 bpm) and a drift estimate
     (±0.3 s); the playhead follows the audio even as the tempo changes.
  Calibration is stored per audio source — switching to another recording
  restores that source's own offset and anchors.`,
	SectionSearch: `Online search
  / / i      focus query box
  Tab / ↓    move to results
  Enter      search / open result
  j / k      move through results
  m          load more results
  Esc        cancel / clear / back
  Sources: Ultimate Guitar [UG], Songsterr [ST], GuitarTabs.cc [GT],
           GuitareTab.com [GR]`,
	SectionSettings: `Settings
  j / k      move between settings
  ← / →      adjust value (volume steps)
  Enter      toggle strict audio / mute volume
  Esc / h / b back`,
}

// globalBlock lists keys that work on every screen.
const globalBlock = `Global keys
  t          cycle theme
  ?          open this help (filtered to the current screen)
  q / Ctrl+c quit

Configuration: ~/.config/fretboard/config.json`
