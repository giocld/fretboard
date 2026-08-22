package viewer

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"fretboard/internal/model"
	"fretboard/internal/parser"
	"fretboard/internal/player"
	"fretboard/internal/ui/kit"
	"fretboard/internal/ui/msgs"
	tea "github.com/charmbracelet/bubbletea"
)

// verifyTickMsg drives the passive sync-verification countdown (S4.1): the
// chip's auto-keep deadline is advanced on the UI tick, never a timer.
type verifyTickMsg struct{}

// verifyAutoKeepAfter is how long a pending auto-alignment verification waits
// before the anchors are accepted silently (playback never blocks on it).
const verifyAutoKeepAfter = 30 * time.Second

func verifyTickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return verifyTickMsg{} })
}

// handleVerifyTick advances the pending verification session. While pending
// the chip re-renders with the fresh countdown; once kept (auto or manual)
// the chip is removed and the tick stops.
func (m ViewerModel) handleVerifyTick(verifyTickMsg) (ViewerModel, tea.Cmd) {
	if m.verify == nil {
		return m, nil
	}
	state := m.verify.AutoKeepIfElapsed(time.Now())
	if state == player.VerifyPending {
		m.refresh()
		return m, verifyTickCmd()
	}
	// Kept, refined, or downgraded: the chip's job is done either way.
	m.verify = nil
	m.refresh()
	return m, nil
}

// startVerifySession opens the passive verification chip after an auto
// alignment lands: the anchors are kept automatically after 30s, or the
// user can press s during the pending window to refine manually.
func (m *ViewerModel) startVerifySession(anchors int, drift time.Duration) {
	if anchors < 2 {
		return // too few anchors to verify meaningfully
	}
	m.verify = player.NewVerifySession(anchors, drift, verifyAutoKeepAfter)
}

// ---- shared download progress (S3.3) ----

// downloadState is the goroutine-safe progress slot for an in-flight audio
// download: the downloader goroutine writes into it, View() reads it. The
// pointer is shared across ViewerModel copies, so the value survives the
// value-receiver Update loop.
type downloadState struct {
	mu      sync.Mutex
	active  bool
	percent float64
}

func (d *downloadState) set(p float64) {
	d.mu.Lock()
	d.percent = p
	d.mu.Unlock()
}

func (d *downloadState) get() (bool, float64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.active, d.percent
}

func (d *downloadState) begin() {
	d.mu.Lock()
	d.active = true
	d.percent = 0
	d.mu.Unlock()
	// The hook is a package global; the viewer is the only download driver,
	// so registering on begin and clearing on end cannot clobber another
	// download (fetchingAudio serializes them).
	player.OnDownloadProgress = func(p float64) { d.set(p) }
}

func (d *downloadState) end() {
	d.mu.Lock()
	d.active = false
	d.mu.Unlock()
	player.OnDownloadProgress = nil
}

// downloadState lazily allocates the shared progress slot on the model.
func (m *ViewerModel) downloadState() *downloadState {
	if m.download == nil {
		m.download = &downloadState{}
	}
	return m.download
}

// ---- audio cache screen (S3.3) ----

// cacheEntry is one row of the cache screen: name, absolute path, size,
// and last-modified time (the LRU play-time proxy).
type cacheEntry struct {
	name string
	path string
	size int64
	mod  time.Time
}

// openCacheScreen lists the audio cache and opens the K overlay.
func (m *ViewerModel) openCacheScreen() {
	m.showCache = true
	m.cacheCur = 0
	m.loadCacheList()
	m.errMsg = ""
	m.refresh()
}

