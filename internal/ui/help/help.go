package help

import (
	"strings"

	"github.com/YOUR_USERNAME/fretboard/internal/ui/kit"
	"github.com/YOUR_USERNAME/fretboard/internal/ui/msgs"
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
  s          cycle sort order
  o          online search
  f          toggle favorite
  d          delete tab
  r          reload library
  Esc / h    back to home
  ?          help
  t          cycle theme
  q / Ctrl+c quit

Tab viewer
  a          pick audio source (local / online / MIDI)
  Space / p  play/pause
  + / -      BPM up/down
  j / k      prev/next bar (shown in header)
  h / l      pan left/right
  gg         first bar
  G          last bar
  0-9 Enter  jump to bar
  Esc        clear/back/stop
  b          back to library
  H          back to home
  t          cycle theme
  ?          help
  q / Ctrl+c quit

Online search
  / / i      focus query box
  Tab / ↓    move to results
  Enter      search / open result
  j / k      move through results
  Esc        cancel / clear / back
  ?          help
  q / Ctrl+c quit

Configuration: ~/.config/fretboard/config.json
`)
