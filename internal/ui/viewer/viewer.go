package viewer

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"fretboard/internal/model"
	"fretboard/internal/player"
	"fretboard/internal/ui/kit"
	"fretboard/internal/ui/msgs"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

// ViewerModel displays a parsed tab and can play it back.
type ViewerModel struct {
	tab               *model.Tab
	viewport          viewport.Model
	engine            *player.Engine
	tabPath           string
	tabID             int64
	audioDirs         []string
	allowOnline       bool
	resolvedAudio     string
	fetchingAudio     bool
	pendingPlay       bool
	fetchingCatalog   bool
	showAudioPicker   bool
	audioCatalog      player.AudioCatalog
	audioCursor       int
	selectedSourceIdx int
	audioSync         bool
	audioOffset       float64 // seconds into the audio file where the tab starts
	syncPoints        []player.SyncPoint
	loopStartBar      int
	loopEndBar        int
	follow            bool
	linear            bool
	cursorBar         int
	cursorCol         int
	panOffset         int
	playing           bool
	schedule          []player.PlaybackStep
	stepIdx           int
	tickDur           time.Duration
	bpm               int
	jumpBuffer        string
	lastKey           string
	lastKeyAt         time.Time
	width             int
	height            int
	errMsg            string
}

// NewViewerModel creates a viewer with default size.
func NewViewerModel() ViewerModel {
	vp := viewport.New(80, 20)
	return ViewerModel{
		viewport: vp,
		engine:   player.NewEngine(),
		width:    80,
		height:   24,
		bpm:      120,
	}
}

// LoadTab sets the tab to display and refreshes the viewport content.
func (m *ViewerModel) LoadTab(tab *model.Tab, tabPath string, tabID int64) {
	m.tab = tab
	m.tabPath = tabPath
	m.tabID = tabID
	m.cursorBar = 0
	m.cursorCol = 0
	m.panOffset = 0
	m.jumpBuffer = ""
	m.playing = false
	m.schedule = nil
	m.stepIdx = 0
	m.tickDur = 0
	m.errMsg = ""
	m.bpm = player.TabBPM(tab)
	m.resolvedAudio = player.FindAudio(tab, tabPath, m.audioDirs)
	m.fetchingAudio = false
	m.fetchingCatalog = false
	m.pendingPlay = false
	m.showAudioPicker = false
	m.audioCatalog = player.AudioCatalog{}
	m.audioCursor = 0
	m.selectedSourceIdx = 0
	m.audioSync = false
	m.audioOffset = 0
	m.syncPoints = nil
	m.loopStartBar = 0
	m.loopEndBar = 0
	m.follow = true
	if tab != nil && tab.Metadata != nil {
		if raw := strings.TrimSpace(tab.Metadata[model.MetaKeyAudioOffset]); raw != "" {
			if v, err := strconv.ParseFloat(raw, 64); err == nil {
				m.audioOffset = v
			}
		}
		m.syncPoints = parseSyncPoints(tab.Metadata[model.MetaKeySyncPoints])
	}
	_ = m.engine.Stop()
	m.engine.SetLoop(0, 0)
	m.refresh()
}

func (m *ViewerModel) refresh() {
	if m.tab == nil {
		m.viewport.SetContent(kit.MutedStyle.Render("No tab loaded."))
		return
	}
	cur := &kit.TabCursor{Bar: m.cursorBar, Col: m.cursorCol, Playing: m.playing}
	if m.linear {
		m.viewport.SetContent(kit.RenderTabLinear(m.tab, m.panOffset, cur))
	} else {
		m.viewport.SetContent(kit.RenderTabGrid(m.tab, m.viewport.Width, m.panOffset, cur))
	}
	if m.follow {
		m.ensureCursorVisible()
	}
}

// Init is part of the tea.Model interface.
func (m ViewerModel) Init() tea.Cmd {
	return nil
}