// loadCacheList re-reads the cache directory into the screen's rows. The
// directory comes from CacheStats so the viewer never hard-codes it.
func (m *ViewerModel) loadCacheList() {
	m.cacheRows = nil
	_, _, dir := player.CacheStats()
	if dir == "" {
		return
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, ent := range ents {
		if ent.IsDir() {
			continue
		}
		info, err := ent.Info()
		if err != nil {
			continue
		}
		m.cacheRows = append(m.cacheRows, cacheEntry{
			name: ent.Name(),
			path: filepath.Join(dir, ent.Name()),
			size: info.Size(),
			mod:  info.ModTime(),
		})
	}
}

// handleCacheKey drives the cache overlay: j/k move, d deletes one entry,
// D deletes everything (EvictLRU with a 0-byte keep), r re-lists, Esc
// closes. Deletions are quick local file ops, safe in the Update loop.
func (m ViewerModel) handleCacheKey(msg tea.KeyMsg) (ViewerModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.showCache = false
		m.refresh()
		return m, nil
	case "j", "down":
		if m.cacheCur < len(m.cacheRows)-1 {
			m.cacheCur++
		}
		return m, nil
	case "k", "up":
		if m.cacheCur > 0 {
			m.cacheCur--
		}
		return m, nil
	case "r":
		m.loadCacheList()
		m.refresh()
		return m, nil
	case "d":
		if m.cacheCur < 0 || m.cacheCur >= len(m.cacheRows) {
			return m, nil
		}
		entry := m.cacheRows[m.cacheCur]
		if err := os.Remove(entry.path); err != nil {
			m.errMsg = "Delete failed: " + err.Error()
		} else {
			m.infoMsg = "Deleted " + entry.name
			m.loadCacheList()
			m.cacheCur = min(m.cacheCur, len(m.cacheRows)-1)
			m.cacheCur = max(m.cacheCur, 0)
		}
		m.refresh()
		return m, nil
	case "D":
		if _, err := player.EvictLRU(0); err != nil {
			m.errMsg = "Clear failed: " + err.Error()
		} else {
			m.infoMsg = "Audio cache cleared"
			m.loadCacheList()
			m.cacheCur = 0
		}
		m.refresh()
		return m, nil
	}
	return m, nil
}

// renderCacheScreen draws the cache overlay: per-entry rows, the total
// versus the cap, and the delete keys.
func renderCacheScreen(m ViewerModel) string {
	entries, total, dir := player.CacheStats()
	capBytes := int64(player.AudioCacheMaxGB) << 30
	var lines []string
	if dir == "" {
		lines = append(lines, kit.MutedStyle.Render("Audio cache unavailable"))
	} else if len(m.cacheRows) == 0 {
		lines = append(lines, kit.MutedStyle.Render("Cache is empty"))
	}
	for i, e := range m.cacheRows {
		prefix := "  "
		if i == m.cacheCur {
			prefix = "▸ "
		}
		line := fmt.Sprintf("%s%s  %10s  %s", prefix, kit.Truncate(e.name, 40), humanBytes(e.size), e.mod.Format("2006-01-02 15:04"))
		if i == m.cacheCur {
			line = kit.ListSelected.Render(line)
		} else {
			line = kit.ListNormal.Render(line)
		}
		lines = append(lines, line)
	}
	lines = append(lines, "")
	lines = append(lines, kit.MutedStyle.Render(fmt.Sprintf("%d entries · %s of %s cap", entries, humanBytes(total), humanBytes(capBytes))))
	lines = append(lines, kit.MutedStyle.Render("j/k move  d delete  D delete all  r refresh  Esc close"))
	return "\n" + kit.RenderPanel(m.width-2, "Audio cache", strings.Join(lines, "\n"))
}

