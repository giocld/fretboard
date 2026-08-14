package viewer

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"fretboard/internal/player"
	"fretboard/internal/ui/msgs"
	tea "github.com/charmbracelet/bubbletea"
)

// seekBy returns the clamped audio position d away from the current playback
// position, and whether a seek is possible at all. Seeking only applies to
// real audio playback: the position is clamped to the track length when it is
// known, otherwise just to zero. (The hyperplan "F7 = 15%" reading is
// impossible because SetRate clamps at 0.25, so F7 is a 15-second seek.)
func seekBy(m ViewerModel, d time.Duration) (time.Duration, bool) {
	if !m.playing || m.engine.Mode() != "audio" {
		return 0, false
	}
	pos := m.engine.Elapsed() + d
	if dur := m.engine.AudioDuration(); dur > 0 && pos > dur {
		pos = dur
	}
	if pos < 0 {
		pos = 0
	}
	return pos, true
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

// loopRestartPos returns the audio position at which an A-B loop restarts:
// the loop start bar warped through the same merged-anchor map the step
// mapping consumes, so a loop wraps on the recording's timeline instead of
// the tab's schedule@BPM line. With no anchors the TimeMapper degenerates to
// the naive formula, so the unanchored path falls back to loopStartTime()
// exactly.
func (m ViewerModel) loopRestartPos() time.Duration {
	if len(m.schedule) == 0 || m.loopStartBar <= 0 {
		return m.loopStartTime()
	}
	points := syncPointsZeroBased(player.MergeAnchors(m.syncPoints, m.autoAnchors))
	if len(points) == 0 {
		return m.loopStartTime()
	}
	tm := player.NewTimeMapper(m.schedule, points, m.bpm)
	tm.SetAudioOffset(m.audioOffsetDur())
	start, _ := tm.WarpedLoopTimes(m.loopStartBar-1, m.loopEndBar)
	return start
}

// resumeAudioPos maps the cursor's bar/col through the anchor warp to the
// audio position where a paused session must continue. Without a schedule
// there is nothing to map; without anchors the map degenerates to the naive
// schedule position (score at BPM plus the calibrated offset).
func (m ViewerModel) resumeAudioPos() time.Duration {
	if len(m.schedule) == 0 {
		return 0
	}
	tm := player.NewTimeMapper(m.schedule, syncPointsZeroBased(player.MergeAnchors(m.syncPoints, m.autoAnchors)), m.bpm)
	tm.SetAudioOffset(m.audioOffsetDur())
	return tm.ResumePos(m.cursorBar, m.cursorCol)
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
// loop times through the anchor warp: audio time of the half-open 0-based
// range. With anchors the TimeMapper stretches the region onto the
// recording's timeline; without anchors its naive path is exactly schedule
// time at the bars plus the calibrated intro offset. With no schedule yet
// (paused before first play) the region is left to PlaybackStartedMsg to arm.
func (m *ViewerModel) applyLoopRegion() {
	if m.loopStartBar <= 0 || m.loopEndBar <= 0 {
		m.engine.SetLoop(0, 0)
		return
	}
	if len(m.schedule) == 0 {
		return
	}
	tm := player.NewTimeMapper(m.schedule, syncPointsZeroBased(player.MergeAnchors(m.syncPoints, m.autoAnchors)), m.bpm)
	tm.SetAudioOffset(m.audioOffsetDur())
	start, end := tm.WarpedLoopTimes(m.loopStartBar-1, m.loopEndBar)
	if end > start {
		m.engine.SetLoop(start, end)
	}
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

// nextStepIndexFrom returns the index to play after the step at idx, with
// the A-B loop wrap applied.
func (m ViewerModel) nextStepIndexFrom(idx int) int {
	next := idx + 1
	if m.loopEndBar > 0 {
		atEnd := next >= len(m.schedule)
		beyondLoop := !atEnd && m.schedule[next].Bar >= m.loopEndBar
		if atEnd || beyondLoop {
			// Wrap to the loop start; atEnd also covers a loop whose end bar
			// is the tab's last bar (no later step can trigger it).
			for i, s := range m.schedule {
				if s.Bar >= m.loopStartBar-1 {
					return i
				}
			}
		}
	}
	return next
}

// stepDur converts ticks to the wall duration used by the deadline clock.
func stepDur(ticks, bpm int) time.Duration {
	return time.Duration(player.StepDuration(ticks, bpm)) * time.Millisecond
}

// stopPlayback halts audio, clears UI playback state, and banks the
// practice time of the session that just ended.
func (m *ViewerModel) stopPlayback() {
	if m.playing && !m.practiceStart.IsZero() {
		m.practiceSecs += int64(time.Since(m.practiceStart) / time.Second)
	}
	m.practiceStart = time.Time{}
	m.resetPlayback()
	_ = m.engine.Stop()
	m.persistPracticeTime()
}

// persistPracticeTime folds the session's practice seconds into the tab's
// practice_seconds metadata so the total survives restarts.
func (m *ViewerModel) persistPracticeTime() {
	if m.tab == nil || m.practiceSecs <= 0 {
		return
	}
	if m.tab.Metadata == nil {
		m.tab.Metadata = map[string]string{}
	}
	total := 0
	if raw := strings.TrimSpace(m.tab.Metadata["practice_seconds"]); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			total = n
		}
	}
	m.tab.Metadata["practice_seconds"] = strconv.Itoa(total + int(m.practiceSecs))
	m.practiceSecs = 0
}

// practiceTotal returns the accumulated practice seconds (persisted total
// plus the running session).
func (m ViewerModel) practiceTotal() int {
	total := 0
	if m.tab != nil && m.tab.Metadata != nil {
		if raw := strings.TrimSpace(m.tab.Metadata["practice_seconds"]); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil {
				total = n
			}
		}
	}
	total += int(m.practiceSecs)
	if m.playing && !m.practiceStart.IsZero() {
		total += int(time.Since(m.practiceStart) / time.Second)
	}
	return total
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
		// Bank the cursor's mapped audio position before stopping so a
		// later Space resumes mid-song instead of restarting the file.
		m.resumePos = m.resumeAudioPos()
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
	opts := m.playbackOpts()
	opts.resume = m.resumePos
	m.resumePos = 0
	return startPlaybackCmd(m.engine, m.displayTab(), m.bpm, m.tabPath, m.audioDirs, src, m.playbackStartIndex(), opts)
}

// programNames is the instrument cycle for MIDI playback (`y`): GM programs
// a guitarist would actually pick, starting with the steel-guitar default.
var programNames = []struct {
	num  int
	name string
}{
	{25, "steel"},
	{24, "nylon"},
	{27, "clean"},
	{29, "overdrive"},
	{33, "bass"},
}

// nextProgram cycles the GM program through programNames.
func nextProgram(current int) int {
	for i, p := range programNames {
		if p.num == current {
			return programNames[(i+1)%len(programNames)].num
		}
	}
	return programNames[0].num
}

// programLabel renders a GM program number as its display name.
func programLabel(program int) string {
	for _, p := range programNames {
		if p.num == program {
			return p.name
		}
	}
	return fmt.Sprintf("prog %d", program)
}
