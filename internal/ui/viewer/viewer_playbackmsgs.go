package viewer

import (
	"time"

	"fretboard/internal/player"
	"fretboard/internal/ui/msgs"
	tea "github.com/charmbracelet/bubbletea"
)

// handlePlaybackStarted arms the deadline clock and re-arms the A-B loop on playback start.
func (m ViewerModel) handlePlaybackStarted(msg msgs.PlaybackStartedMsg) (ViewerModel, tea.Cmd) {
	m.playing = true
	m.schedule = msg.Schedule
	m.stepIdx = msg.StepIdx
	m.tickDur = msg.Duration
	m.audioSync = msg.AudioSync
	m.practiceStart = time.Now()
	// Drift nudge: without sync points the cursor maps at the tab's
	// BPM; if the recording is a different tempo, warn once so the user
	// knows to anchor.
	if m.audioSync && len(m.syncPoints) == 0 && m.tab != nil {
		if dur := m.engine.AudioDuration(); dur > 0 && len(m.schedule) > 0 {
			derived := player.DeriveBPMFromAudio(m.schedule, dur, m.audioOffsetDur())
			if hint := driftNudge(derived, m.bpm); hint != "" && m.infoMsg == "" {
				m.infoMsg = hint
			}
		}
	}
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
	if !m.audioSync {
		// Deadline clock for the MIDI loop: the first tick fires at the
		// end of the step the start command already sounded.
		m.stepClock.Start(m.tickDur)
		m.driftMs = 0
	}
	return m, tea.Batch(tickCmd(m.tickDur), monitorPlaybackCmd(m.engine))
}

// handlePlaybackError stops playback and surfaces the playback error.
func (m ViewerModel) handlePlaybackError(msg msgs.PlaybackErrorMsg) (ViewerModel, tea.Cmd) {
	m.stopPlayback()
	m.errMsg = msg.Err.Error()
	m.refresh()
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// handlePlaybackMonitor tracks the playhead against the audio and re-arms the loop.
func (m ViewerModel) handlePlaybackMonitor(msg msgs.PlaybackMonitorMsg) (ViewerModel, tea.Cmd) {
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
		// 0-based bar indices, so convert before mapping. The auto tempo
		// map merges under user anchors (user wins per bar).
		combined := player.MergeAnchors(m.syncPoints, m.autoAnchors)
		points := syncPointsZeroBased(combined)
		if len(points) == 0 {
			points = []player.SyncPoint{{Bar: 0, Seconds: m.audioOffset}}
		}
		idx := player.StepIndexAtSyncPoints(m.schedule, points, elapsed.Seconds(), m.bpm)
		if m.autoActive {
			// Live drift meter + bounded self-correction: when the
			// playhead has drifted off the recording's detected onsets,
			// snap it to the onset-aligned position. With onset strengths
			// available, equidistant onsets resolve to the stronger one.
			var snapIdx int
			var ok bool
			if m.autoStrengths != nil && len(m.autoStrengths) == len(m.autoOnsets) {
				weighted := make([]player.Onset, len(m.autoOnsets))
				for i, o := range m.autoOnsets {
					weighted[i] = player.Onset{Time: o, Strength: m.autoStrengths[i]}
				}
				snapIdx, ok = player.CorrectStepSnapWithStrength(m.schedule, points, elapsed, weighted, m.bpm)
			} else {
				snapIdx, ok = player.CorrectStepSnap(m.schedule, points, elapsed, m.autoOnsets, m.bpm)
			}
			if ok {
				idx = snapIdx
			}
			m.syncDrift = 0
			if n, ok := player.NearestOnset(m.autoOnsets, elapsed, 500*time.Millisecond); ok {
				m.syncDrift = (elapsed - n).Seconds()
			}
		}
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
			// A recording that ends before the tab does (radio edit, live
			// cut) must not look like a crash: say what happened and how
			// to restart.
			if !atEnd {
				m.errMsg = trackEndedBanner(m.engine.AudioDuration())
			}
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
}

// handlePlaybackTick advances the MIDI deadline clock one step.
func (m ViewerModel) handlePlaybackTick(msg msgs.PlaybackTickMsg) (ViewerModel, tea.Cmd) {
	if !m.playing || len(m.schedule) == 0 {
		return m, nil
	}
	if m.audioSync {
		return m, monitorPlaybackCmd(m.engine)
	}
	// Deadline-clock MIDI loop: the tick fired at the absolute deadline
	// of the step we are about to play. Roll the clock past it (the
	// advance is by the step being played, so render/processing time
	// never shifts the beat), then catch up if we are late.
	next := m.nextStepIndexFrom(m.stepIdx)
	if next >= len(m.schedule) {
		m.stopPlayback()
		m.refresh()
		return m, nil
	}
	m.stepClock.Next(stepDur(m.schedule[next].Ticks, m.bpm))
	// Catch up: any step whose deadline already passed is skipped
	// (bounded) instead of starting late — late ticks must never
	// accumulate.
	jumps := 0
	for jumps < 8 && next < len(m.schedule)-1 && m.stepClock.Late(time.Now()) > 0 {
		m.stepClock.Next(stepDur(m.schedule[next+1].Ticks, m.bpm))
		next = m.nextStepIndexFrom(next)
		jumps++
	}
	if next >= len(m.schedule) {
		m.stopPlayback()
		m.refresh()
		return m, nil
	}
	m.stepIdx = next
	step := m.schedule[next]
	m.cursorBar = step.Bar
	m.cursorCol = step.Col
	m.tickDur = stepDur(step.Ticks, m.bpm)
	if m.engine.Mode() == "midi" {
		if err := m.engine.PlayMIDIStep(m.displayTab(), step, m.bpm); err != nil {
			m.errMsg = err.Error()
		}
	}
	// Drift telemetry: how late the clock was when the tick arrived.
	m.driftMs = 0
	if lat := m.stepClock.Late(time.Now()); lat > 20*time.Millisecond {
		m.driftMs = lat.Milliseconds()
	}
	m.ensureCursorVisible()
	m.refresh()
	wait := time.Until(m.stepClock.Deadline())
	if wait < time.Millisecond {
		wait = time.Millisecond
	}
	return m, tea.Batch(tickCmd(wait), monitorPlaybackCmd(m.engine))
}
