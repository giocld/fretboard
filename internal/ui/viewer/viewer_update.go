package viewer

import (
	"fmt"
	"strconv"
	"strings"

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
			var cmds []tea.Cmd
			if cmd := m.applySelectedSourceStateOnly(); cmd != nil {
				m.calibrating = true // the async BPM probe is in flight
				cmds = append(cmds, cmd)
			}
			src := m.selectedSource()
			if src.ID != "" {
				if m.tab.Metadata == nil {
					m.tab.Metadata = map[string]string{}
				}
				m.tab.Metadata["audio_source"] = src.ID
			}
			m.restoreCalibrationForSource()
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
		var cmds []tea.Cmd
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
				if cmd := m.applySelectedSourceStateOnly(); cmd != nil {
					m.calibrating = true // the async BPM probe is in flight
					cmds = append(cmds, cmd)
				}
			} else if prevPick < len(m.audioCatalog.Sources) {
				m.selectedSourceIdx = prevPick
			}
		}
		m.refresh()
		cmds = append(cmds, m.maybeDetectIntroCmd(), m.maybeAlignCmd())
		return m, tea.Batch(cmds...)
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
	m.calibrating = false // the silence probe landed for the current source
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

// handleBPMDerived applies an asynchronously derived audio tempo. The probe
// ran in a background command, so the source may have changed while it was in
// flight: apply only when the source is still current and the tab does not
// already record a tempo (belt and suspenders with the command-creation guard).
func (m ViewerModel) handleBPMDerived(msg msgs.BPMDerivedMsg) (ViewerModel, tea.Cmd) {
	if msg.SourceID == "" || msg.SourceID != m.currentSourceID() {
		return m, nil // the user switched sources while the probe ran
	}
	m.calibrating = false // the BPM probe landed for the current source
	if m.tab != nil && m.tab.Metadata != nil && strings.TrimSpace(m.tab.Metadata[model.MetaKeyBPM]) != "" {
		return m, nil // a recorded tempo wins over the derived one
	}
	if msg.BPM <= 0 {
		return m, nil
	}
	m.bpm = player.ClampBPM(msg.BPM)
	m.refresh()
	return m, nil
}

// handleAlignment applies a completed auto-alignment result. The two-tier
// gate lives here: confident results auto-apply, borderline ones are
// presented through the candidates message, and weak ones are rejected with
// a hint — never silently, never a hard reject of the source.
func (m ViewerModel) handleAlignment(msg msgs.AlignmentMsg) (ViewerModel, tea.Cmd) {
	if !m.matchesAudioTab(msg.TabID, msg.TabPath, msg.Artist, msg.Title) {
		return m, nil
	}
	if msg.SourceID != "" && msg.SourceID != m.currentSourceID() {
		return m, nil // the user switched sources while the analysis ran
	}
	m.calibrating = false // the alignment analysis landed for the current source
	if msg.Err != nil {
		// F3: an analysis failure must surface, never apply partial state.
		m.errMsg = fmt.Sprintf("Audio analysis failed: %v", msg.Err)
		m.refresh()
		return m, nil
	}
	if msg.BPM <= 0 {
		return m, nil // usable-but-weak analysis: nothing to apply
	}
	if msg.Confidence < 0.6 {
		// The 0.4-0.6 band normally arrives as AlignmentCandidatesMsg (the
		// command routes it); a direct weak AlignmentMsg keeps the legacy
		// hint path, and the reject band is never silent.
		if msg.Confidence >= 0.4 {
			m.infoMsg = "Audio alignment is weak - press s at a recognizable bar to anchor"
		} else {
			m.infoMsg = "Audio alignment too weak to apply - press s at a recognizable bar to anchor"
		}
		m.refresh()
		return m, nil
	}
	return m.applyAlignment(msg)
}

