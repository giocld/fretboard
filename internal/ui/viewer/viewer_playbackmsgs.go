package viewer

import (
	"time"

	"fretboard/internal/player"
	"fretboard/internal/ui/msgs"
	tea "github.com/charmbracelet/bubbletea"
)

// handlePlaybackStarted arms the deadline clock and re-arms the A-B loop on playback start.
func (m ViewerModel) handlePlaybackStarted(msg msgs.PlaybackStartedMsg) (ViewerModel, tea.Cmd) {
	m.playGen++
	m.playing = true
	m.schedule = msg.Schedule
	m.stepIdx = msg.StepIdx
	m.tickDur = msg.Duration
	m.audioSync = msg.AudioSync
	m.endBanner = false // a new playback clears any previously shown track-ended banner
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
		// The command sounded the first note before its message entered the
		// UI queue. Anchor to that instant, not the later handler time.
		started := msg.Started
		if started.IsZero() {
			started = time.Now()
		}
		m.stepClock.StartAt(started, m.tickDur)
		m.driftMs = 0
		wait := time.Until(m.stepClock.Deadline())
		if wait < time.Millisecond {
			wait = time.Millisecond
		}
		return m, tea.Batch(tickCmd(m.playGen, wait), monitorPlaybackCmd(m.playGen, false))
	}
	return m, monitorPlaybackCmd(m.playGen, true)
}

// handlePlaybackError stops playback and surfaces the playback error.
func (m ViewerModel) handlePlaybackError(msg msgs.PlaybackErrorMsg) (ViewerModel, tea.Cmd) {
	m.stopPlayback()
	m.endBanner = false // a failed restart must not keep the banner state
	m.errMsg = msg.Err.Error()
	m.refresh()
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// handlePlaybackMonitor tracks the playhead against the audio and re-arms the loop.
func (m ViewerModel) handlePlaybackMonitor(msg msgs.PlaybackMonitorMsg) (ViewerModel, tea.Cmd) {
	if msg.Gen != m.playGen {
		return m, nil
	}
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
			if m.loopPassed(elapsed) {
				if err := m.engine.RestartAt(m.loopRestartPos()); err != nil {
					m.errMsg = "Loop restart failed: " + err.Error()
					m.stopPlayback()
					m.refresh()
					return m, nil
				}
				m.noteLoopPass() // S8.1: a completed A-B loop pass
				if m.sessionRamp {
					// The session's tempo ramped: audio cannot re-time
					// mid-file, so restart the player at the new BPM (the
					// same path the +/- keys take).
					m.sessionRamp = false
					m.refresh()
					return m, startPlaybackCmd(m.engine, m.displayTab(), m.bpm, m.tabPath, m.audioDirs, m.selectedSource(), m.playbackStartIndex(), m.playbackOpts())
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
			// An onset *ahead* of the audio clock is not audible yet.
			// Snapping to it skipped a note, then the next poll could jump
			// back when the onset fell inside the snap threshold. Likewise
			// never rewind a note that was already shown in this pass.
			if ok {
				idx = boundedAudioSnap(m.stepIdx, idx, snapIdx)
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
				m.endBanner = true
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
	return m, monitorPlaybackCmd(m.playGen, m.audioSync)
}

// boundedAudioSnap keeps live onset correction from showing a future note
// before the player reaches it or rewinding a note already displayed. The
// tempo map is monotone in audio time, so snapped <= mapped is the same as
// "the snapped onset has already sounded": an onset still ahead of the audio
// clock maps past the current position and is refused. mapped (not snapped)
// is returned outside the window, so seeks and loop wraps follow the audio.
func boundedAudioSnap(current, mapped, snapped int) int {
	if snapped >= current && snapped <= mapped {
		return snapped
	}
	return mapped
}

// handlePlaybackTick advances the MIDI deadline clock one step.
func (m ViewerModel) handlePlaybackTick(msg msgs.PlaybackTickMsg) (ViewerModel, tea.Cmd) {
	if msg.Gen != m.playGen {
		return m, nil
	}
	if !m.playing || len(m.schedule) == 0 {
		return m, nil
	}
	if m.audioSync {
		return m, nil // only the monitor chain samples audio
	}
	// Deadline-clock MIDI loop: the tick fired at the absolute deadline
	// of the step we are about to play. Roll the clock past it (the
	// advance is by the step being played, so render/processing time
	// does not normally shift the beat).
	next := m.nextStepIndexFrom(m.stepIdx)
	if m.sessionMode && m.loopEndBar > 0 && next < len(m.schedule) && m.schedule[next].Bar < m.schedule[m.stepIdx].Bar {
		// S8.1: the A-B loop wrapped — count the pass (and ramp the tempo
		// when the session configured one).
		m.noteLoopPass()
	}
	if next >= len(m.schedule) {
		m.stopPlayback()
		m.refresh()
		return m, nil
	}
	onsetLate := m.stepClock.Late(time.Now())
	m.stepClock.Next(stepDur(m.schedule[next].Ticks, m.bpm))
	// Never drop a scheduled note to catch up with UI latency. If the
	// entire next step would already have elapsed, rebase from this note's
	// actual onset rather than queuing a burst of late notes. Small delays
	// keep the absolute deadline and naturally settle on the next beat.
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
	if m.stepClock.Late(time.Now()) > 0 {
		m.stepClock.Rebase(m.tickDur)
	}
	// Drift telemetry: how late the clock was when the tick arrived.
	m.driftMs = 0
	if onsetLate > 20*time.Millisecond {
		m.driftMs = onsetLate.Milliseconds()
	}
	m.ensureCursorVisible()
	m.refresh()
	wait := time.Until(m.stepClock.Deadline())
	if wait < time.Millisecond {
		wait = time.Millisecond
	}
	return m, tickCmd(m.playGen, wait)
}
