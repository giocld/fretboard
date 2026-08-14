package viewer

import (
	"encoding/json"
	"fmt"
	"strings"

	"fretboard/internal/model"
	"fretboard/internal/player"
	tea "github.com/charmbracelet/bubbletea"
)

func (m ViewerModel) matchesAudioTab(tabID int64, tabPath, artist, title string) bool {
	if m.tab == nil {
		return false
	}
	if tabID > 0 && m.tabID > 0 {
		return tabID == m.tabID
	}
	if tabPath != "" && m.tabPath != "" && tabPath == m.tabPath {
		return true
	}
	return m.tab.Artist == artist && m.tab.Title == title
}

func pickAudioSourceIndex(tab *model.Tab, cat player.AudioCatalog) int {
	rejected := rejectedSources(tab)
	if tab != nil && tab.Metadata != nil {
		if srcID := strings.TrimSpace(tab.Metadata["audio_source"]); srcID != "" && !rejected[srcID] {
			if found := cat.FindByID(srcID); found >= 0 {
				return found
			}
		}
	}
	// Prefer a ready local backing track; default to MIDI tab synth (not online/BestIndex).
	for i, src := range cat.Sources {
		if src.Kind == player.SourceLocal && src.Path != "" && player.FileExists(src.Path) && !rejected[src.ID] {
			return i
		}
	}
	return 0
}

// rejectedSources returns the set of audio source IDs the user marked as the
// wrong version (persisted under the rejected_audio metadata key).
func rejectedSources(tab *model.Tab) map[string]bool {
	out := map[string]bool{}
	if tab == nil || tab.Metadata == nil {
		return out
	}
	var ids []string
	if err := json.Unmarshal([]byte(tab.Metadata["rejected_audio"]), &ids); err != nil {
		return out
	}
	for _, id := range ids {
		out[id] = true
	}
	return out
}

// rejectCurrentSource records the selected source as the wrong version and
// re-picks the next best candidate, skipping every rejected source.
func (m *ViewerModel) rejectCurrentSource() (ViewerModel, tea.Cmd) {
	if m.tab == nil {
		return *m, nil
	}
	src := m.selectedSource()
	if src.Kind == player.SourceMIDI {
		m.errMsg = "Nothing to reject — MIDI is not a recording"
		m.refresh()
		return *m, nil
	}
	if m.tab.Metadata == nil {
		m.tab.Metadata = map[string]string{}
	}
	var ids []string
	_ = json.Unmarshal([]byte(m.tab.Metadata["rejected_audio"]), &ids)
	ids = append(ids, src.ID)
	data, _ := json.Marshal(ids)
	m.tab.Metadata["rejected_audio"] = string(data)

	m.manualPick = false
	m.selectedSourceIdx = m.autoPickIndex(m.audioCatalog)
	if m.selectedSourceIdx >= len(m.audioCatalog.Sources) {
		m.selectedSourceIdx = 0
	}
	m.audioCursor = m.selectedSourceIdx
	m.restoreCalibrationForSource()
	newSrc := m.selectedSource()
	m.infoMsg = fmt.Sprintf("Rejected %q — now on %q", src.Label, newSrc.Label)
	m.refresh()
	var cmds []tea.Cmd
	cmds = append(cmds, m.saveTabPrefsCmd())
	if newSrc.Kind == player.SourceOnline && (newSrc.Path == "" || !player.FileExists(newSrc.Path)) {
		m.fetchingAudio = true
		cmds = append(cmds, m.downloadSelectedSourceCmd())
	}
	if len(cmds) > 0 {
		return *m, tea.Batch(cmds...)
	}
	return *m, nil
}

// pickStrictAudioSourceIndex is the studio-lock variant: local files first,
// then the best-scoring candidate that passes strict selection (official or
// backing), then MIDI. Live shows, covers, and lessons are never auto-picked
// because they fight the tab's tempo and arrangement.
func pickStrictAudioSourceIndex(tab *model.Tab, cat player.AudioCatalog) int {
	rejected := rejectedSources(tab)
	if tab != nil && tab.Metadata != nil {
		if srcID := strings.TrimSpace(tab.Metadata["audio_source"]); srcID != "" && !rejected[srcID] {
			if found := cat.FindByID(srcID); found >= 0 {
				return found
			}
		}
	}
	best := -1
	for i, src := range cat.Sources {
		// Only studio-compatible local files win the local shortcut: a
		// "Song (Live).mp3" must not outrank the official recording.
		if src.Kind == player.SourceLocal && src.Path != "" && player.FileExists(src.Path) && src.StrictOK && !rejected[src.ID] {
			return i
		}
		if !src.StrictOK || rejected[src.ID] {
			continue
		}
		if best < 0 || src.Score > cat.Sources[best].Score {
			best = i
		}
	}
	if best >= 0 {
		return best
	}
	return 0 // MIDI: safer than auto-downloading a mismatched recording
}

