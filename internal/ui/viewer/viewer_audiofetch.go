package viewer

import (
	"fretboard/internal/player"
	"fretboard/internal/ui/msgs"
	tea "github.com/charmbracelet/bubbletea"
)

// maybeAlignCmd returns a command that auto-aligns the selected audio
// source against the tab (once per source per session): it probes the
// leading silence for an offset prior, runs the onset analysis, and delivers
// the result for the viewer to apply.
func (m *ViewerModel) maybeAlignCmd() tea.Cmd {
	if m.tab == nil || m.alignedSources == nil {
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
	if m.alignedSources[src.ID] {
		return nil
	}
	m.alignedSources[src.ID] = true
	tab, tabID, tabPath, srcID := m.tab, m.tabID, m.tabPath, src.ID
	baseBPM := player.TabBPM(tab)
	if baseBPM <= 0 {
		baseBPM = 120
	}
	return func() tea.Msg {
		hint, _ := player.LeadingSilence(path)
		a := player.AlignAudio(tab, path, hint)
		msg := msgs.AlignmentMsg{
			SourceID: srcID, BPM: a.BPM, Offset: a.Offset, Confidence: a.Confidence,
			Artist: tab.Artist, Title: tab.Title, TabID: tabID, TabPath: tabPath,
			Onsets: a.Onsets, OnsetStrengths: a.Strengths, Err: a.Err,
		}
		if a.Confidence >= 0.6 && a.BPM > 0 {
			// Measure bar anchors from the aligned onsets: the auto tempo map.
			expected := player.ExpectedOnsets(tab, baseBPM)
			scale := float64(baseBPM) / float64(a.BPM)
			msg.Anchors = player.TempoAnchors(expected, a.Onsets, scale, a.Offset, a.BPM, 4)
		}
		return msg
	}
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
		return tea.Batch(m.maybeDetectIntroCmd(), m.maybeAlignCmd())
	}
	m.fetchingCatalog = true
	return tea.Batch(fetchAudioCatalogCmd(m.tab, m.tabPath, m.tabID, m.audioDirs, allowOnline), m.maybeDetectIntroCmd())
}
