package help

import (
	"strings"

	"fretboard/internal/ui/kit"
	"fretboard/internal/ui/msgs"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

// HelpModel shows a scrollable keybinding help screen.
type HelpModel struct {
	viewport viewport.Model
	width    int
	height   int
}

// NewHelpModel creates a help screen.
func NewHelpModel() HelpModel {
	vp := viewport.New(80, 20)
	vp.SetContent(helpText)
	return HelpModel{viewport: vp, width: 80, height: 24}
}

// Init is part of the tea.Model interface.
func (m HelpModel) Init() tea.Cmd {
	return nil
}

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
	case tea.KeyMsg:
		switch msg.String() {
		case kit.KeyQuit, kit.KeyQuit2, "?", "esc":
			return m, func() tea.Msg { return msgs.CloseHelpMsg{} }
		}
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// View is part of the tea.Model interface.
func (m HelpModel) View() string {
	panel := kit.RenderPanel(m.width-2, "Keyboard reference", m.viewport.View())
	body := "\n" + panel
	footer := kit.RenderFooter(m.width, []kit.KeyHint{
		{Key: "Esc", Label: "close"},
		{Key: "j/k", Label: "scroll"},
		{Key: "q", Label: "quit"},
	})
	return kit.LayoutScreen(m.width, m.height, kit.FormatBreadcrumb("help"), body, footer)
}

var helpText = strings.TrimSpace(`
fretboard — keyboard reference

Home
  j / ↓      move through actions
  k / ↑      move up
  Enter      select action or recent tab
  l          open library
  o          online search
  t          cycle theme
  q / Ctrl+c quit

Library browser
  j / ↓      move down
  k / ↑      move up
  Enter      open selected tab
  /          fuzzy filter (type j/k; ↑/↓ move; Esc×2 home)
  F          toggle favorites-only view
  s          cycle sort order
  o          online search
  f          toggle favorite
  e          edit title/artist (re-import overwrites)
  x          export tab to file + clipboard
  d          delete tab (with confirmation)
  r          reload library
  Esc / h    back to home
  ?          help
  t          cycle theme
  q / Ctrl+c quit

Tab viewer
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
  v          toggle grid / linear layout
  f          follow-mode auto-scroll
  i / u      set loop point A / B (x clears)
  X          export tab to file + clipboard
  [ / ]      nudge audio sync ±0.5s ({ } = ±5s, , . = ±0.1s, o resets)
  s          sync current bar to audio position (during audio playback)
  S          clear all sync points
  Esc        clear/back/stop
  b          back to library
  H          back to home
  t          cycle theme
  ?          help
  q / Ctrl+c quit

Syncing with a recording
  1. Pick the studio/official source (a) — in strict mode live,
     cover, and lesson recordings are excluded from auto-pick.
  2. Intros are auto-detected from leading silence (↔ +Xs); fine-tune
     with [ ] (0.5 s) or , . (0.1 s).
  3. During playback, jump to a bar you recognize (number + Enter), then
     press s exactly when you hear it. Repeat at 2–3 bars.
  4. Anchors build a tempo map (118→122 bpm) and a drift estimate
     (±0.3 s); the playhead follows the audio even as the tempo changes.
  Calibration is stored per audio source — switching to another recording
  restores that source's own offset and anchors.

Online search
  / / i      focus query box
  Tab / ↓    move to results
  Enter      search / open result
  j / k      move through results
  Esc        cancel / clear / back
  ?          help
  q / Ctrl+c quit
  Sources: Ultimate Guitar [UG], Songsterr [ST], GuitarTabs.cc [GT],
           GuitareTab.com [GR]

Configuration: ~/.config/fretboard/config.json
`)