// autoPickIndex chooses the auto-play source for the current strict mode and
// notes when strict selection rejected every online candidate.
func (m *ViewerModel) autoPickIndex(cat player.AudioCatalog) int {
	if m.strictAudio {
		idx := pickStrictAudioSourceIndex(m.tab, cat)
		if idx == 0 && cat.HasStrictRejected() {
			m.infoMsg = "No studio/official match — open [a] to pick audio manually"
		}
		return idx
	}
	return pickAudioSourceIndex(m.tab, cat)
}

// recommendedSourceIdx returns the catalog index the picker should mark as
// the recommended pick for the current strict mode.
func (m ViewerModel) recommendedSourceIdx() int {
	if m.strictAudio {
		return pickStrictAudioSourceIndex(m.tab, m.audioCatalog)
	}
	return pickAudioSourceIndex(m.tab, m.audioCatalog)
}

func (m ViewerModel) selectedSource() player.AudioSource {
	if src := m.audioCatalog.Selected(m.selectedSourceIdx); src != nil {
		return *src
	}
	return player.AudioSource{ID: "midi", Kind: player.SourceMIDI, Label: "MIDI synthesizer"}
}

func (m *ViewerModel) applySelectedSource(deriveBPM bool) {
	src := m.selectedSource()
	if src.Kind == player.SourceMIDI {
		m.resolvedAudio = ""
		return
	}
	path := src.Path
	if src.Kind == player.SourceOnline && (path == "" || !player.FileExists(path)) {
		if m.resolvedAudio != "" && player.FileExists(m.resolvedAudio) {
			path = m.resolvedAudio
		} else {
			return
		}
	}
	if path != "" {
		m.resolvedAudio = path
	}
	if deriveBPM && m.tab != nil && path != "" {
		if dur, err := player.ProbeDuration(path); err == nil && dur > 0 {
			schedule := player.BuildSchedule(m.tab)
			if meta := m.tab.Metadata; meta == nil || strings.TrimSpace(meta[model.MetaKeyBPM]) == "" {
				m.bpm = player.DeriveBPMFromAudio(schedule, dur, m.audioOffsetDur())
			}
		}
	}
}

func (m ViewerModel) openAudioPicker() (ViewerModel, tea.Cmd) {
	if m.tab == nil {
		return m, nil
	}
	m.showAudioPicker = true
	m.errMsg = ""
	if len(m.audioCatalog.Sources) <= 1 {
		m.fetchingCatalog = true
		return m, fetchAudioCatalogCmd(m.tab, m.tabPath, m.tabID, m.audioDirs, m.allowOnline)
	}
	m.audioCursor = m.selectedSourceIdx
	return m, nil
}

func (m ViewerModel) handleAudioPickerKey(msg tea.KeyMsg) (ViewerModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.showAudioPicker = false
		m.fetchingCatalog = false
		return m, nil
	case "j", "down":
		if m.audioCursor < len(m.audioCatalog.Sources)-1 {
			m.audioCursor++
		}
		return m, nil
	case "k", "up":
		if m.audioCursor > 0 {
			m.audioCursor--
		}
		return m, nil
	case "r":
		m.fetchingCatalog = true
		return m, fetchAudioCatalogCmd(m.tab, m.tabPath, m.tabID, m.audioDirs, m.allowOnline)
	case "enter":
		if len(m.audioCatalog.Sources) == 0 {
			return m, nil
		}
		if m.playing {
			m.stopPlayback()
		}
		m.selectedSourceIdx = m.audioCursor
		m.showAudioPicker = false
		m.manualPick = true // sticky: keep this choice across catalog refreshes
		src := m.selectedSource()
		// Switching recordings means switching calibration: restore the new
		// source's intro offset and sync anchors.
		m.restoreCalibrationForSource()
		if src.Kind == player.SourceOnline && (src.Path == "" || !player.FileExists(src.Path)) {
			m.fetchingAudio = true
			m.pendingPlay = true
			return m, m.downloadSelectedSourceCmd()
		}
		m.applySelectedSource(true)
		if m.tab != nil {
			if m.tab.Metadata == nil {
				m.tab.Metadata = map[string]string{}
			}
			m.tab.Metadata["audio_source"] = src.ID
		}
		return m, tea.Batch(m.saveTabPrefsCmd(), m.maybeDetectIntroCmd(), m.maybeAlignCmd())
	}
	return m, nil
}
