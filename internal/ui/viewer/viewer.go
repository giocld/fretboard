package viewer

import (
	"encoding/json"
	"fmt"
	"math"
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
	strictAudio       bool
	infoMsg           string
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
	m.infoMsg = ""
	m.restoreCalibrationForSource()
	_ = m.engine.Stop()
	m.engine.SetLoop(0, 0)
	m.refresh()
}

// currentSourceID returns the ID of the selected audio source, or "" when no
// catalog is loaded. Calibration is stored per source so switching from a
// studio version (short intro) to a live version (long intro) restores the
// right offset instead of sharing one misaligned value.
func (m ViewerModel) currentSourceID() string {
	src := m.selectedSource()
	if src.Kind == player.SourceMIDI {
		return ""
	}
	return src.ID
}

// audioOffsetKey returns the metadata key holding the calibrated intro
// offset for the current source, falling back to the legacy global key.
func (m ViewerModel) audioOffsetKey() string {
	if id := m.currentSourceID(); id != "" {
		return model.MetaKeyAudioOffset + ":" + id
	}
	return model.MetaKeyAudioOffset
}

// syncPointsKey returns the metadata key holding sync anchors for the current
// source, falling back to the legacy global key.
func (m ViewerModel) syncPointsKey() string {
	if id := m.currentSourceID(); id != "" {
		return model.MetaKeySyncPoints + ":" + id
	}
	return model.MetaKeySyncPoints
}

// restoreCalibrationForSource loads the intro offset and sync anchors for the
// currently selected source (falling back to the legacy per-tab values), so a
// source switch never carries another recording's calibration.
func (m *ViewerModel) restoreCalibrationForSource() {
	m.audioOffset = 0
	m.syncPoints = nil
	if m.tab == nil || m.tab.Metadata == nil {
		return
	}
	offset := ""
	for _, key := range []string{m.audioOffsetKey(), model.MetaKeyAudioOffset} {
		if offset = strings.TrimSpace(m.tab.Metadata[key]); offset != "" {
			break
		}
	}
	if offset != "" {
		if v, err := strconv.ParseFloat(offset, 64); err == nil {
			m.audioOffset = v
		}
	}
	for _, key := range []string{m.syncPointsKey(), model.MetaKeySyncPoints} {
		if points := parseSyncPoints(m.tab.Metadata[key]); len(points) > 0 {
			m.syncPoints = points
			break
		}
	}
}

// saveCalibrationForSource persists the current offset and anchors under the
// current source's keys, mirroring the legacy keys so tabs without source
// metadata keep working (and older builds stay compatible).
func (m *ViewerModel) saveCalibrationForSource() {
	if m.tab == nil {
		return
	}
	if m.tab.Metadata == nil {
		m.tab.Metadata = map[string]string{}
	}
	offset := strconv.FormatFloat(m.audioOffset, 'f', 1, 64)
	m.tab.Metadata[m.audioOffsetKey()] = offset
	m.tab.Metadata[model.MetaKeyAudioOffset] = offset
	m.saveSyncPoints()
}

