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
	m.calibrating = true // the onset analysis is in flight until its msg lands
	return func() tea.Msg {
		hint, _ := player.LeadingSilence(path)
		cands, err := player.RankAlignments(tab, path, hint)
		if err != nil {
			return msgs.AlignmentMsg{SourceID: srcID, Err: err, Artist: tab.Artist, Title: tab.Title, TabID: tabID, TabPath: tabPath}
		}
		if len(cands) == 0 {
			return msgs.AlignmentMsg{SourceID: srcID, Artist: tab.Artist, Title: tab.Title, TabID: tabID, TabPath: tabPath}
		}
		// One analysis drives both the auto path and the ranked list: the
		// band of the best candidate decides how the viewer treats it.
		top := cands[0]
		band, _ := player.ClassifyBand(top.Alignment.Confidence, top.Coverage, top.IdentityZone)
		switch band {
		case player.BandAuto:
			// Confident and well covered: apply without asking.
			msg := msgs.AlignmentMsg{
				SourceID: srcID, BPM: top.Alignment.BPM, Offset: top.Alignment.Offset, Confidence: top.Alignment.Confidence,
				Artist: tab.Artist, Title: tab.Title, TabID: tabID, TabPath: tabPath,
				Onsets: top.Alignment.Onsets, OnsetStrengths: top.Alignment.Strengths, Err: top.Alignment.Err,
			}
			if msg.BPM > 0 {
				// Measure bar anchors from the aligned onsets: the auto tempo map.
				expected := player.ExpectedOnsets(tab, baseBPM)
				scale := float64(baseBPM) / float64(msg.BPM)
				msg.Anchors = player.TempoAnchors(expected, msg.Onsets, scale, msg.Offset, msg.BPM, 4)
			}
			return msg
		case player.BandPresent:
			// Present the top-N for the user to confirm or dismiss.
			return msgs.AlignmentCandidatesMsg{
				SourceID: srcID, Candidates: cands,
				Artist: tab.Artist, Title: tab.Title, TabID: tabID, TabPath: tabPath,
			}
		default:
			// Reject: never silent — the weak branch of handleAlignment hints
			// at manual anchoring, and the source stays usable.
			return msgs.AlignmentMsg{
				SourceID: srcID, BPM: top.Alignment.BPM, Offset: top.Alignment.Offset, Confidence: top.Alignment.Confidence,
				Artist: tab.Artist, Title: tab.Title, TabID: tabID, TabPath: tabPath,
				Onsets: top.Alignment.Onsets, OnsetStrengths: top.Alignment.Strengths,
			}
		}
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
	m.calibrating = true // the silence probe is in flight until its msg lands
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
	var cmds []tea.Cmd
	cat, err := player.BuildAudioCatalog(m.tab, m.tabPath, m.audioDirs, false)
	if err == nil && len(cat.Sources) > 0 {
		m.audioCatalog = cat
		m.selectedSourceIdx = m.autoPickIndex(cat)
		if m.selectedSourceIdx >= len(m.audioCatalog.Sources) {
			m.selectedSourceIdx = 0
		}
		m.audioCursor = m.selectedSourceIdx
		m.restoreCalibrationForSource()
		if cmd := m.applySelectedSourceStateOnly(); cmd != nil {
			m.calibrating = true // the async BPM probe is in flight
			cmds = append(cmds, cmd)
		}
	}
	if !allowOnline || !player.OnlineAudioAvailable() || player.AudioSearchQuery(m.tab) == "" {
		cmds = append(cmds, m.maybeDetectIntroCmd(), m.maybeAlignCmd())
		return tea.Batch(cmds...)
	}
	m.fetchingCatalog = true
	cmds = append(cmds, fetchAudioCatalogCmd(m.tab, m.tabPath, m.tabID, m.audioDirs, allowOnline), m.maybeDetectIntroCmd())
	return tea.Batch(cmds...)
}
