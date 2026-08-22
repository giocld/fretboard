package app

import (
	"fmt"
	"strings"

	"fretboard/internal/config"
	"fretboard/internal/diag"
	"fretboard/internal/parser"
	"fretboard/internal/player"
	"fretboard/internal/ui/kit"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// tourStartMsg raises the first-run tour overlay. Router-internal message:
// the tour is app state, not a cross-screen contract.
type tourStartMsg struct{}

// diagProbeMsg delivers the startup dependency probe result: the display
// names of missing critical dep groups ("fluidsynth/timidity", "mpv/ffplay").
type diagProbeMsg struct {
	missing []string
}

// onlineAudioAvailable reports whether yt-dlp is installed. It is a var so
// tests can force the online path without installing yt-dlp.
var onlineAudioAvailable = player.OnlineAudioAvailable

// tourCards is the first-run tour (8.3): three short cards walked with
// Enter and skipped with Esc.
var tourCards = []struct{ title, body string }{
	{
		"Your library",
		"Start here: your tab library, favorites, and most-recent plays. " +
			"Press l to open the browser.",
	},
	{
		"Open a tab",
		"In the library, press Enter on a row to open the tab in the viewer. " +
			"Space plays it, with synced audio when a backing track is found.",
	},
	{
		"Need help?",
		"Press ? anywhere to open the keymap. It is filtered to the screen " +
			"you are on, and / searches it.",
	},
}

// consentText explains the one-time online-audio consent (3.4): what
// "online audio" actually does before the user commits to it.
const consentText = "Online audio downloads a song's audio from YouTube (via yt-dlp) " +
	"into a local cache for playback."

// tourCheckCmd returns a command that raises the first-run tour overlay when
// the tour has not been seen yet. Queued from Init so the tour never appears
// in unit tests that drive Update directly.
func (m AppModel) tourCheckCmd() tea.Cmd {
	return func() tea.Msg {
		cfg, err := config.Load()
		if err != nil || cfg.TourSeen {
			return nil
		}
		return tourStartMsg{}
	}
}

// diagProbeCmd runs the dependency probe asynchronously (8.2):
// diag.RunChecks includes a live yt-dlp probe that can take up to 15s, so
// startup never blocks — results surface when they arrive.
func (m AppModel) diagProbeCmd() tea.Cmd {
	return func() tea.Msg {
		missing := criticalMissing(diag.RunChecks())
		if len(missing) == 0 {
			return nil
		}
		return diagProbeMsg{missing: missing}
	}
}

// criticalMissing extracts the critical playback deps that are absent:
// MIDI playback needs fluidsynth or timidity, audio playback needs mpv or
// ffplay. The returned names are display labels for the home banner.
func criticalMissing(results []diag.CheckResult) []string {
	ok := map[string]bool{}
	for _, r := range results {
		ok[r.Name] = r.OK
	}
	var missing []string
	switch {
	case !ok["fluidsynth"] && !ok["timidity"]:
		missing = append(missing, "fluidsynth/timidity")
	case !ok["fluidsynth"]:
		missing = append(missing, "fluidsynth")
	case !ok["timidity"]:
		missing = append(missing, "timidity")
	}
	switch {
	case !ok["mpv"] && !ok["ffplay"]:
		missing = append(missing, "mpv/ffplay")
	case !ok["mpv"]:
		missing = append(missing, "mpv")
	case !ok["ffplay"]:
		missing = append(missing, "ffplay")
	}
	return missing
}

// tourAdvance walks to the next tour card; the last card completes the tour.
func (m AppModel) tourAdvance() (AppModel, tea.Cmd) {
	if m.tourCard+1 < len(tourCards) {
		m.tourCard++
		return m, nil
	}
	return m.tourFinish()
}

// tourSkip dismisses the tour early. Completion and skip both mark the tour
// seen so it never shows again.
func (m AppModel) tourSkip() (AppModel, tea.Cmd) {
	return m.tourFinish()
}

func (m AppModel) tourFinish() (AppModel, tea.Cmd) {
	m.tourPending = false
	m.tourCard = 0
	cfg, _ := config.Load()
	cfg.TourSeen = true
	_ = config.Save(cfg)
	return m, nil
}

// consentKey resolves the one-time online-audio consent: Accept persists
// consent and proceeds with online audio; Decline pins the session to
// local-only and never re-prompts. Every other key is swallowed — the modal
// waits for a decision.
func (m AppModel) consentKey(key tea.KeyMsg) (AppModel, tea.Cmd) {
	switch key.String() {
	case "enter", "y":
		cfg, _ := config.Load()
		cfg.ConsentOnlineAudio = true
		_ = config.Save(cfg)
		m.consentPending = false
		return m, m.viewer.BeginAudioFetch(true)
	case "esc", "n":
		m.consentDeclined = true
		m.consentPending = false
		return m, m.viewer.BeginAudioFetch(false)
	}
	return m, nil
}

// gpPickerState is an in-flight multi-track Guitar Pro import: the picked
// file's tracks and the highlighted row.
type gpPickerState struct {
	path   string
	tracks []parser.GPTrack
	cursor int
}

// gpPickerKey drives the track picker: j/k moves, Enter imports the
// highlighted track, Esc cancels the import.
func (m AppModel) gpPickerKey(key tea.KeyMsg) (AppModel, tea.Cmd) {
	switch key.String() {
	case "j", "down":
		if m.gpPicker.cursor+1 < len(m.gpPicker.tracks) {
			m.gpPicker.cursor++
		}
	case "k", "up":
		if m.gpPicker.cursor > 0 {
			m.gpPicker.cursor--
		}
	case "enter":
		return m.gpImportPicked()
	case "esc":
		m.gpPicker = nil
	}
	return m, nil
}

// overlay renders a centered overlay panel over the current screen.
func (m AppModel) overlay(title, body string) string {
	width := min(m.width-4, 64)
	if width < 20 {
		width = 20
	}
	panel := kit.RenderPanel(width, title, body)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, panel)
}