// humanBytes renders a byte count compactly (e.g. "4.5 MiB").
func humanBytes(n int64) string {
	if n < 0 {
		n = 0
	}
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

// ---- chord-sheet mode (S1.2) ----

// chordWordRe splits a chord sheet into whitespace runs so each chord-shaped
// token can be transposed independently without disturbing spacing.
var chordWordRe = regexp.MustCompile(`\S+`)

// chordText returns the raw chord-sheet text with the current transpose
// applied to every chord-shaped token (non-chord text passes through
// unchanged; parser.TransposeChord already leaves unparseable words alone).
func (m ViewerModel) chordText() string {
	raw := ""
	if m.tab != nil && m.tab.Metadata != nil {
		raw = m.tab.Metadata["raw"]
	}
	return transposeChordSheetText(raw, m.transpose)
}

func transposeChordSheetText(raw string, semitones int) string {
	if semitones%12 == 0 || raw == "" {
		return raw
	}
	return chordWordRe.ReplaceAllStringFunc(raw, func(w string) string {
		return parser.TransposeChord(w, semitones)
	})
}

// ---- quality badge (S1.1) ----

// qualityTiming returns the timing word ("solid"|"approximate"|"sloppy")
// for a tab-kind sheet, or "" when the metadata is missing (no badge).
func (m ViewerModel) qualityTiming() string {
	if m.tab == nil || m.tab.Metadata == nil {
		return ""
	}
	if m.tab.Metadata["kind"] != "tab" {
		return ""
	}
	return strings.TrimSpace(m.tab.Metadata["quality_timing"])
}

// ---- tempo provenance (S2.1) ----

// rememberedTempo reads the per-tab value a previous session recorded, the
// "remembered" rung of the resolution chain.
func rememberedTempo(tab *model.Tab) int {
	if tab == nil || tab.Metadata == nil {
		return 0
	}
	if n, err := strconv.Atoi(strings.TrimSpace(tab.Metadata[model.MetaKeyTempo])); err == nil && n > 0 {
		return n
	}
	return 0
}

// resolvePlaybackTempo walks the provenance chain for the current tab and
// applies the winning BPM unless the user nudged the tempo manually. The
// provenance label is stored for the playback status line.
func (m *ViewerModel) resolvePlaybackTempo() {
	if m.tab == nil {
		return
	}
	bpm, src := player.ResolveTempo(m.tab, m.derivedBPM, rememberedTempo(m.tab))
	m.tempoSrc, m.tempoSrcSet = src, true
	if !m.manualBPM && bpm != m.bpm {
		m.bpm = bpm
	}
}

// ---- calibration chip (S4.3) ----

// calibration state kinds, driving the chip color: gray uncalibrated, green
// verified, amber pending.
const (
	calibUncalibrated = iota
	calibPending
	calibVerified
)

// calibrationAudioLoaded reports whether a real recording is selected and
// its file is usable — the chip only renders for actual audio, not MIDI.
func (m ViewerModel) calibrationAudioLoaded() bool {
	src := m.selectedSource()
	if src.Kind == player.SourceMIDI {
		return false
	}
	if m.resolvedAudio != "" && player.FileExists(m.resolvedAudio) {
		return true
	}
	return src.Path != "" && player.FileExists(src.Path)
}

// calibrationKind resolves the chip color state from the verify session
// first (pending wins over any stored calibration), then falls back to the
// persisted calibration.
func (m ViewerModel) calibrationKind() int {
	if m.verify != nil {
		switch m.verify.State {
		case player.VerifyPending:
			return calibPending
		case player.VerifyKept:
			return calibVerified
		}
		return calibUncalibrated
	}
	if m.audioOffset != 0 || len(m.syncPoints) > 0 || m.autoActive {
		return calibVerified
	}
	return calibUncalibrated
}

// calibrationWord is the human state word inside the chip.
func (m ViewerModel) calibrationWord() string {
	switch m.calibrationKind() {
	case calibPending:
		return "pending"
	case calibVerified:
		if m.verify != nil {
			return "verified"
		}
		return "calibrated"
	default:
		return "uncalibrated"
	}
}

// calibrationChip renders the compact sync chip for the current source:
// offset, anchor count, drift, and the color-coded calibration word. Empty
// when no audio is loaded.
func (m ViewerModel) calibrationChip() string {
	if m.tab == nil || !m.calibrationAudioLoaded() {
		return ""
	}
	driftText := "±?"
	if q, ok := m.syncQuality(); ok {
		driftText = fmt.Sprintf("±%.1fs", q)
	}
	text := fmt.Sprintf("[sync: %+.1fs · %d anchors · drift %s · %s]",
		m.audioOffset, len(m.syncPoints), driftText, m.calibrationWord())
	switch m.calibrationKind() {
	case calibPending:
		return kit.WarningStyle.Render(text)
	case calibVerified:
		return kit.SuccessStyle.Render(text)
	default:
		return kit.MutedStyle.Render(text)
	}
}

// ---- GP multi-track switcher (S5.3b) ----

// gpTrackMeta is one entry of the "tracks" metadata array the app writes at
// Guitar Pro import: identity only, the actual tabs come from re-parsing
// the GP file on switch.
type gpTrackMeta struct {
	Name       string `json:"name"`
	Instrument string `json:"instrument"`
	Strings    int    `json:"strings"`
	Tuning     string `json:"tuning"`
}

// loadGPTrackMeta parses the "tracks" metadata key (AppShell writes it at
// GP import) into the switcher state. A missing or malformed key leaves the
// tab single-track.
func (m *ViewerModel) loadGPTrackMeta() {
	m.gpTracks = nil
	m.gpTrackIdx = 0
	if m.tab == nil || m.tab.Metadata == nil {
		return
	}
	raw := strings.TrimSpace(m.tab.Metadata["tracks"])
	if raw == "" {
		return
	}
	var tracks []gpTrackMeta
	if err := json.Unmarshal([]byte(raw), &tracks); err != nil || len(tracks) == 0 {
		return
	}
	m.gpTracks = tracks
}

// cycleGPTrack switches to the next GP track: it re-parses the GP file for
// that track's real tab and keeps the loaded title/artist and metadata.
func (m ViewerModel) cycleGPTrack() (ViewerModel, tea.Cmd) {
	if m.tab == nil || len(m.gpTracks) <= 1 {
		m.errMsg = "No multi-track Guitar Pro file loaded"
		m.refresh()
		return m, nil
	}
	if m.playing {
		m.stopPlayback()
	}
	next := (m.gpTrackIdx + 1) % len(m.gpTracks)
	tracks, err := parser.ParseGuitarProTracks(m.tabPath)
	if err != nil {
		m.errMsg = "Cannot reload Guitar Pro tracks: " + err.Error()
		m.refresh()
		return m, nil
	}
	if next >= len(tracks) || tracks[next].Tab == nil {
		m.errMsg = "Track data unavailable (gp-parser returned metadata only)"
		m.refresh()
		return m, nil
	}
	newTab := tracks[next].Tab
	newTab.Title = m.tab.Title
	newTab.Artist = m.tab.Artist
	if m.tab.Metadata != nil {
		if newTab.Metadata == nil {
			newTab.Metadata = map[string]string{}
		}
		for k, v := range m.tab.Metadata {
			newTab.Metadata[k] = v
		}
	}
	m.LoadTab(newTab, m.tabPath, m.tabID)
	m.gpTrackIdx = next
	name := m.gpTracks[next].Name
	if name == "" {
		name = fmt.Sprintf("track %d", next+1)
	}
	inst := m.gpTracks[next].Instrument
	m.infoMsg = fmt.Sprintf("Track %d/%d: %s (%s)", next+1, len(m.gpTracks), name, inst)
	m.refresh()
	return m, nil
}

// currentTrackName returns the display name of the active GP track.
func (m ViewerModel) currentTrackName() string {
	if len(m.gpTracks) <= 1 {
		return ""
	}
	name := m.gpTracks[m.gpTrackIdx].Name
	if name == "" {
		name = fmt.Sprintf("track %d", m.gpTrackIdx+1)
	}
	return name
}

// ---- drums (S5.3a) ----

// isDrums reports whether the loaded tab is a drum part (player-side
// detection, cached at load). Drum tabs get a header label and lose
// transpose/tuning edits.
func (m ViewerModel) isDrums() bool { return m.drums }

// ---- practice session (S8.1) ----

// sessionRampLoops is how many clean loop passes trigger a +5% tempo ramp.
const sessionRampLoops = 3

// rampStepBPM is the +5% ramp step, rounded to a 5-BPM granularity so it
// matches the +/- keys' step feel.
func rampStepBPM(bpm int) int {
	step := int(math.Round(float64(bpm) * 0.05))
	if step < 5 {
		step = 5
	}
	return step
}

// startSession enters practice-session mode: the A-B loop repeats, tempo
// ramps +5% every sessionRampLoops clean loops toward the target (set with
// +/- while paused; defaults to the starting tempo = no ramp).
func (m *ViewerModel) startSession() {
	m.sessionMode = true
	m.sessionLoops = 0
	m.sessionClean = 0
	m.sessionBaseBPM = m.bpm
	m.sessionTargetBPM = m.bpm
	m.sessionStart = time.Now()
	m.sessionRamp = false
	m.sessionCard = ""
	m.errMsg = ""
	m.infoMsg = fmt.Sprintf("Session: loop %d-%d · %d bpm — +/- while paused sets the target, M exits",
		m.loopStartBar, m.loopEndBar, m.bpm)
	m.refresh()
}

// endSessionCmd ends the session, shows the summary card, and emits
// msgs.PracticeSessionMsg so the app records the practice (the viewer has
// no library handle).
func (m ViewerModel) endSessionCmd() (ViewerModel, tea.Cmd) {
	if m.playing {
		m.stopPlayback()
	}
	dur := int64(0)
	if !m.sessionStart.IsZero() {
		dur = int64(time.Since(m.sessionStart) / time.Second)
	}
	loops := m.sessionLoops
	card := fmt.Sprintf("%d loops · %d→%d bpm · %d min", loops, m.sessionBaseBPM, m.bpm, dur/60)
	m.sessionCard = card
	m.sessionMode = false
	m.sessionLoops = 0
	m.sessionClean = 0
	m.sessionStart = time.Time{}
	m.sessionRamp = false
	m.infoMsg = card
	m.errMsg = ""
	m.refresh()
	if loops > 0 && m.tabID > 0 {
		msg := msgs.PracticeSessionMsg{TabID: m.tabID, DurationSec: dur, TempoBPM: m.bpm, Loops: loops}
		return m, func() tea.Msg { return msg }
	}
	return m, nil
}

// noteLoopPass counts one completed A-B loop pass. On the configured ramp
// cadence it steps the tempo toward the target: MIDI re-bases the deadline
// clock seamlessly; audio marks the session for a restart (the monitor
// performs it like the +/- keys do).
func (m *ViewerModel) noteLoopPass() {
	if !m.sessionMode {
		return
	}
	m.sessionLoops++
	m.sessionClean++
	if m.sessionTargetBPM <= m.sessionBaseBPM || m.bpm >= m.sessionTargetBPM {
		return
	}
	if m.sessionClean < sessionRampLoops {
		return
	}
	m.sessionClean = 0
	next := m.bpm + rampStepBPM(m.bpm)
	if next > m.sessionTargetBPM {
		next = m.sessionTargetBPM
	}
	if next == m.bpm {
		return
	}
	m.bpm = next
	if m.playing && m.tab != nil {
		if m.audioSync {
			// Audio cannot re-time mid-file: restart the player at the new
			// tempo (same path as +/-), flagged for the monitor to run.
			m.sessionRamp = true
			_ = m.engine.Stop()
			m.resetPlayback()
		} else {
			m.stepClock.Rebase(stepDur(m.schedule[m.stepIdx].Ticks, m.bpm))
		}
	}
}

// ---- contextual footer hints (S8.3) ----

// footerHints returns the 4-5 most relevant keys for the current viewer
// state: session mode, calibration in flight, playing, chord sheet, or the
// idle practice view. q quit is always the last hint so the truncation
// logic keeps it visible.
func (m ViewerModel) footerHints() []kit.KeyHint {
	quit := kit.KeyHint{Key: "q", Label: "quit"}
	if m.tab == nil {
		return []kit.KeyHint{quit}
	}
	if m.sessionMode {
		return []kit.KeyHint{
			{Key: "M", Label: "exit"},
			{Key: "+/-", Label: "target"},
			{Key: "Space", Label: "pause"},
			{Key: "m", Label: "metronome"},
			{Key: "C", Label: "count-in"},
			quit,
		}
	}
	if m.calibrating {
		return []kit.KeyHint{
			{Key: "s", Label: "sync bar"},
			{Key: "S", Label: "undo"},
			{Key: "[ ]", Label: "offset"},
			{Key: "o", Label: "reset"},
			{Key: "w", Label: "realign"},
			quit,
		}
	}
	if m.playing {
		return []kit.KeyHint{
			{Key: "Space/p", Label: "pause"},
			{Key: "+/-", Label: "BPM"},
			{Key: "s", Label: "sync bar"},
			{Key: "m", Label: "metronome"},
			{Key: "f7", Label: "+15s"},
			quit,
		}
	}
	if m.chordSheet {
		return []kit.KeyHint{
			{Key: "T/Z", Label: "transpose"},
			{Key: "E/$", Label: "edit"},
			{Key: "ctrl+p", Label: "print"},
			{Key: "g", Label: "open"},
			quit,
		}
	}
	hints := []kit.KeyHint{
		{Key: "a", Label: "audio"},
		{Key: "Space/p", Label: "play"},
		{Key: "+/-", Label: "BPM"},
		{Key: "s", Label: "sync"},
		{Key: "i/u", Label: "loop"},
	}
	if len(m.gpTracks) > 1 {
		hints = append(hints, kit.KeyHint{Key: "t", Label: "track"})
	}
	hints = append(hints, quit)
	return hints
}
