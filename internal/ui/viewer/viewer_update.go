package viewer

import (
	"fmt"
	"strconv"

	"fretboard/internal/model"
	"fretboard/internal/player"
	"fretboard/internal/ui/msgs"
	tea "github.com/charmbracelet/bubbletea"
)

// handleAudioFetched applies a completed background audio download.
func (m ViewerModel) handleAudioFetched(msg msgs.AudioFetchedMsg) (ViewerModel, tea.Cmd) {
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
				cmds = append(cmds, startPlaybackCmd(m.engine, m.displayTab(), m.bpm, m.tabPath, m.audioDirs, m.selectedSource(), m.playbackStartIndex(), m.playbackOpts()))
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
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// handleAudioCatalog applies a refreshed audio source catalog.
func (m ViewerModel) handleAudioCatalog(msg msgs.AudioCatalogMsg) (ViewerModel, tea.Cmd) {
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
				if m.manualPick && m.selectedSourceIdx >= 0 && m.selectedSourceIdx < len(msg.Catalog.Sources) {
					// A manually chosen source survives catalog refreshes:
					// keep it when it is still available.
					cur := msg.Catalog.Sources[m.selectedSourceIdx]
					if idx := msg.Catalog.FindByID(cur.ID); idx >= 0 {
						m.selectedSourceIdx = idx
					} else {
						m.selectedSourceIdx = m.autoPickIndex(msg.Catalog)
					}
				} else {
					m.selectedSourceIdx = m.autoPickIndex(msg.Catalog)
				}
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
		return m, tea.Batch(m.maybeDetectIntroCmd(), m.maybeAlignCmd())
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// handleIntroDetected applies an auto-detected intro offset.
func (m ViewerModel) handleIntroDetected(msg msgs.IntroDetectedMsg) (ViewerModel, tea.Cmd) {
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
		m.infoMsg = fmt.Sprintf("auto-detected intro offset %+.1fs — fine-tune with [ ] , .", m.audioOffset)
		m.refresh()
		return m, m.saveTabPrefsCmd()
	}
	return m, nil
}

// handleAlignment applies a completed auto-alignment result.
func (m ViewerModel) handleAlignment(msg msgs.AlignmentMsg) (ViewerModel, tea.Cmd) {
	if !m.matchesAudioTab(msg.TabID, msg.TabPath, msg.Artist, msg.Title) {
		return m, nil
	}
	if msg.SourceID != "" && msg.SourceID != m.currentSourceID() {
		return m, nil // the user switched sources while the analysis ran
	}
	if msg.BPM <= 0 || msg.Confidence < 0.6 {
		if msg.BPM > 0 && msg.Confidence >= 0.4 {
			m.infoMsg = "Audio alignment is weak — press s at a recognizable bar to anchor"
			m.refresh()
		}
		return m, nil
	}
	// Apply the detected tempo and the per-source intro offset (marked
	// auto so silence probing does not re-run).
	m.bpm = player.ClampBPM(msg.BPM)
	if m.audioOffset == 0 && len(m.syncPoints) == 0 {
		m.audioOffset = msg.Offset.Seconds()
		if m.tab.Metadata == nil {
			m.tab.Metadata = map[string]string{}
		}
		offset := strconv.FormatFloat(m.audioOffset, 'f', 1, 64)
		m.tab.Metadata[m.audioOffsetKey()] = offset
		m.tab.Metadata[model.MetaKeyAudioOffset] = offset
		if msg.SourceID != "" {
			m.tab.Metadata["audio_aligned:"+msg.SourceID] = "1"
		}
		m.infoMsg = fmt.Sprintf("Auto-aligned %d BPM, offset +%.1fs (confidence %.0f%%)", msg.BPM, m.audioOffset, msg.Confidence*100)
	}
	// The measured bar anchors become the auto tempo map; the onsets
	// feed the live drift meter. Persisted per source so later sessions
	// restore it without re-running the analysis.
	m.autoAnchors = msg.Anchors
	m.autoOnsets = msg.Onsets
	m.autoActive = len(msg.Anchors) >= 2
	m.syncDrift = 0
	if id := m.currentSourceID(); id != "" {
		m.tab.Metadata["tempo_map:"+id] = player.MarshalTempoMap(msg.Anchors, msg.Onsets)
	}
	m.refresh()
	return m, m.saveTabPrefsCmd()
}
