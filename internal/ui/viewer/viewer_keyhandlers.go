package viewer

import (
	"fmt"
	"time"

	"fretboard/internal/player"
	"fretboard/internal/ui/kit"
	"fretboard/internal/ui/msgs"
	tea "github.com/charmbracelet/bubbletea"
)

// handleKeyPractice applies the playback, practice, audio and loop keys.
func (m ViewerModel) handleKeyPractice(msg tea.KeyMsg) (ViewerModel, tea.Cmd) {
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
		m.manualBPM = true // a user nudge wins over the provenance chain
		m.jumpBuffer = ""
		if m.sessionMode && !m.playing {
			// While paused, +/- set the session's ramp target.
			m.sessionTargetBPM = m.bpm
		}
		if m.playing && m.tab != nil {
			if m.audioSync {
				// Audio: restart the player so the mapping matches the new tempo.
				_ = m.engine.Stop()
				m.resetPlayback()
				m.refresh()
				return m, startPlaybackCmd(m.engine, m.displayTab(), m.bpm, m.tabPath, m.audioDirs, m.selectedSource(), m.playbackStartIndex(), m.playbackOpts())
			}
			// MIDI: re-base the deadline clock — the current step gets a
			// fresh duration at the new tempo, the session never restarts.
			m.stepClock.Rebase(stepDur(m.schedule[m.stepIdx].Ticks, m.bpm))
		}
		m.refresh()
	case "-", "_":
		m.bpm = player.ClampBPM(m.bpm - 5)
		m.manualBPM = true
		m.jumpBuffer = ""
		if m.sessionMode && !m.playing {
			m.sessionTargetBPM = m.bpm
		}
		if m.playing && m.tab != nil {
			if m.audioSync {
				_ = m.engine.Stop()
				m.resetPlayback()
				m.refresh()
				return m, startPlaybackCmd(m.engine, m.displayTab(), m.bpm, m.tabPath, m.audioDirs, m.selectedSource(), m.playbackStartIndex(), m.playbackOpts())
			}
			m.stepClock.Rebase(stepDur(m.schedule[m.stepIdx].Ticks, m.bpm))
		}
		m.refresh()
	case "P":
		m.perfMode = !m.perfMode
		m.jumpBuffer = ""
		m.refresh()
	case "M":
		// S8.1 practice session: M toggles session mode (paused + loop set
		// to enter), exits with a summary card and a PracticeSessionMsg.
		m.jumpBuffer = ""
		if m.sessionMode {
			m.sessionCard = ""
			return m.endSessionCmd()
		}
		if m.playing {
			m.errMsg = "Pause playback before entering session mode"
			m.refresh()
			return m, nil
		}
		if m.loopStartBar <= 0 || m.loopEndBar <= 0 {
			m.errMsg = "Set a loop first (i start / u end), then press M"
			m.refresh()
			return m, nil
		}
		m.startSession()
		return m, nil
	case "s":
		if m.tab == nil {
			break
		}
		if m.verify != nil && m.verify.State == player.VerifyPending {
			// S4.1: a manual anchor during the verification window cancels
			// the passive keep — the user is calibrating by hand now.
			m.verify.Refine()
			m.verify = nil
			m.errMsg = ""
			m.infoMsg = "Alignment verification skipped — sync anchors are manual now"
			m.refresh()
			return m, nil
		}
		if m.playing && m.audioSync {
			m.errMsg = ""
			return m.setSyncPoint()
		}
		if !m.playing && m.calibrationAudioLoaded() {
			// S4.2: paused calibration — jump to a bar and anchor it to the
			// audio position the cursor maps to; the anchor is sanity-checked.
			return m.setPausedSyncPoint()
		}
		m.errMsg = "Sync bar needs a real recording: play with an audio source (a + Space), then press s here"
		m.refresh()
	case "U":
		// S4.2 one-key undo of the last sync anchor (u is the loop end key).
		m.jumpBuffer = ""
		if len(m.syncPoints) > 0 {
			last := m.syncPoints[len(m.syncPoints)-1]
			m.syncPoints = m.syncPoints[:len(m.syncPoints)-1]
			m.saveSyncPoints()
			m.errMsg = ""
			m.warnMsg = ""
			m.infoMsg = fmt.Sprintf("Undid sync anchor at bar %d", last.Bar)
			m.refresh()
		} else {
			m.errMsg = "No sync anchors to undo"
			m.refresh()
		}
	case "S":
		if len(m.syncPoints) > 0 {
			last := m.syncPoints[len(m.syncPoints)-1]
			m.syncPoints = m.syncPoints[:len(m.syncPoints)-1]
			m.saveSyncPoints()
			m.errMsg = ""
			m.infoMsg = fmt.Sprintf("Removed sync anchor at bar %d — press S again to remove more", last.Bar)
			m.refresh()
		} else {
			m.errMsg = "No sync points to remove"
			m.refresh()
		}
	case "E", "$":
		// S1.3 quick edit: spawn $EDITOR on the raw text, re-parse on exit.
		return m.startQuickEdit()
	case "ctrl+p":
		// S1.4 print: write the HTML export next to the tab file.
		return m.printHTML()
	case "K":
		// S3.3 cache screen: list entries, delete one or all.
		m.openCacheScreen()
		return m, nil
	case "t":
		// S5.3b: cycle the Guitar Pro track (multi-track files only).
		return m.cycleGPTrack()
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
		if err := m.engine.SetRate(m.engine.Rate() * 1.1); err != nil {
			m.errMsg = err.Error()
		}
		m.refresh()
	case "<":
		if err := m.engine.SetRate(m.engine.Rate() / 1.1); err != nil {
			m.errMsg = err.Error()
		}
		m.refresh()
	case "r":
		if err := m.engine.SetRate(1); err != nil {
			m.errMsg = err.Error()
		}
		m.refresh()
	case "w":
		// "Wrong version": reject the current source and move to the next
		// candidate; the rejection is persisted so future sessions skip it.
		m.jumpBuffer = ""
		return m.rejectCurrentSource()
	case "W", "f9":
		// Re-run the auto alignment for the current source (e.g. after a
		// better recording was downloaded). F9 is the explicit realign
		// shortcut; W shares the same behavior.
		return m.realignAudio()
	case "m":
		m.metronome = !m.metronome
		m.jumpBuffer = ""
		m.refresh()
	case "C":
		m.countIn = (m.countIn + 1) % 3
		m.jumpBuffer = ""
		m.refresh()
	case "f8":
		// F8 mirrors C: cycle the count-in lead-in length 0/1/2 bars.
		m.countIn = (m.countIn + 1) % 3
		m.jumpBuffer = ""
		m.refresh()
	case "f7":
		// F7 seeks +15s in the backing audio (the hyperplan "F7 = 15%"
		// reading is impossible because SetRate clamps at 0.25). bubbletea
		// cannot distinguish shift+F7 from F7, so plain F7 is forward-only.
		m.jumpBuffer = ""
		if pos, ok := seekBy(m, 15*time.Second); ok {
			if err := m.engine.RestartAt(pos); err != nil {
				m.errMsg = err.Error()
				m.refresh()
				return m, nil
			}
			m.errMsg = ""
		}
		m.refresh()
		return m, nil
	case "f12":
		// F12 reports the repeat-order MIDI event summary. No events view
		// exists yet, so the counts surface as an info status line.
		m.jumpBuffer = ""
		if m.tab != nil {
			evts, err := player.Events(m.displayTab(), m.bpm)
			if err != nil {
				m.errMsg = err.Error()
				m.refresh()
				return m, nil
			}
			nOns := 0
			for _, e := range evts {
				if e.Type == player.NoteOn {
					nOns++
				}
			}
			m.infoMsg = fmt.Sprintf("%d events / %d note-ons (repeat order)", len(evts), nOns)
			m.errMsg = ""
			m.refresh()
			return m, nil
		}
		m.errMsg = "No tab to summarize"
		m.refresh()
		return m, nil
	case "y":
		m.program = nextProgram(m.program)
		m.jumpBuffer = ""
		if m.playing && m.tab != nil && m.engine.Mode() == "midi" {
			_ = m.engine.Stop()
			m.resetPlayback()
			m.refresh()
			return m, startPlaybackCmd(m.engine, m.displayTab(), m.bpm, m.tabPath, m.audioDirs, m.selectedSource(), m.playbackStartIndex(), m.playbackOpts())
		}
		m.refresh()
	case "[", "{", "]", "}", ",", ".", "o":
		return m.adjustAudioOffset(msg.String())
	case "esc":
		if m.jumpBuffer != "" {
			m.jumpBuffer = ""
			return m, nil
		}
		if m.errMsg != "" || m.infoMsg != "" || m.warnMsg != "" {
			m.errMsg = ""
			m.infoMsg = ""
			m.warnMsg = ""
			m.refresh()
			return m, nil
		}
		if m.sessionCard != "" {
			m.sessionCard = ""
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
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// realignAudio re-runs the auto alignment for the current source: it clears
// the already-aligned identity marker so maybeAlignCmd starts a fresh
// analysis, and reports an error when there is no tab or audio catalog to
// align. Shared by the W and F9 keys.
func (m ViewerModel) realignAudio() (ViewerModel, tea.Cmd) {
	m.jumpBuffer = ""
	if m.tab != nil && m.alignedIdentity != nil {
		if id := m.currentSourceID(); id != "" {
			delete(m.alignedIdentity, id)
		}
		m.errMsg = ""
		m.infoMsg = "Re-running audio alignment..."
		m.refresh()
		return m, m.maybeAlignCmd()
	}
	m.errMsg = "No audio source to align"
	m.refresh()
	return m, nil
}