// applyAlignment applies a completed alignment: the detected tempo, the
// per-source intro offset (marked auto so silence probing does not re-run),
// the measured bar anchors as the auto tempo map, the onsets for the live
// drift meter, and per-source persistence. Shared by the auto-apply path and
// the user-confirmed candidate — the caller decides whether the result is
// trustworthy enough to reach this point.
func (m ViewerModel) applyAlignment(msg msgs.AlignmentMsg) (ViewerModel, tea.Cmd) {
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
	// The measured bar anchors become the auto tempo map; the onsets feed
	// the live drift meter. Persisted per source so later sessions restore
	// it without re-running the analysis.
	anchors := msg.Anchors
	if len(anchors) == 0 && msg.BPM > 0 && len(msg.Onsets) > 0 {
		// A confirmed candidate carries no measured anchors: derive them
		// from its onsets exactly like the command does for auto results.
		baseBPM := player.TabBPM(m.tab)
		if baseBPM <= 0 {
			baseBPM = 120
		}
		expected := player.ExpectedOnsets(m.tab, baseBPM)
		scale := float64(baseBPM) / float64(msg.BPM)
		anchors = player.TempoAnchors(expected, msg.Onsets, scale, msg.Offset, msg.BPM, 4)
	}
	m.autoAnchors = anchors
	m.autoOnsets = msg.Onsets
	m.autoStrengths = msg.OnsetStrengths
	m.autoActive = len(anchors) >= 2
	m.syncDrift = 0
	if id := m.currentSourceID(); id != "" {
		m.tab.Metadata["tempo_map:"+id] = player.MarshalTempoMap(anchors, msg.Onsets, msg.OnsetStrengths)
	}
	m.refresh()
	return m, m.saveTabPrefsCmd()
}

// handleAlignmentCandidates opens the alignment confirm overlay with the
// top-N ranked hypotheses for the still-current source.
func (m ViewerModel) handleAlignmentCandidates(msg msgs.AlignmentCandidatesMsg) (ViewerModel, tea.Cmd) {
	if !m.matchesAudioTab(msg.TabID, msg.TabPath, msg.Artist, msg.Title) {
		return m, nil
	}
	if msg.SourceID != "" && msg.SourceID != m.currentSourceID() {
		return m, nil // the user switched sources while the analysis ran
	}
	m.calibrating = false // the alignment analysis landed for the current source
	m.alignmentCandidates = msg.Candidates
	m.alignmentPick = -1
	m.showAlignmentConfirm = len(msg.Candidates) > 0
	m.refresh()
	return m, nil
}

// handleAlignmentConfirmKey drives the confirm overlay while it is open:
// 1/2/3 accepts that candidate, Enter accepts the top pick (or the last
// picked candidate), a/b/c/d accepts the picked candidate with that offset
// variant, Esc dismisses without applying.
func (m ViewerModel) handleAlignmentConfirmKey(msg tea.KeyMsg) (ViewerModel, tea.Cmd) {
	s := msg.String()
	if len(s) == 1 && s[0] >= '1' && s[0] <= '3' {
		idx := int(s[0] - '1')
		if idx < len(m.alignmentCandidates) {
			m.alignmentPick = idx
		}
		return m.applyCandidate(m.alignmentPick, 0)
	}
	switch s {
	case "esc":
		m.alignmentCandidates = nil
		m.showAlignmentConfirm = false
		m.alignmentPick = -1
		m.refresh()
		return m, nil
	case "enter":
		idx := m.alignmentPick
		if idx < 0 || idx >= len(m.alignmentCandidates) {
			idx = 0
		}
		return m.applyCandidate(idx, 0)
	case "a", "b", "c", "d":
		idx := m.alignmentPick
		if idx < 0 || idx >= len(m.alignmentCandidates) {
			idx = 0
		}
		return m.applyCandidate(idx, int(s[0]-'a')+1)
	}
	return m, nil
}

// applyCandidate applies the chosen candidate (optionally with one of its
// +- half-beat / +- one-bar offset variants) through the shared apply path
// and closes the confirm overlay.
func (m ViewerModel) applyCandidate(idx, varIdx int) (ViewerModel, tea.Cmd) {
	if idx < 0 || idx >= len(m.alignmentCandidates) {
		return m, nil
	}
	c := m.alignmentCandidates[idx]
	offset := c.Alignment.Offset
	if varIdx > 0 && varIdx-1 < len(c.Variants) {
		offset = c.Variants[varIdx-1].Offset
	}
	artist, title := "", ""
	if m.tab != nil {
		artist, title = m.tab.Artist, m.tab.Title
	}
	msg := msgs.AlignmentMsg{
		SourceID:       m.currentSourceID(),
		BPM:            c.Alignment.BPM,
		Offset:         offset,
		Confidence:     c.Coverage,
		Artist:         artist,
		Title:          title,
		TabID:          m.tabID,
		TabPath:        m.tabPath,
		Onsets:         c.Alignment.Onsets,
		OnsetStrengths: c.Alignment.Strengths,
		Err:            c.Alignment.Err,
	}
	m.alignmentCandidates = nil
	m.showAlignmentConfirm = false
	m.alignmentPick = -1
	return m.applyAlignment(msg)
}
