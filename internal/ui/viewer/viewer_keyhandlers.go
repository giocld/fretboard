package viewer

import (
	"fmt"

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
		m.jumpBuffer = ""
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
		m.jumpBuffer = ""
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
	case "s":
		if m.audioSync && m.playing && m.tab != nil {
			m.errMsg = ""
			return m.setSyncPoint()
		}
		m.errMsg = "Sync bar needs a real recording: play with an audio source (a + Space), then press s here"
		m.refresh()
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
	case "W":
		// Re-run the auto alignment for the current source (e.g. after a
		// better recording was downloaded).
		m.jumpBuffer = ""
		if m.tab != nil && m.alignedSources != nil {
			if id := m.currentSourceID(); id != "" {
				delete(m.alignedSources, id)
			}
			m.errMsg = ""
			m.infoMsg = "Re-running audio alignment..."
			m.refresh()
			return m, m.maybeAlignCmd()
		}
		m.errMsg = "No audio source to align"
		m.refresh()
	case "m":
		m.metronome = !m.metronome
		m.jumpBuffer = ""
		m.refresh()
	case "C":
		m.countIn = (m.countIn + 1) % 3
		m.jumpBuffer = ""
		m.refresh()
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
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}