// Update is part of the tea.Model interface.
func (m ViewerModel) Update(msg tea.Msg) (ViewerModel, tea.Cmd) {
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
	case msgs.AudioFetchedMsg:
		if m.matchesAudioTab(msg.TabID, msg.TabPath, msg.Artist, msg.Title) {
			wantPlay := m.pendingPlay
			m.fetchingAudio = false
			m.pendingPlay = false
			if msg.Err == nil && msg.Path != "" {
				m.audioCatalog.SetSourcePath(m.selectedSourceIdx, msg.Path)
				m.resolvedAudio = msg.Path
				m.applySelectedSource(true)
				src := m.selectedSource()
				if src.ID != "" {
					if m.tab.Metadata == nil {
						m.tab.Metadata = map[string]string{}
					}
					m.tab.Metadata["audio_source"] = src.ID
				}
				var cmds []tea.Cmd
				if wantPlay {
					cmds = append(cmds, startPlaybackCmd(m.engine, m.tab, m.bpm, m.tabPath, m.audioDirs, m.selectedSource(), m.playbackStartIndex()))
				}
				if cmd := m.saveTabPrefsCmd(); cmd != nil {
					cmds = append(cmds, cmd)
				}
				m.refresh()
				if len(cmds) > 0 {
					return m, tea.Batch(cmds...)
				}
			} else if msg.Err != nil {
				m.errMsg = msg.Err.Error()
			}
			m.refresh()
		}
	case msgs.AudioCatalogMsg:
		if m.matchesAudioTab(msg.TabID, msg.TabPath, msg.Artist, msg.Title) {
			m.fetchingCatalog = false
			if msg.Err == nil && len(msg.Catalog.Sources) > 0 {
				prevPick := m.selectedSourceIdx
				prevCursor := m.audioCursor
				m.audioCatalog = msg.Catalog
				if m.showAudioPicker {
					if prevCursor < len(m.audioCatalog.Sources) {
						m.audioCursor = prevCursor
					} else if len(m.audioCatalog.Sources) > 0 {
						m.audioCursor = len(m.audioCatalog.Sources) - 1
					}
				} else if !m.playing {
					m.selectedSourceIdx = pickAudioSourceIndex(m.tab, msg.Catalog)
					if m.selectedSourceIdx >= len(m.audioCatalog.Sources) {
						m.selectedSourceIdx = 0
					}
					m.audioCursor = m.selectedSourceIdx
					m.applySelectedSource(true)
				} else if prevPick < len(m.audioCatalog.Sources) {
					m.selectedSourceIdx = prevPick
				}
			} else if msg.Err != nil && m.showAudioPicker {
				m.errMsg = msg.Err.Error()
			}
			m.refresh()
		}
	case msgs.PlaybackStartedMsg:
		m.playing = true
		m.schedule = msg.Schedule
		m.stepIdx = msg.StepIdx
		m.tickDur = msg.Duration
		m.audioSync = msg.AudioSync
		if len(m.schedule) > 0 && m.stepIdx >= 0 && m.stepIdx < len(m.schedule) {
			step := m.schedule[m.stepIdx]
			m.cursorBar = step.Bar
			m.cursorCol = step.Col
			m.ensureCursorVisible()
		}
		m.refresh()
		return m, tea.Batch(tickCmd(m.tickDur), monitorPlaybackCmd(m.engine))
	case msgs.PlaybackErrorMsg:
		m.stopPlayback()
		m.errMsg = msg.Err.Error()
		m.refresh()
	case msgs.PlaybackMonitorMsg:
		if m.engine.ShutdownRequested() {
			m.stopPlayback()
			return m, nil
		}
		if !m.playing {
			return m, nil
		}
		if m.audioSync && len(m.schedule) > 0 {
			elapsed := m.engine.Elapsed()
			if m.loopEndBar > 0 {
				if end, _, ok := m.engine.LoopRegion(); ok && elapsed >= end {
					_ = m.engine.RestartAt(m.loopStartTime())
					elapsed = m.engine.Elapsed()
				}
			}
			points := m.syncPoints
			if len(points) == 0 {
				points = []player.SyncPoint{{Bar: 1, Seconds: m.audioOffset}}
			}
			idx := player.StepIndexAtSyncPoints(m.schedule, points, elapsed.Seconds(), m.bpm)
			if idx != m.stepIdx {
				m.stepIdx = idx
				step := m.schedule[idx]
				m.cursorBar = step.Bar
				m.cursorCol = step.Col
				m.ensureCursorVisible()
				m.refresh()
			}
		}
		if m.engine.PlaybackEnded() {
			atEnd := len(m.schedule) == 0 || m.stepIdx >= len(m.schedule)-1
			if m.audioSync {
				m.stopPlayback()
				m.refresh()
				return m, nil
			}
			if m.engine.Mode() == "midi" && !atEnd {
				m.errMsg = "MIDI engine stopped early"
				if m.engine.LastError != "" {
					m.errMsg = "MIDI stopped: " + m.engine.LastError
				}
			}
			if atEnd || m.engine.Mode() != "midi" {
				m.stopPlayback()
				m.refresh()
				return m, nil
			}
		}
		return m, monitorPlaybackCmd(m.engine)
	case msgs.PlaybackTickMsg:
		if !m.playing || len(m.schedule) == 0 {
			return m, nil
		}
		if m.audioSync {
			return m, monitorPlaybackCmd(m.engine)
		}
		m.stepIdx++
		if m.stepIdx >= len(m.schedule) {
			m.stopPlayback()
			m.refresh()
			return m, nil
		}
		step := m.schedule[m.stepIdx]
		if m.loopEndBar > 0 && step.Bar > m.loopEndBar {
			m.stepIdx = 0
			for i, s2 := range m.schedule {
				if s2.Bar >= m.loopStartBar {
					m.stepIdx = i
					break
				}
			}
			step = m.schedule[m.stepIdx]
		}
		m.cursorBar = step.Bar
		m.cursorCol = step.Col
		m.tickDur = time.Duration(player.StepDuration(step.Ticks, m.bpm)) * time.Millisecond
		if m.engine.Mode() == "midi" {
			if err := m.engine.PlayMIDIStep(m.tab, step, m.bpm); err != nil {
				m.errMsg = err.Error()
			}
		}
		m.ensureCursorVisible()
		m.refresh()
		return m, tea.Batch(tickCmd(m.tickDur), monitorPlaybackCmd(m.engine))
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m ViewerModel) handleKey(msg tea.KeyMsg) (ViewerModel, tea.Cmd) {
	if m.showAudioPicker {
		return m.handleAudioPickerKey(msg)
	}
	if s := msg.String(); len(s) == 1 && s[0] >= '0' && s[0] <= '9' {
		m.jumpBuffer += s
		return m, nil
	}

	switch msg.String() {
	case kit.KeyQuit, kit.KeyQuit2:
		m.stopPlayback()
		return m, tea.Quit
	case "b":
		m.stopPlayback()
		m.jumpBuffer = ""
		return m, func() tea.Msg { return msgs.ViewLibraryMsg{} }
	case "H":
		m.stopPlayback()
		m.jumpBuffer = ""
		return m, func() tea.Msg { return msgs.ViewHomeMsg{} }
	case "a":
		return m.openAudioPicker()
	case " ", "p":
		cmd := m.togglePlayback()
		m.jumpBuffer = ""
		return m, cmd
	case "+", "=":
		m.bpm = player.ClampBPM(m.bpm + 5)
		m.jumpBuffer = ""
		if m.playing && m.tab != nil {
			_ = m.engine.Stop()
			m.resetPlayback()
			m.refresh()
			return m, startPlaybackCmd(m.engine, m.tab, m.bpm, m.tabPath, m.audioDirs, m.selectedSource(), m.playbackStartIndex())
		}
		m.refresh()
	case "-", "_":
		m.bpm = player.ClampBPM(m.bpm - 5)
		m.jumpBuffer = ""
		if m.playing && m.tab != nil {
			_ = m.engine.Stop()
			m.resetPlayback()
			m.refresh()
			return m, startPlaybackCmd(m.engine, m.tab, m.bpm, m.tabPath, m.audioDirs, m.selectedSource(), m.playbackStartIndex())
		}
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
	case "s":
		if m.audioSync && m.playing && m.tab != nil {
			return m.setSyncPoint()
		}
	case "S":
		if len(m.syncPoints) > 0 {
			m.syncPoints = nil
			m.saveSyncPoints()
			m.refresh()
		}
	case "i":
		if m.tab != nil {
			return m.setLoopPoint(true)
		}
	case "u":
		if m.tab != nil {
			return m.setLoopPoint(false)
		}
	case "x":
		m.loopStartBar, m.loopEndBar = 0, 0
		m.engine.SetLoop(0, 0)
		m.refresh()
	case ">":
		_ = m.engine.SetRate(m.engine.Rate() * 1.1)
		m.refresh()
	case "<":
		_ = m.engine.SetRate(m.engine.Rate() / 1.1)
		m.refresh()
	case "r":
		_ = m.engine.SetRate(1)
		m.refresh()
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
	case "[", "{", "]", "}", "o":
		return m.adjustAudioOffset(msg.String())
	case "esc":
		if m.jumpBuffer != "" {
			m.jumpBuffer = ""
			return m, nil
		}
		if m.errMsg != "" {
			m.errMsg = ""
			m.refresh()
			return m, nil
		}
		if m.playing {
			m.stopPlayback()
			m.refresh()
			return m, nil
		}
		m.stopPlayback()
		return m, func() tea.Msg { return msgs.ViewLibraryMsg{} }
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

// adjustAudioOffset nudges the audio start offset used to sync the tab cursor
// with a real recording, and persists it on the tab.
func (m ViewerModel) adjustAudioOffset(key string) (ViewerModel, tea.Cmd) {
	switch key {
	case "[":
		m.audioOffset -= 0.5
	case "{":
		m.audioOffset -= 5
	case "]":
		m.audioOffset += 0.5
	case "}":
		m.audioOffset += 5
	case "o":
		m.audioOffset = 0
	}
	if m.audioOffset < -60 {
		m.audioOffset = -60
	}
	if m.audioOffset > 300 {
		m.audioOffset = 300
	}
	m.refresh()
	if m.tab == nil {
		return m, nil
	}
	if m.tab.Metadata == nil {
		m.tab.Metadata = map[string]string{}
	}
	m.tab.Metadata[model.MetaKeyAudioOffset] = strconv.FormatFloat(m.audioOffset, 'f', 1, 64)
	return m, m.saveTabPrefsCmd()
}

// audioOffsetDuration converts the calibrated audio offset to a duration.
func (m ViewerModel) audioOffsetDuration() time.Duration {
	return time.Duration(m.audioOffset * float64(time.Second))
}

// parseSyncPoints decodes the persisted [[bar, seconds], ...] metadata.
func parseSyncPoints(raw string) []player.SyncPoint {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var points []player.SyncPoint
	if err := json.Unmarshal([]byte(raw), &points); err != nil {
		return nil
	}
	seen := map[int]bool{}
	clean := points[:0]
	for _, p := range points {
		if p.Bar < 1 || seen[p.Bar] {
			continue
		}
		seen[p.Bar] = true
		clean = append(clean, p)
	}
	return clean
}

// setSyncPoint anchors the current bar to the current audio position.
func (m ViewerModel) setSyncPoint() (ViewerModel, tea.Cmd) {
	elapsed := m.engine.Elapsed()
	bar := m.cursorBar + 1
	if bar == 1 {
		m.audioOffset = elapsed.Seconds()
		m.tab.Metadata[model.MetaKeyAudioOffset] = strconv.FormatFloat(m.audioOffset, 'f', 1, 64)
	}
	m.syncPoints = append(m.syncPoints, player.SyncPoint{Bar: bar, Seconds: elapsed.Seconds()})
	m.saveSyncPoints()
	m.refresh()
	return m, m.saveTabPrefsCmd()
}

// saveSyncPoints persists the anchors (plus the bar-1 offset anchor) to the
// tab metadata as JSON.
func (m *ViewerModel) saveSyncPoints() {
	if m.tab == nil {
		return
	}
	points := append([]player.SyncPoint{{Bar: 1, Seconds: m.audioOffset}}, m.syncPoints...)
	seen := map[int]bool{}
	out := points[:0]
	for _, p := range points {
		if seen[p.Bar] {
			continue
		}
		seen[p.Bar] = true
		out = append(out, p)
	}
	if len(out) == 1 {
		m.tab.Metadata[model.MetaKeySyncPoints] = ""
		return
	}
	data, err := json.Marshal(out)
	if err != nil {
		return
	}
	m.tab.Metadata[model.MetaKeySyncPoints] = string(data)
}

// loopStartTime returns the schedule time of the loop start bar.
func (m ViewerModel) loopStartTime() time.Duration {
	if len(m.schedule) == 0 {
		return 0
	}
	return player.ScheduleTimeAtBar(m.schedule, m.loopStartBar, m.bpm)
}

// setLoopPoint registers the A (start) or B (end) loop boundary at the current
// bar, mapped to schedule time, and arms the engine loop region.
func (m ViewerModel) setLoopPoint(isStart bool) (ViewerModel, tea.Cmd) {
	bar := m.cursorBar + 1
	if isStart {
		m.loopStartBar = bar
	} else {
		m.loopEndBar = bar
	}
	if m.loopStartBar > 0 && m.loopEndBar > 0 && m.loopEndBar <= m.loopStartBar {
		m.loopEndBar = m.loopStartBar + 1
	}
	if len(m.schedule) > 0 {
		start := player.ScheduleTimeAtBar(m.schedule, m.loopStartBar, m.bpm)
		end := player.ScheduleTimeAtBar(m.schedule, m.loopEndBar+1, m.bpm)
		if end > start {
			m.engine.SetLoop(start, end)
		}
	}
	m.refresh()
	return m, nil
}

func (m ViewerModel) matchesAudioTab(tabID int64, tabPath, artist, title string) bool {
	if m.tab == nil {
		return false
	}
	if tabID > 0 && m.tabID > 0 {
		return tabID == m.tabID
	}
	if tabPath != "" && m.tabPath != "" && tabPath == m.tabPath {
		return true
	}
	return m.tab.Artist == artist && m.tab.Title == title
}

// resetPlayback clears UI playback state without starting audio.
func (m *ViewerModel) resetPlayback() {
	m.playing = false
	m.audioSync = false
	m.schedule = nil
	m.stepIdx = 0
	m.tickDur = 0
	m.pendingPlay = false
}

// stopPlaybackForNav halts audio before navigating away from the viewer.
func (m *ViewerModel) stopPlaybackForNav() {
	if m.playing {
		m.stopPlayback()
	}
}

// stopPlayback halts audio and clears UI playback state.
func (m *ViewerModel) stopPlayback() {
	m.resetPlayback()
	_ = m.engine.Stop()
}

func (m ViewerModel) playbackStartIndex() int {
	if m.tab == nil {
		return 0
	}
	schedule := player.BuildSchedule(m.tab)
	if len(schedule) == 0 {
		return 0
	}
	if m.playing && m.stepIdx >= 0 && m.stepIdx < len(schedule) {
		return m.stepIdx
	}
	return player.StepIndexAtPosition(schedule, m.cursorBar, m.cursorCol)
}

func (m ViewerModel) saveTabPrefsCmd() tea.Cmd {
	if m.tab == nil {
		return nil
	}
	if m.tabID <= 0 && strings.TrimSpace(m.tabPath) == "" {
		return nil
	}
	return func() tea.Msg { return msgs.TabPrefsSaveMsg{} }
}

func (m *ViewerModel) togglePlayback() tea.Cmd {
	if m.tab == nil {
		return nil
	}
	if m.playing {
		m.stopPlayback()
		m.refresh()
		return nil
	}
	if m.fetchingAudio {
		return nil
	}
	m.errMsg = ""
	src := m.selectedSource()
	if src.Kind == player.SourceOnline && (src.Path == "" || !player.FileExists(src.Path)) {
		if m.resolvedAudio != "" && player.FileExists(m.resolvedAudio) {
			m.audioCatalog.SetSourcePath(m.selectedSourceIdx, m.resolvedAudio)
			src = m.selectedSource()
		} else {
			m.fetchingAudio = true
			m.pendingPlay = true
			return m.downloadSelectedSourceCmd()
		}
	}
	return startPlaybackCmd(m.engine, m.tab, m.bpm, m.tabPath, m.audioDirs, src, m.playbackStartIndex())
}

func (m *ViewerModel) maxPanOffset() int {
	if m.tab == nil {
		return 0
	}
	metrics := kit.BarGridLayout(m.tab, m.viewport.Width)
	gridWidth := metrics.BarsPerRow * metrics.BarWidth
	visible := m.viewport.Width - 10
	if visible < 20 {
		visible = 20
	}
	if gridWidth <= visible {
		return 0
	}
	return gridWidth - visible
}

func (m *ViewerModel) ensureCursorVisible() {
	if m.tab == nil {
		return
	}
	target := barGridLineOffset(m.tab, m.cursorBar, m.viewport.Width)
	if target < m.viewport.YOffset {
		m.viewport.SetYOffset(target)
	} else if target >= m.viewport.YOffset+m.viewport.Height {
		m.viewport.SetYOffset(target - m.viewport.Height + 1)
	}
}

// barGridLineOffset returns the content line where the cursor bar's row begins,
// using the same page-layout metrics as the renderer.
func barGridLineOffset(tab *model.Tab, barIdx, availWidth int) int {
	offset := 0
	if tab.Title != "" {
		offset += 2
	}
	if len(tab.Bars) == 0 {
		return offset
	}
	metrics := kit.BarGridLayout(tab, availWidth)
	row := barIdx / metrics.BarsPerRow
	return offset + row*metrics.RowHeight
}


// View is part of the tea.Model interface.
func (m ViewerModel) View() string {
	crumb := kit.FormatBreadcrumb("home", "library", "viewer")
	panelTitle := "Tab"
	if m.tab != nil {
		crumb = kit.FormatBreadcrumb("home", "library", m.tab.Title)
		panelTitle = fmt.Sprintf("%s · %s · bar %d/%d · %d BPM", m.tab.Title, m.tab.Tuning.Label(), m.cursorBar+1, len(m.tab.Bars), m.bpm)
		if m.playing {
			label := m.engine.ActiveDriver
			if m.engine.Mode() == "audio" {
				if label == "" {
					label = "audio"
				}
				panelTitle += kit.SuccessStyle.Render("  ▶ " + label)
			} else if label != "" {
				panelTitle += kit.SuccessStyle.Render("  ▶ midi:" + label)
			} else {
				panelTitle += kit.SuccessStyle.Render("  ▶ midi")
			}
		}
		if src := m.selectedSource(); !m.playing {
			if m.fetchingCatalog || m.fetchingAudio {
				panelTitle += kit.MutedStyle.Render("  … finding audio")
			} else if src.Kind == player.SourceMIDI {
				panelTitle += kit.MutedStyle.Render("  ♪ midi")
			} else if m.resolvedAudio != "" {
				panelTitle += kit.MutedStyle.Render("  ♪ " + filepath.Base(m.resolvedAudio))
			}
		}
		if m.audioOffset != 0 {
			panelTitle += kit.MutedStyle.Render(fmt.Sprintf("  ↔ %+.1fs", m.audioOffset))
		}
		if len(m.syncPoints) > 0 {
			panelTitle += kit.MutedStyle.Render(fmt.Sprintf("  ⚓%d", len(m.syncPoints)))
		}
		if m.loopStartBar > 0 && m.loopEndBar > 0 {
			panelTitle += kit.MutedStyle.Render(fmt.Sprintf("  ↻ %d-%d", m.loopStartBar, m.loopEndBar))
		}
		if rate := m.engine.Rate(); rate != 1 {
			panelTitle += kit.MutedStyle.Render(fmt.Sprintf("  ⏩ ×%.2f", rate))
		}
		if !m.showAudioPicker {
			panelTitle += kit.MutedStyle.Render("  [a] source")
		}
	}
	if m.jumpBuffer != "" {
		panelTitle += kit.MutedStyle.Render("  [jump " + m.jumpBuffer + "]")
	}
	if m.errMsg != "" {
		panelTitle += "  " + kit.ErrorStyle.Render("⚠ "+kit.Truncate(m.errMsg, 48))
	}

	body := "\n" + kit.RenderPanel(m.width-2, panelTitle, m.viewport.View())
	if m.showAudioPicker {
		body += RenderAudioPicker(m.width, m.audioCatalog, m.audioCursor, m.fetchingCatalog)
	}

	playLabel := "play"
	if m.playing {
		playLabel = "pause"
	}
	footer := kit.RenderFooter(m.width, []kit.KeyHint{
		{Key: "a", Label: "audio"},
		{Key: "Space/p", Label: playLabel},
		{Key: "+/-", Label: "BPM"},
		{Key: "> <", Label: "speed"},
		{Key: "[ ]", Label: "sync ±0.5s"},
		{Key: "o", Label: "reset sync"},
		{Key: "s", Label: "sync bar"},
		{Key: "i/u", Label: "loop A-B"},
		{Key: "x", Label: "clear loop"},
		{Key: "v", Label: "layout"},
		{Key: "f", Label: "follow"},
		{Key: "j/k", Label: "scroll"},
		{Key: "b", Label: "library"},
		{Key: "q", Label: "quit"},
	})
	return kit.LayoutScreen(m.width, m.height, crumb, body, footer)
}

// SetAudioDirs configures extra directories to search for backing tracks.
func (m *ViewerModel) SetAudioDirs(dirs []string) {
	m.audioDirs = append([]string(nil), dirs...)
}

func pickAudioSourceIndex(tab *model.Tab, cat player.AudioCatalog) int {
	if tab != nil && tab.Metadata != nil {
		if srcID := strings.TrimSpace(tab.Metadata["audio_source"]); srcID != "" {
			if found := cat.FindByID(srcID); found >= 0 {
				return found
			}
		}
	}
	// Prefer a ready local backing track; default to MIDI tab synth (not online/BestIndex).
	for i, src := range cat.Sources {
		if src.Kind == player.SourceLocal && src.Path != "" && player.FileExists(src.Path) {
			return i
		}
	}
	return 0
}

func (m ViewerModel) selectedSource() player.AudioSource {
	if src := m.audioCatalog.Selected(m.selectedSourceIdx); src != nil {
		return *src
	}
	return player.AudioSource{ID: "midi", Kind: player.SourceMIDI, Label: "MIDI synthesizer"}
}

func (m *ViewerModel) applySelectedSource(deriveBPM bool) {
	src := m.selectedSource()
	if src.Kind == player.SourceMIDI {
		m.resolvedAudio = ""
		return
	}
	path := src.Path
	if src.Kind == player.SourceOnline && (path == "" || !player.FileExists(path)) {
		if m.resolvedAudio != "" && player.FileExists(m.resolvedAudio) {
			path = m.resolvedAudio
		} else {
			return
		}
	}
	if path != "" {
		m.resolvedAudio = path
	}
	if deriveBPM && m.tab != nil && path != "" {
		if dur, err := player.ProbeDuration(path); err == nil && dur > 0 {
			schedule := player.BuildSchedule(m.tab)
			if meta := m.tab.Metadata; meta == nil || strings.TrimSpace(meta[model.MetaKeyBPM]) == "" {
				m.bpm = player.DeriveBPMFromAudio(schedule, dur, m.audioOffsetDuration())
			}
		}
	}
}

func (m ViewerModel) openAudioPicker() (ViewerModel, tea.Cmd) {
	if m.tab == nil {
		return m, nil
	}
	m.showAudioPicker = true
	m.errMsg = ""
	if len(m.audioCatalog.Sources) <= 1 {
		m.fetchingCatalog = true
		return m, fetchAudioCatalogCmd(m.tab, m.tabPath, m.tabID, m.audioDirs, m.allowOnline)
	}
	m.audioCursor = m.selectedSourceIdx
	return m, nil
}

func (m ViewerModel) handleAudioPickerKey(msg tea.KeyMsg) (ViewerModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.showAudioPicker = false
		m.fetchingCatalog = false
		return m, nil
	case "j", "down":
		if len(m.audioCatalog.Sources) > 0 && m.audioCursor < len(m.audioCatalog.Sources)-1 {
			m.audioCursor++
		}
		return m, nil
	case "k", "up":
		if m.audioCursor > 0 {
			m.audioCursor--
		}
		return m, nil
	case "r":
		m.fetchingCatalog = true
		return m, fetchAudioCatalogCmd(m.tab, m.tabPath, m.tabID, m.audioDirs, m.allowOnline)
	case "enter":
		if len(m.audioCatalog.Sources) == 0 {
			return m, nil
		}
		if m.playing {
			m.stopPlayback()
		}
		m.selectedSourceIdx = m.audioCursor
		m.showAudioPicker = false
		src := m.selectedSource()
		if src.Kind == player.SourceOnline && (src.Path == "" || !player.FileExists(src.Path)) {
			m.fetchingAudio = true
			m.pendingPlay = true
			return m, m.downloadSelectedSourceCmd()
		}
		m.applySelectedSource(true)
		if m.tab != nil {
			if m.tab.Metadata == nil {
				m.tab.Metadata = map[string]string{}
			}
			m.tab.Metadata["audio_source"] = src.ID
		}
		return m, m.saveTabPrefsCmd()
	}
	return m, nil
}

func (m ViewerModel) downloadSelectedSourceCmd() tea.Cmd {
	tab := m.tab
	tabID := m.tabID
	tabPath := m.tabPath
	src := m.selectedSource()
	return func() tea.Msg {
		path, err := player.EnsureAudioSource(tab, src)
		return msgs.AudioFetchedMsg{Path: path, Err: err, Artist: tab.Artist, Title: tab.Title, TabID: tabID, TabPath: tabPath}
	}
}

// BeginAudioFetch loads ranked audio options in the background.
func (m *ViewerModel) BeginAudioFetch(allowOnline bool) tea.Cmd {
	m.allowOnline = allowOnline
	if m.tab == nil {
		return nil
	}
	m.resolvedAudio = player.FindAudio(m.tab, m.tabPath, m.audioDirs)
	cat, err := player.BuildAudioCatalog(m.tab, m.tabPath, m.audioDirs, false)
	if err == nil && len(cat.Sources) > 0 {
		m.audioCatalog = cat
		m.selectedSourceIdx = pickAudioSourceIndex(m.tab, cat)
		if m.selectedSourceIdx >= len(m.audioCatalog.Sources) {
			m.selectedSourceIdx = 0
		}
		m.audioCursor = m.selectedSourceIdx
		m.applySelectedSource(true)
	}
	if !allowOnline || !player.OnlineAudioAvailable() || player.AudioSearchQuery(m.tab) == "" {
		return nil
	}
	m.fetchingCatalog = true
	return fetchAudioCatalogCmd(m.tab, m.tabPath, m.tabID, m.audioDirs, allowOnline)
}

// StopPlayback halts any running audio.
func (m *ViewerModel) StopPlayback() { m.stopPlayback() }

// ShutdownAudio releases the synthesizer engine.
func (m *ViewerModel) ShutdownAudio() { m.engine.Shutdown() }

// SetVolume sets the synthesizer volume (0-100).
func (m *ViewerModel) SetVolume(v int) {
	if v < 0 {
		v = 0
	}
	if v > 100 {
		v = 100
	}
	m.engine.Volume = v
}

// SetSoundfont sets the soundfont used by the synthesizer.
func (m *ViewerModel) SetSoundfont(path string) { m.engine.Soundfont = path }

// SetError sets the viewer error banner.
func (m *ViewerModel) SetError(msg string) { m.errMsg = msg }

// Tab returns the currently loaded tab, or nil.
func (m *ViewerModel) Tab() *model.Tab { return m.tab }

// TabPath returns the path of the currently loaded tab.
func (m *ViewerModel) TabPath() string { return m.tabPath }

// TabID returns the library id of the currently loaded tab.
func (m *ViewerModel) TabID() int64 { return m.tabID }