func (m *ViewerModel) refresh() {
	if m.tab == nil {
		m.viewport.SetContent(kit.MutedStyle.Render("No tab loaded."))
		return
	}
	cur := &kit.TabCursor{Bar: m.cursorBar, Col: m.cursorCol, Playing: m.playing}
	if m.loopStartBar > 0 && m.loopEndBar > m.loopStartBar {
		cur.LoopStartBar = m.loopStartBar - 1
		cur.LoopEndBar = m.loopEndBar
	}
	if m.linear {
		m.viewport.SetContent(kit.RenderTabLinearBody(m.tab, m.panOffset, cur))
	} else {
		m.viewport.SetContent(kit.RenderTabGridBody(m.tab, m.viewport.Width, m.panOffset, cur))
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
				m.restoreCalibrationForSource()
				var cmds []tea.Cmd
				if wantPlay {
					cmds = append(cmds, startPlaybackCmd(m.engine, m.tab, m.bpm, m.tabPath, m.audioDirs, m.selectedSource(), m.playbackStartIndex()))
				}
				if cmd := m.saveTabPrefsCmd(); cmd != nil {
					cmds = append(cmds, cmd)
				}
				if cmd := m.maybeDetectIntroCmd(); cmd != nil {
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
			// A failed online search must not hide the sources that did
			// resolve (local files, MIDI): keep them and surface the error.
			if msg.Err != nil {
				m.errMsg = msg.Err.Error()
			}
			if len(msg.Catalog.Sources) > 0 {
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
					m.selectedSourceIdx = m.autoPickIndex(msg.Catalog)
					if m.selectedSourceIdx >= len(m.audioCatalog.Sources) {
						m.selectedSourceIdx = 0
					}
					m.audioCursor = m.selectedSourceIdx
					// Restore the source's calibration before deriving BPM so
					// the intro offset is excluded from the tempo math.
					m.restoreCalibrationForSource()
					m.applySelectedSource(true)
				} else if prevPick < len(m.audioCatalog.Sources) {
					m.selectedSourceIdx = prevPick
				}
			}
			m.refresh()
			return m, m.maybeDetectIntroCmd()
		}
	case msgs.IntroDetectedMsg:
		if !m.matchesAudioTab(msg.TabID, msg.TabPath, msg.Artist, msg.Title) {
			return m, nil
		}
		if msg.SourceID != "" && msg.SourceID != m.currentSourceID() {
			return m, nil // the user switched sources while the probe ran
		}
		if msg.Err == nil && msg.Offset > 0 && m.audioOffset == 0 && len(m.syncPoints) == 0 {
			m.audioOffset = msg.Offset.Seconds()
			if m.tab.Metadata == nil {
				m.tab.Metadata = map[string]string{}
			}
			if id := m.currentSourceID(); id != "" {
				m.tab.Metadata["audio_offset_auto:"+id] = "1"
			}
			m.tab.Metadata["audio_offset_auto"] = "1"
			offset := strconv.FormatFloat(m.audioOffset, 'f', 1, 64)
			m.tab.Metadata[m.audioOffsetKey()] = offset
			m.tab.Metadata[model.MetaKeyAudioOffset] = offset
			m.infoMsg = fmt.Sprintf("auto-detected intro ↔ %+.1fs — fine-tune with [ ] , .", m.audioOffset)
			m.refresh()
			return m, m.saveTabPrefsCmd()
		}
		return m, nil
	case msgs.PlaybackStartedMsg:
		m.playing = true
		m.schedule = msg.Schedule
		m.stepIdx = msg.StepIdx
		m.tickDur = msg.Duration
		m.audioSync = msg.AudioSync
		// Re-arm the A-B loop region from the stored bars: loop points set
		// while paused never reached the engine before, so audio-synced
		// playback silently never looped.
		m.applyLoopRegion()
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
					if err := m.engine.RestartAt(m.loopStartTime()); err != nil {
						m.errMsg = "Loop restart failed: " + err.Error()
						m.stopPlayback()
						m.refresh()
						return m, nil
					}
					elapsed = m.engine.Elapsed()
				}
			}
			// Sync points persist user-facing 1-based bars; the schedule uses
			// 0-based bar indices, so convert before mapping.
			points := syncPointsZeroBased(m.syncPoints)
			if len(points) == 0 {
				points = []player.SyncPoint{{Bar: 0, Seconds: m.audioOffset}}
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
				m.refresh()
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
		if m.loopEndBar > 0 {
			atEnd := m.stepIdx >= len(m.schedule)
			beyondLoop := !atEnd && m.schedule[m.stepIdx].Bar >= m.loopEndBar
			if atEnd || beyondLoop {
				// Wrap to the loop start. atEnd also covers a loop whose end
				// bar is the tab's last bar (no later step can trigger it).
				m.stepIdx = 0
				for i, s2 := range m.schedule {
					if s2.Bar >= m.loopStartBar-1 {
						m.stepIdx = i
						break
					}
				}
			}
		}
		if m.stepIdx >= len(m.schedule) {
			m.stopPlayback()
			m.refresh()
			return m, nil
		}
		step := m.schedule[m.stepIdx]
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
			m.errMsg = ""
			return m.setSyncPoint()
		}
		m.errMsg = "Sync bar needs a real recording: play with an audio source (a + Space), then press s here"
		m.refresh()
	case "S":
		if len(m.syncPoints) > 0 {
			m.syncPoints = nil
			m.saveSyncPoints()
			m.errMsg = ""
			m.refresh()
		} else {
			m.errMsg = "No sync points to clear"
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
	case "[", "{", "]", "}", ",", ".", "o":
		return m.adjustAudioOffset(msg.String())
	case "esc":
		if m.jumpBuffer != "" {
			m.jumpBuffer = ""
			return m, nil
		}
		if m.errMsg != "" || m.infoMsg != "" {
			m.errMsg = ""
			m.infoMsg = ""
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
// with a real recording, and persists it under the current source's key.
func (m ViewerModel) adjustAudioOffset(key string) (ViewerModel, tea.Cmd) {
	switch key {
	case "[":
		m.audioOffset -= 0.5
	case ",":
		m.audioOffset -= 0.1
	case "{":
		m.audioOffset -= 5
	case "]":
		m.audioOffset += 0.5
	case ".":
		m.audioOffset += 0.1
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
	offset := strconv.FormatFloat(m.audioOffset, 'f', 1, 64)
	m.tab.Metadata[m.audioOffsetKey()] = offset
	m.tab.Metadata[model.MetaKeyAudioOffset] = offset
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
	}
	m.syncPoints = append(m.syncPoints, player.SyncPoint{Bar: bar, Seconds: elapsed.Seconds()})
	m.saveSyncPoints()
	if m.tab != nil && m.tab.Metadata != nil {
		m.tab.Metadata[m.audioOffsetKey()] = strconv.FormatFloat(m.audioOffset, 'f', 1, 64)
		m.tab.Metadata[model.MetaKeyAudioOffset] = strconv.FormatFloat(m.audioOffset, 'f', 1, 64)
	}
	m.refresh()
	return m, m.saveTabPrefsCmd()
}

// saveSyncPoints persists the anchors (plus the bar-1 offset anchor) to the
// current source's metadata key (mirroring the legacy key) as JSON.
func (m *ViewerModel) saveSyncPoints() {
	if m.tab == nil || m.tab.Metadata == nil {
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
		m.tab.Metadata[m.syncPointsKey()] = ""
		m.tab.Metadata[model.MetaKeySyncPoints] = ""
		return
	}
	data, err := json.Marshal(out)
	if err != nil {
		return
	}
	m.tab.Metadata[m.syncPointsKey()] = string(data)
	m.tab.Metadata[model.MetaKeySyncPoints] = string(data)
}

// loopStartTime returns the audio file position of the loop start bar:
// schedule time (0-based bar, converted from the user-facing 1-based bar)
// plus the calibrated intro offset.
func (m ViewerModel) loopStartTime() time.Duration {
	if len(m.schedule) == 0 {
		return 0
	}
	return player.ScheduleTimeAtBar(m.schedule, m.loopStartBar-1, m.bpm) + m.audioOffsetDur()
}

// syncPointsZeroBased converts persisted sync points (user-facing 1-based
// bars) to the schedule's 0-based bar indices for time mapping.
func syncPointsZeroBased(points []player.SyncPoint) []player.SyncPoint {
	out := make([]player.SyncPoint, 0, len(points))
	for _, p := range points {
		if p.Bar > 0 {
			p.Bar--
		}
		out = append(out, p)
	}
	return out
}

// audioOffsetDur returns the calibrated intro offset as a duration.
func (m ViewerModel) audioOffsetDur() time.Duration {
	return time.Duration(m.audioOffset * float64(time.Second))
}

// tempoMap returns the low→high BPM range spanned by the per-segment tempi
// derived from the sync anchors, when at least two anchors exist and the
// tempo actually varies between them.
func (m ViewerModel) tempoMap() ([2]int, bool) {
	if len(m.syncPoints) < 2 || len(m.schedule) == 0 {
		return [2]int{}, false
	}
	points := syncPointsZeroBased(m.syncPoints)
	low, high := 0, 0
	for i := 0; i+1 < len(points); i++ {
		b := player.SegmentBPM(m.schedule, points[i], points[i+1])
		if b <= 0 {
			return [2]int{}, false
		}
		if low == 0 || b < low {
			low = b
		}
		if b > high {
			high = b
		}
	}
	if low <= 0 || high <= low {
		return [2]int{}, false
	}
	return [2]int{low, high}, true
}

// syncQuality returns the RMS drift (seconds) between the sync anchors and
// the tempo implied by the first segment, when at least two anchors exist.
// Each later anchor is predicted from the first segment's tempo; the error
// between prediction and reality is the drift the anchor mapping corrects.
func (m ViewerModel) syncQuality() (float64, bool) {
	if len(m.syncPoints) < 2 || len(m.schedule) == 0 {
		return 0, false
	}
	points := syncPointsZeroBased(m.syncPoints)
	baseBPM := player.SegmentBPM(m.schedule, points[0], points[1])
	if baseBPM <= 0 {
		return 0, false
	}
	var sum float64
	n := 0
	for i := 2; i < len(points); i++ {
		ticks := player.TicksBetweenBars(m.schedule, points[0].Bar, points[i].Bar)
		predicted := points[0].Seconds + player.TicksToSeconds(ticks, baseBPM)
		err := predicted - points[i].Seconds
		sum += err * err
		n++
	}
	if n == 0 {
		return 0, false
	}
	return math.Sqrt(sum / float64(n)), true
}

// setLoopPoint registers the A (start) or B (end) loop boundary at the current
// bar and re-arms the engine region. The engine region is also re-armed at
// playback start so loops set while paused work from the first pass.
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
	m.applyLoopRegion()
	m.refresh()
	return m, nil
}

// applyLoopRegion maps the stored A-B bars (1-based, inclusive) to engine
// loop times: schedule time at the half-open 0-based range plus the calibrated
// intro offset. With no schedule yet (paused before first play) the region is
// left to PlaybackStartedMsg to arm.
func (m *ViewerModel) applyLoopRegion() {
	if m.loopStartBar <= 0 || m.loopEndBar <= 0 {
		m.engine.SetLoop(0, 0)
		return
	}
	if len(m.schedule) == 0 {
		return
	}
	start := player.ScheduleTimeAtBar(m.schedule, m.loopStartBar-1, m.bpm) + m.audioOffsetDur()
	end := player.ScheduleTimeAtBar(m.schedule, m.loopEndBar, m.bpm) + m.audioOffsetDur()
	if end > start {
		m.engine.SetLoop(start, end)
	}
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
	m.infoMsg = ""
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
	target := m.cursorBarLineOffset()
	if target < m.viewport.YOffset {
		m.viewport.SetYOffset(target)
	} else if target >= m.viewport.YOffset+m.viewport.Height {
		m.viewport.SetYOffset(target - m.viewport.Height + 1)
	}
}

// cursorBarLineOffset returns the content line where the cursor bar begins,
// using the active layout's own row math (grid rows vs linear blocks) so
// follow-scroll lands exactly on the playhead.
func (m *ViewerModel) cursorBarLineOffset() int {
	if m.tab == nil || m.cursorBar < 0 {
		return 0
	}
	var offsets []int
	if m.linear {
		offsets = kit.LinearBarLineOffsets(m.tab)
	} else {
		offsets = kit.GridBarLineOffsets(m.tab, m.viewport.Width)
	}
	if m.cursorBar >= len(offsets) {
		return 0
	}
	return offsets[m.cursorBar]
}


// View is part of the tea.Model interface. The tab viewer separates the title
// (primary), the status row (secondary), and the tab body (content): the
// status indicators no longer pile onto a single title line.
func (m ViewerModel) View() string {
	crumb := kit.FormatBreadcrumb("home", "library", "viewer")
	var title, status string
	if m.tab != nil {
		crumb = kit.FormatBreadcrumb("home", "library", m.tab.Title)
		title = kit.PanelTitleStyle.Render(m.tab.Title)
		if m.tab.Artist != "" {
			title += kit.MutedStyle.Render("  — " + m.tab.Artist)
		}
		if b := strings.TrimSpace(m.tab.Metadata[model.MetaKeySourceBadge]); b != "" {
			title += kit.MutedStyle.Render("  " + b)
		}
		status = kit.MutedStyle.Render(fmt.Sprintf("%s · bar %d/%d · %d BPM",
			m.tab.Tuning.Label(), m.cursorBar+1, len(m.tab.Bars), m.bpm))
		if m.playing {
			label := m.engine.ActiveDriver
			if m.engine.Mode() == "audio" {
				if label == "" {
					label = "audio"
				}
				status += "  " + kit.SuccessStyle.Render("▶ " + label)
			} else if label != "" {
				status += "  " + kit.SuccessStyle.Render("▶ midi:" + label)
			} else {
				status += "  " + kit.SuccessStyle.Render("▶ midi")
			}
		}
		if src := m.selectedSource(); !m.playing {
			if m.fetchingCatalog || m.fetchingAudio {
				status += kit.MutedStyle.Render("  … finding audio")
			} else if src.Kind == player.SourceMIDI {
				status += kit.MutedStyle.Render("  ♪ midi")
			} else if m.resolvedAudio != "" {
				status += kit.MutedStyle.Render("  ♪ " + filepath.Base(m.resolvedAudio))
			}
		}
		if m.audioOffset != 0 {
			status += kit.MutedStyle.Render(fmt.Sprintf("  ↔ %+.1fs", m.audioOffset))
		}
		if len(m.syncPoints) > 0 {
			status += kit.MutedStyle.Render(fmt.Sprintf("  ⚓%d", len(m.syncPoints)))
		}
		if q, ok := m.syncQuality(); ok {
			status += kit.InfoStyle.Render(fmt.Sprintf("  ±%.2fs", q))
		}
		if map_, ok := m.tempoMap(); ok {
			status += kit.InfoStyle.Render(fmt.Sprintf("  %d→%d bpm", map_[0], map_[1]))
		}
		if m.loopStartBar > 0 && m.loopEndBar > 0 {
			status += kit.MutedStyle.Render(fmt.Sprintf("  ↻ %d-%d", m.loopStartBar, m.loopEndBar))
		}
		if rate := m.engine.Rate(); rate != 1 {
			status += kit.MutedStyle.Render(fmt.Sprintf("  ⏩ ×%.2f", rate))
		}
	}
	if m.jumpBuffer != "" {
		status += kit.MutedStyle.Render("  [jump " + m.jumpBuffer + "]")
	}
	if m.infoMsg != "" {
		status += "  " + kit.InfoStyle.Render(kit.Truncate(m.infoMsg, 48))
	}
	if m.errMsg != "" {
		status += "  " + kit.ErrorStyle.Render("⚠ " + kit.Truncate(m.errMsg, 48))
	}
	status = kit.Truncate(status, m.width-8)

	body := "\n"
	if m.tab != nil {
		body += title + "\n"
		if status != "" {
			body += status + "\n"
		}
		body += kit.RenderDivider(m.width-4) + "\n\n"
		body += kit.RenderPanel(m.width-2, "", m.viewport.View())
	} else {
		body += kit.RenderPanel(m.width-2, "Tab", m.viewport.View())
	}
	if m.showAudioPicker {
		body += RenderAudioPicker(m.width, m.audioCatalog, m.audioCursor, m.fetchingCatalog, m.strictAudio, m.recommendedSourceIdx())
	}

	playLabel := "play"
	if m.playing {
		playLabel = "pause"
	}
	statusLine := ""
	if m.tab != nil && m.tab.Title != "" {
		statusLine = fmt.Sprintf("♪ %s", kit.Truncate(m.tab.Title, 24))
	}
	footer := kit.RenderFooterWithStatus(m.width, statusLine, []kit.KeyHint{
		{Key: "a", Label: "audio"},
		{Key: "Space/p", Label: playLabel},
		{Key: "+/-", Label: "BPM"},
		{Key: "> <", Label: "speed"},
		{Key: "[ ] , .", Label: "sync"},
		{Key: "o", Label: "reset"},
		{Key: "s", Label: "sync bar"},
		{Key: "i/u", Label: "loop"},
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

// pickStrictAudioSourceIndex is the studio-lock variant: local files first,
// then the best-scoring candidate that passes strict selection (official or
// backing), then MIDI. Live shows, covers, and lessons are never auto-picked
// because they fight the tab's tempo and arrangement.
func pickStrictAudioSourceIndex(tab *model.Tab, cat player.AudioCatalog) int {
	if tab != nil && tab.Metadata != nil {
		if srcID := strings.TrimSpace(tab.Metadata["audio_source"]); srcID != "" {
			if found := cat.FindByID(srcID); found >= 0 {
				return found
			}
		}
	}
	best := -1
	for i, src := range cat.Sources {
		if src.Kind == player.SourceLocal && src.Path != "" && player.FileExists(src.Path) {
			return i
		}
		if !src.StrictOK {
			continue
		}
		if best < 0 || src.Score > cat.Sources[best].Score {
			best = i
		}
	}
	if best >= 0 {
		return best
	}
	return 0 // MIDI: safer than auto-downloading a mismatched recording
}

// autoPickIndex chooses the auto-play source for the current strict mode and
// notes when strict selection rejected every online candidate.
func (m *ViewerModel) autoPickIndex(cat player.AudioCatalog) int {
	if m.strictAudio {
		idx := pickStrictAudioSourceIndex(m.tab, cat)
		if idx == 0 && cat.HasStrictRejected() {
			m.infoMsg = "No studio/official match — open [a] to pick audio manually"
		}
		return idx
	}
	return pickAudioSourceIndex(m.tab, cat)
}

// SetStrictAudio toggles studio-lock audio selection (auto-pick and
// recommendation prefer official/backing recordings over live/cover/lesson).
func (m *ViewerModel) SetStrictAudio(v bool) {
	m.strictAudio = v
}

// recommendedSourceIdx returns the catalog index the picker should mark as
// the recommended pick for the current strict mode.
func (m ViewerModel) recommendedSourceIdx() int {
	if m.strictAudio {
		return pickStrictAudioSourceIndex(m.tab, m.audioCatalog)
	}
	return pickAudioSourceIndex(m.tab, m.audioCatalog)
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
		// Switching recordings means switching calibration: restore the new
		// source's intro offset and sync anchors.
		m.restoreCalibrationForSource()
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
		return m, tea.Batch(m.saveTabPrefsCmd(), m.maybeDetectIntroCmd())
	}
	return m, nil
}

// maybeDetectIntroCmd returns a command that probes the selected audio file's
// leading silence for an auto intro offset — but only when the file exists,
// no manual calibration exists yet, and the probe hasn't run for this source.
func (m *ViewerModel) maybeDetectIntroCmd() tea.Cmd {
	if m.tab == nil || m.audioOffset != 0 || len(m.syncPoints) > 0 {
		return nil
	}
	src := m.selectedSource()
	if src.Kind == player.SourceMIDI {
		return nil
	}
	path := src.Path
	if path == "" || !player.FileExists(path) {
		return nil
	}
	if m.tab.Metadata != nil {
		if m.tab.Metadata["audio_offset_auto:"+src.ID] == "1" || m.tab.Metadata["audio_offset_auto"] == "1" {
			return nil
		}
	}
	srcID := src.ID
	tab, tabID, tabPath := m.tab, m.tabID, m.tabPath
	return func() tea.Msg {
		offset, err := player.LeadingSilence(path)
		if err != nil {
			return msgs.IntroDetectedMsg{SourceID: srcID, Err: err, Artist: tab.Artist, Title: tab.Title, TabID: tabID, TabPath: tabPath}
		}
		return msgs.IntroDetectedMsg{SourceID: srcID, Offset: offset, Artist: tab.Artist, Title: tab.Title, TabID: tabID, TabPath: tabPath}
	}
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
		m.selectedSourceIdx = m.autoPickIndex(cat)
		if m.selectedSourceIdx >= len(m.audioCatalog.Sources) {
			m.selectedSourceIdx = 0
		}
		m.audioCursor = m.selectedSourceIdx
		m.restoreCalibrationForSource()
		m.applySelectedSource(true)
	}
	if !allowOnline || !player.OnlineAudioAvailable() || player.AudioSearchQuery(m.tab) == "" {
		return m.maybeDetectIntroCmd()
	}
	m.fetchingCatalog = true
	return tea.Batch(fetchAudioCatalogCmd(m.tab, m.tabPath, m.tabID, m.audioDirs, allowOnline), m.maybeDetectIntroCmd())
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