// consentView renders the online-audio consent overlay.
func (m AppModel) consentView() string {
	body := consentText + "\n\n" +
		kit.InfoStyle.Render("Accept") + "  allow on-demand audio downloads to the local cache\n" +
		kit.MutedStyle.Render("Decline") + "  local audio only for this session\n\n" +
		kit.MutedStyle.Render("[Enter/y] accept   [Esc/n] decline")
	return m.overlay("Online audio consent", body)
}

// tourView renders the current first-run tour card.
func (m AppModel) tourView() string {
	card := tourCards[m.tourCard]
	body := fmt.Sprintf("%s\n\n%s\n\n%s\n%s",
		kit.InfoStyle.Render(card.title), card.body,
		fmt.Sprintf("card %d/%d", m.tourCard+1, len(tourCards)),
		kit.MutedStyle.Render("[Enter] next   [Esc] skip"))
	return m.overlay("Welcome to fretboard", body)
}

// gpPickerView renders the track picker for a multi-track Guitar Pro file.
func (m AppModel) gpPickerView() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("This Guitar Pro file has %d tracks — pick one to import:\n\n", len(m.gpPicker.tracks)))
	for i, tr := range m.gpPicker.tracks {
		name := tr.Name
		if name == "" {
			name = fmt.Sprintf("Track %d", i+1)
		}
		line := fmt.Sprintf("%s  %s  %s", name, tr.Instrument, trackStringsLabel(tr))
		if i == m.gpPicker.cursor {
			b.WriteString(kit.ListSelected.Render("▸ " + line))
		} else {
			b.WriteString(kit.ListNormal.Render("  " + line))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n" + kit.MutedStyle.Render("[j/k] move   [Enter] import   [Esc] cancel"))
	return m.overlay("Import Guitar Pro track", b.String())
}

// trackStringsLabel renders a track's string count ("6 strings").
func trackStringsLabel(tr parser.GPTrack) string {
	if tr.Strings <= 0 {
		return ""
	}
	return fmt.Sprintf("%d strings", tr.Strings)
}
