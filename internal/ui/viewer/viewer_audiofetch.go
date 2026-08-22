package viewer

import (
	"fmt"
	"hash"
	"hash/fnv"
	"io"
	"os"
	"strconv"

	"fretboard/internal/player"
	"fretboard/internal/ui/msgs"
	tea "github.com/charmbracelet/bubbletea"
)

// maybeAlignCmd returns a command that auto-aligns the selected audio
// source against the tab (once per source per session while the source's
// identity is unchanged): it probes the leading silence for an offset prior,
// runs the onset analysis, and delivers the result for the viewer to apply.
// A source that was already aligned for THIS file is skipped; swapping the
// audio file (or editing the tab) changes the identity, so the analysis
// re-runs for the same source.
func (m *ViewerModel) maybeAlignCmd() tea.Cmd {
	if m.tab == nil || m.alignedIdentity == nil {
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
	if m.alignedIdentity[src.ID] == identityFor(*m, src.ID) {
		return nil
	}
	m.alignedIdentity[src.ID] = identityFor(*m, src.ID)
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

func (m *ViewerModel) downloadSelectedSourceCmd() tea.Cmd {
	tab := m.tab
	tabID := m.tabID
	tabPath := m.tabPath
	src := m.selectedSource()
	// S3.3: surface the downloader's progress in the status row. The state
	// slot is shared across model copies (pointer field), and the package
	// hook is registered for the fetch's lifetime.
	st := m.downloadState()
	st.begin()
	return func() tea.Msg {
		defer st.end()
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

// identityFor returns an FNV-1a fingerprint of the inputs an alignment was
// computed against: the loaded document and the audio file behind the given
// source. The document-hash covers the tab path, title, artist and the total
// MIDI ticks of the playback schedule (player.ScheduleTotalTicks of
// player.BuildSchedule — a cheap, BPM-independent structural measure of the
// song as written). The content-hash covers the audio file's size, its mtime
// in nanoseconds, and the first and last 64 KiB of its bytes (cheap and
// change-detecting for swapped files). An unchanged tab and file produce the
// same string across calls and sessions; swapping the file (or editing the
// tab) produces a different one, so a persisted alignment can be invalidated.
func identityFor(m ViewerModel, srcID string) string {
	h := fnv.New64a()
	// document-hash
	hashField(h, m.tabPath)
	title, artist := "", ""
	total := int64(0)
	if m.tab != nil {
		title, artist = m.tab.Title, m.tab.Artist
		total = player.ScheduleTotalTicks(player.BuildSchedule(m.tab))
	}
	hashField(h, title)
	hashField(h, artist)
	hashField(h, strconv.FormatInt(total, 10))
	// content-hash of the audio file this source currently points at.
	path := ""
	if idx := m.audioCatalog.FindByID(srcID); idx >= 0 {
		path = m.audioCatalog.Sources[idx].Path
	}
	hashFileContent(h, path)
	return fmt.Sprintf("%016x", h.Sum64())
}

// hashField folds a length-prefixed string into the hash so concatenated
// values never collide ("ab"+"c" != "a"+"bc").
func hashField(h hash.Hash64, s string) {
	fmt.Fprintf(h, "%d:%s", len(s), s)
}

// hashFileContent folds a file's change-detecting attributes into the hash.
// A missing file still hashes deterministically, over size 0.
func hashFileContent(h hash.Hash64, path string) {
	info, err := os.Stat(path)
	if err != nil {
		hashField(h, "0")
		hashField(h, "0")
		return
	}
	hashField(h, strconv.FormatInt(info.Size(), 10))
	hashField(h, strconv.FormatInt(info.ModTime().UnixNano(), 10))
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	const chunk = 64 * 1024
	head := make([]byte, chunk)
	n, _ := io.ReadFull(f, head)
	hashField(h, string(head[:n]))
	if info.Size() >= chunk {
		tail := make([]byte, chunk)
		if _, err := f.Seek(info.Size()-chunk, io.SeekStart); err == nil {
			n, _ := io.ReadFull(f, tail)
			hashField(h, string(tail[:n]))
		}
	}
}
