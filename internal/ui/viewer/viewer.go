package viewer

import (
	"time"

	"fretboard/internal/model"
	"fretboard/internal/player"
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
	// Practice tools (realtime MIDI).
	metronome    bool
	countIn      int // 0/1/2 bars of lead-in clicks
	program      int // GM program for MIDI playback (0 = default steel)
	loopStartBar int
	loopEndBar   int
	follow       bool
	linear       bool
	// Reading tools.
	transpose     int // ±semitones, session-only (0 = as written)
	showNotes     bool
	searchActive  bool
	searchInput   string
	searchMatches []searchMatch
	searchIdx     int
	// Practice tools.
	perfMode      bool      // performance mode: hide the tab, show the section + progress
	practiceStart time.Time // when the current playback session started
	practiceSecs  int64     // practice seconds accumulated this session
	// MIDI deadline clock: absolute step deadlines so the beat never drifts.
	stepClock player.StepClock
	driftMs   int64 // measured lateness of the last tick (ms, 0 = on time)
	// Auto-alignment: which sources have been aligned this session.
	alignedSources map[string]bool
	// Auto tempo map + drift meter (measured bar anchors and onsets).
	autoAnchors   []player.SyncPoint
	autoOnsets    []time.Duration
	autoStrengths []float64 // normalized onset strengths, aligned with autoOnsets
	autoActive    bool
	syncDrift     float64 // seconds the playhead is off the nearest onset (0 = on)
	// Undo support.
	prevOffset float64 // offset value before the last `o` reset
	manualPick bool    // user chose the source manually; keep it across refreshes

	cursorBar  int
	cursorCol  int
	panOffset  int
	playing    bool
	schedule   []player.PlaybackStep
	stepIdx    int
	tickDur    time.Duration
	resumePos  time.Duration // audio position to resume at on the next play
	bpm        int
	jumpBuffer string
	lastKey    string
	lastKeyAt  time.Time
	width      int
	height     int
	errMsg     string
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
	m.resumePos = 0
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
	m.prevOffset = 0
	m.manualPick = false
	m.perfMode = false
	m.practiceStart = time.Time{}
	m.practiceSecs = 0
	m.alignedSources = map[string]bool{}
	m.autoAnchors = nil
	m.autoOnsets = nil
	m.autoStrengths = nil
	m.autoActive = false
	m.syncDrift = 0
	m.restoreCalibrationForSource()
	_ = m.engine.Stop()
	m.engine.SetLoop(0, 0)
	m.refresh()
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
	case tea.MouseMsg:
		// Wheel scrolls the viewer like j/k.
		switch msg.Type {
		case tea.MouseWheelUp:
			return m.handleKey(keyFromMouse("k"))
		case tea.MouseWheelDown:
			return m.handleKey(keyFromMouse("j"))
		}
	case msgs.AudioFetchedMsg:
		return m.handleAudioFetched(msg)
	case msgs.AudioCatalogMsg:
		return m.handleAudioCatalog(msg)
	case msgs.IntroDetectedMsg:
		return m.handleIntroDetected(msg)
	case msgs.BPMDerivedMsg:
		return m.handleBPMDerived(msg)
	case msgs.AlignmentMsg:
		return m.handleAlignment(msg)
	case msgs.PlaybackStartedMsg:
		return m.handlePlaybackStarted(msg)
	case msgs.PlaybackErrorMsg:
		return m.handlePlaybackError(msg)
	case msgs.PlaybackMonitorMsg:
		return m.handlePlaybackMonitor(msg)
	case msgs.PlaybackTickMsg:
		return m.handlePlaybackTick(msg)
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// SetAudioDirs configures extra directories to search for backing tracks.
func (m *ViewerModel) SetAudioDirs(dirs []string) {
	m.audioDirs = append([]string(nil), dirs...)
}

// SetStrictAudio toggles studio-lock audio selection (auto-pick and
// recommendation prefer official/backing recordings over live/cover/lesson).
func (m *ViewerModel) SetStrictAudio(v bool) {
	m.strictAudio = v
}

// StopPlayback halts any running audio.
func (m *ViewerModel) StopPlayback() { m.stopPlayback() }

// ShutdownAudio releases the synthesizer engine.
func (m *ViewerModel) ShutdownAudio() { m.engine.Shutdown() }

// SetVolume sets the synthesizer volume (0-100).
func (m *ViewerModel) SetVolume(v int) {
	v = min(max(v, 0), 100)
	m.engine.Volume = v
}

// SetSoundfont sets the soundfont used by the synthesizer.
func (m *ViewerModel) SetSoundfont(path string) { m.engine.Soundfont = path }

// SetError sets the viewer error banner.
func (m *ViewerModel) SetError(msg string) { m.errMsg = msg }

// SetCursorBar moves the cursor to a 0-based bar (clamped).
func (m *ViewerModel) SetCursorBar(bar int) {
	if m.tab == nil {
		return
	}
	bar = min(max(bar, 0), len(m.tab.Bars)-1)
	m.cursorBar = bar
	m.cursorCol = 0
	m.ensureCursorVisible()
	m.refresh()
}

// SetBPM sets the playback tempo (clamped to the playable range).
func (m *ViewerModel) SetBPM(bpm int) {
	m.bpm = player.ClampBPM(bpm)
	m.refresh()
}

// SetLinear sets the layout mode.
func (m *ViewerModel) SetLinear(linear bool) {
	m.linear = linear
	m.refresh()
}

// CursorBar returns the current 0-based cursor bar.
func (m ViewerModel) CursorBar() int { return m.cursorBar }

// BPM returns the current tempo.
func (m ViewerModel) BPM() int { return m.bpm }

// Linear reports the active layout mode.
func (m ViewerModel) Linear() bool { return m.linear }

// Tab returns the currently loaded tab, or nil.
func (m *ViewerModel) Tab() *model.Tab { return m.tab }

// TabPath returns the path of the currently loaded tab.
func (m *ViewerModel) TabPath() string { return m.tabPath }

// TabID returns the library id of the currently loaded tab.
func (m *ViewerModel) TabID() int64 { return m.tabID }

// SearchMatchesForTest exposes the current search matches for tests and
// external drivers.
func (m ViewerModel) SearchMatchesForTest() []searchMatch { return m.searchMatches }
