package viewer

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fretboard/internal/model"
	"fretboard/internal/player"
	"fretboard/internal/ui/msgs"
)

func TestAudioFetchedMsgUpdatesCatalogPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "backing.mp3")
	if err := os.WriteFile(path, []byte("fake"), 0644); err != nil {
		t.Fatal(err)
	}

	m := NewViewerModel()
	m.tab = &model.Tab{Title: "Layla", Artist: "Clapton"}
	m.tabID = 42
	m.selectedSourceIdx = 1
	m.audioCatalog = player.AudioCatalog{
		Sources: []player.AudioSource{
			{ID: "midi", Kind: player.SourceMIDI, Label: "MIDI"},
			{ID: "yt:abc", Kind: player.SourceOnline, Label: "YouTube", VideoID: "abc"},
		},
	}

	updated, _ := m.Update(msgs.AudioFetchedMsg{
		Path:    path,
		Artist:  "Clapton",
		Title:   "Layla",
		TabID:   42,
		TabPath: "online://ug/1",
	})
	m = updated

	if got := m.audioCatalog.Sources[1].Path; got != path {
		t.Fatalf("catalog path = %q, want %q", got, path)
	}
	if m.resolvedAudio != path {
		t.Fatalf("resolvedAudio = %q, want %q", m.resolvedAudio, path)
	}
}

func TestPickAudioSourceIndexPrefersLocal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "backing.mp3")
	if err := os.WriteFile(path, []byte("fake"), 0644); err != nil {
		t.Fatal(err)
	}
	cat := player.AudioCatalog{Sources: []player.AudioSource{
		{ID: "midi", Kind: player.SourceMIDI, Label: "MIDI"},
		{ID: "local:1", Kind: player.SourceLocal, Label: "Local", Path: path},
		{ID: "yt:abc", Kind: player.SourceOnline, Label: "YouTube", VideoID: "abc", Score: 999},
	}}
	if got := pickAudioSourceIndex(nil, cat); got != 1 {
		t.Fatalf("pickAudioSourceIndex = %d, want local index 1", got)
	}
}

func TestPickAudioSourceIndexHonorsMetadata(t *testing.T) {
	cat := player.AudioCatalog{Sources: []player.AudioSource{
		{ID: "midi", Kind: player.SourceMIDI, Label: "MIDI"},
		{ID: "yt:abc", Kind: player.SourceOnline, Label: "YouTube", VideoID: "abc"},
	}}
	tab := &model.Tab{Metadata: map[string]string{"audio_source": "yt:abc"}}
	if got := pickAudioSourceIndex(tab, cat); got != 1 {
		t.Fatalf("pickAudioSourceIndex = %d, want metadata pick 1", got)
	}
}

func TestAudioFetchedMsgIgnoresStaleTabID(t *testing.T) {
	m := NewViewerModel()
	m.tab = &model.Tab{Title: "Layla", Artist: "Clapton"}
	m.tabID = 2
	m.selectedSourceIdx = 1
	m.audioCatalog = player.AudioCatalog{
		Sources: []player.AudioSource{
			{ID: "midi", Kind: player.SourceMIDI, Label: "MIDI"},
			{ID: "yt:abc", Kind: player.SourceOnline, Label: "YouTube", VideoID: "abc"},
		},
	}
	updated, _ := m.Update(msgs.AudioFetchedMsg{
		Path:   "/tmp/stale.mp3",
		Artist: "Clapton",
		Title:  "Layla",
		TabID:  99,
	})
	m = updated
	if m.resolvedAudio != "" {
		t.Fatalf("stale tab id should not update viewer, got %q", m.resolvedAudio)
	}
}

func TestSaveTabPrefsCmdAllowsTabPathWithoutID(t *testing.T) {
	m := NewViewerModel()
	m.tab = &model.Tab{Title: "Layla", Artist: "Clapton", Metadata: map[string]string{"audio_source": "yt:abc"}}
	m.tabPath = "online://ug/2563800"
	if cmd := m.saveTabPrefsCmd(); cmd == nil {
		t.Fatal("expected save cmd when tabPath is set")
	}
}

// TestAudioCatalogMsgKeepsSourcesAndShowsError guards US-9: when the online
// search fails, the catalog must still apply its local/MIDI sources and the
// failure must surface as a message (previously the whole catalog was dropped).
func TestAudioCatalogMsgKeepsSourcesAndShowsError(t *testing.T) {
	m := NewViewerModel()
	m.tab = &model.Tab{Title: "Layla", Artist: "Clapton"}
	m.tabID = 42
	updated, _ := m.Update(msgs.AudioCatalogMsg{
		Catalog: player.AudioCatalog{Sources: []player.AudioSource{
			{ID: "midi", Kind: player.SourceMIDI, Label: "MIDI"},
			{ID: "local:x", Kind: player.SourceLocal, Label: "Local", Path: "x.mp3"},
		}},
		Err:     fmt.Errorf("yt-dlp search timed out"),
		Artist:  "Clapton",
		Title:   "Layla",
		TabID:   42,
		TabPath: "online://ug/1",
	})
	m = updated
	if len(m.audioCatalog.Sources) != 2 {
		t.Fatalf("catalog must keep its sources on error, got %+v", m.audioCatalog.Sources)
	}
	if m.errMsg == "" || !strings.Contains(m.errMsg, "timed out") {
		t.Fatalf("error must surface in errMsg, got %q", m.errMsg)
	}
}

// TestAudioCatalogMsgErrorWithoutSourcesDoesNotCrash guards the empty-catalog
// error path: no sources to keep, just the message.
func TestAudioCatalogMsgErrorWithoutSourcesDoesNotCrash(t *testing.T) {
	m := NewViewerModel()
	m.tab = &model.Tab{Title: "Layla", Artist: "Clapton"}
	m.tabID = 42
	updated, _ := m.Update(msgs.AudioCatalogMsg{
		Catalog: player.AudioCatalog{},
		Err:     fmt.Errorf("network unreachable"),
		Artist:  "Clapton",
		Title:   "Layla",
		TabID:   42,
		TabPath: "online://ug/1",
	})
	m = updated
	if m.errMsg == "" {
		t.Fatal("error must surface")
	}
}

// TestHandleAlignmentSurfacesErr guards F3: an analysis failure (e.g. no
// audio decoder) must surface as an error message and must never apply
// partial alignment state (tempo, offset, auto tempo map).
func TestHandleAlignmentSurfacesErr(t *testing.T) {
	m := NewViewerModel()
	tab := &model.Tab{Title: "X", Artist: "Y", Tuning: model.Standard,
		Bars: []model.Bar{{Strings: []model.StringLine{{Segments: []model.Segment{{Char: '0', Value: 0, Position: 0, Width: 1}}}}}}}
	m.LoadTab(tab, "x.txt", 0)
	audio := filepath.Join(t.TempDir(), "song.mp3")
	if err := os.WriteFile(audio, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	m.audioCatalog = player.AudioCatalog{Sources: []player.AudioSource{
		{ID: "midi", Kind: player.SourceMIDI, Label: "MIDI"},
		{ID: "local:" + audio, Kind: player.SourceLocal, Label: "song.mp3", Path: audio, Category: player.CatLocal, StrictOK: true},
	}}
	m.selectedSourceIdx = 1

	m, _ = m.Update(msgs.AlignmentMsg{
		SourceID: "local:" + audio, BPM: 118, Offset: 3200 * time.Millisecond, Confidence: 0.85,
		Artist: "Y", Title: "X", TabID: 0, TabPath: "x.txt",
		Anchors: []player.SyncPoint{{Bar: 1, Seconds: 1}, {Bar: 2, Seconds: 3}},
		Err:     errors.New("no audio decoder available (ffmpeg or mpg123)"),
	})
	if m.errMsg == "" || !strings.Contains(m.errMsg, "Audio analysis failed") {
		t.Fatalf("analysis failure must surface in errMsg, got %q", m.errMsg)
	}
	if m.bpm != 120 {
		t.Fatalf("failed analysis must not change the tempo, got %d", m.bpm)
	}
	if m.audioOffset != 0 {
		t.Fatalf("failed analysis must not apply an offset, got %v", m.audioOffset)
	}
	if m.autoAnchors != nil || m.autoActive {
		t.Fatal("failed analysis must not apply the auto tempo map")
	}
	if strings.Contains(m.infoMsg, "Auto-aligned") {
		t.Fatalf("failed analysis must not announce success, got %q", m.infoMsg)
	}
}

func TestSyncPointsZeroBased(t *testing.T) {
	points := []player.SyncPoint{{Bar: 1, Seconds: 5}, {Bar: 2, Seconds: 20}, {Bar: 5, Seconds: 40}}
	got := syncPointsZeroBased(points)
	want := []player.SyncPoint{{Bar: 0, Seconds: 5}, {Bar: 1, Seconds: 20}, {Bar: 4, Seconds: 40}}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("point %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	// Input must not be mutated (persisted state stays 1-based).
	if points[0].Bar != 1 || points[2].Bar != 5 {
		t.Fatalf("input mutated: %+v", points)
	}
}

func TestAdjustAudioOffsetPersistsAndRounds(t *testing.T) {
	tab := &model.Tab{Title: "T", Artist: "A", Metadata: map[string]string{}}
	m := NewViewerModel()
	m.LoadTab(tab, "/tmp/off.txt", 42)

	m, _ = m.adjustAudioOffset("]")
	if m.audioOffset != 0.5 {
		t.Fatalf("nudge up: got %v want 0.5", m.audioOffset)
	}
	if got := tab.Metadata[model.MetaKeyAudioOffset]; got != "0.5" {
		t.Fatalf("metadata: got %q want 0.5", got)
	}
	m, _ = m.adjustAudioOffset("[")
	if m.audioOffset != 0 {
		t.Fatalf("nudge down: got %v want 0", m.audioOffset)
	}
	m, _ = m.adjustAudioOffset("}")
	if m.audioOffset != 5 {
		t.Fatalf("big nudge: got %v want 5", m.audioOffset)
	}
	m, _ = m.adjustAudioOffset("o")
	if m.audioOffset != 0 {
		t.Fatalf("reset: got %v want 0", m.audioOffset)
	}
	if got := tab.Metadata[model.MetaKeyAudioOffset]; got != "0.0" {
		t.Fatalf("metadata after reset: got %q want 0.0", got)
	}
}

func TestLoadTabRestoresAudioOffset(t *testing.T) {
	tab := &model.Tab{Title: "T", Artist: "A", Metadata: map[string]string{model.MetaKeyAudioOffset: "2.5"}}
	m := NewViewerModel()
	m.LoadTab(tab, "/tmp/off2.txt", 7)
	if m.audioOffset != 2.5 {
		t.Fatalf("restored offset: got %v want 2.5", m.audioOffset)
	}
}

func TestParseSyncPoints(t *testing.T) {
	if got := parseSyncPoints(`[{"bar":1,"seconds":10.5},{"bar":3,"seconds":25},{"bar":1,"seconds":99}]`); len(got) != 2 || got[0].Bar != 1 || got[0].Seconds != 10.5 || got[1].Bar != 3 {
		t.Fatalf("parseSyncPoints dedupe/sort wrong: %+v", got)
	}
	if got := parseSyncPoints("garbage"); got != nil {
		t.Fatalf("garbage should yield nil, got %+v", got)
	}
	if got := parseSyncPoints(""); got != nil {
		t.Fatalf("empty should yield nil, got %+v", got)
	}
}

func TestSaveSyncPointsPersistsJSON(t *testing.T) {
	m := NewViewerModel()
	m.LoadTab(sampleTab(), "", 0)
	m.syncPoints = []player.SyncPoint{{Bar: 1, Seconds: 3}, {Bar: 5, Seconds: 20}}
	m.saveSyncPoints()
	raw := m.tab.Metadata[model.MetaKeySyncPoints]
	if raw == "" {
		t.Fatal("sync_points metadata not written")
	}
	if !strings.Contains(raw, `5`) || !strings.Contains(raw, `20`) {
		t.Fatalf("sync_points JSON missing anchor: %s", raw)
	}
	back := parseSyncPoints(raw)
	if len(back) != 2 {
		t.Fatalf("round trip should yield 2 anchors, got %+v", back)
	}
}

func TestSetSyncPointAnchorsCurrentBar(t *testing.T) {
	m := NewViewerModel()
	m.LoadTab(sampleTab(), "", 5)
	m.cursorBar = 3
	m.playing = true
	m.audioSync = true
	m, cmd := m.setSyncPoint()
	if len(m.syncPoints) != 1 || m.syncPoints[0].Bar != 4 {
		t.Fatalf("sync point should anchor bar 4, got %+v", m.syncPoints)
	}
	if cmd == nil {
		t.Fatal("setting a sync point should persist tab prefs")
	}
}

// TestSyncBarUndoRemovesLastAnchor guards S6.1: S removes the most recent
// sync anchor instead of wiping all of them.
func TestSyncBarUndoRemovesLastAnchor(t *testing.T) {
	m := NewViewerModel()
	tab := &model.Tab{Title: "X", Tuning: model.Standard,
		Bars: []model.Bar{{Strings: []model.StringLine{{Segments: []model.Segment{{Char: '0', Value: 0, Position: 0, Width: 1}}}}}}}
	m.LoadTab(tab, "x.txt", 0)
	m.syncPoints = []player.SyncPoint{{Bar: 1, Seconds: 0.5}, {Bar: 3, Seconds: 12.0}, {Bar: 5, Seconds: 25.0}}

	m, _ = m.Update(key("S"))
	if len(m.syncPoints) != 2 || m.syncPoints[1].Bar != 3 {
		t.Fatalf("S should drop the last anchor, got %+v", m.syncPoints)
	}
	if !strings.Contains(m.infoMsg, "Removed sync anchor at bar 5") {
		t.Fatalf("expected an undo message, got %q", m.infoMsg)
	}
	m, _ = m.Update(key("S"))
	m, _ = m.Update(key("S"))
	if len(m.syncPoints) != 0 {
		t.Fatalf("repeated S should remove all anchors, got %+v", m.syncPoints)
	}
	m, _ = m.Update(key("S"))
	if !strings.Contains(m.errMsg, "No sync points") {
		t.Fatalf("S on empty anchors should say so, got %q", m.errMsg)
	}
}

// TestOffsetResetUndoRestores guards S6.1: o resets the offset and pressing
// o again restores the previous value.
func TestOffsetResetUndoRestores(t *testing.T) {
	m := NewViewerModel()
	tab := &model.Tab{Title: "X", Tuning: model.Standard,
		Bars: []model.Bar{{Strings: []model.StringLine{{Segments: []model.Segment{{Char: '0', Value: 0, Position: 0, Width: 1}}}}}}}
	m.LoadTab(tab, "x.txt", 0)
	m.audioOffset = 3.5

	m, _ = m.Update(key("o"))
	if m.audioOffset != 0 {
		t.Fatalf("o should reset the offset, got %v", m.audioOffset)
	}
	m, _ = m.Update(key("o"))
	if m.audioOffset != 3.5 {
		t.Fatalf("second o should restore the previous offset, got %v", m.audioOffset)
	}
}

// TestManualPickStickyAcrossRefresh guards S6.3: a manually chosen audio
// source survives a catalog refresh (auto-pick must not snap back).
func TestManualPickStickyAcrossRefresh(t *testing.T) {
	m := NewViewerModel()
	tab := &model.Tab{Title: "X", Tuning: model.Standard,
		Bars: []model.Bar{{Strings: []model.StringLine{{Segments: []model.Segment{{Char: '0', Value: 0, Position: 0, Width: 1}}}}}}}
	m.LoadTab(tab, "x.txt", 0)
	cat := player.AudioCatalog{Sources: []player.AudioSource{
		{ID: "midi", Kind: player.SourceMIDI, Label: "MIDI"},
		{ID: "yt:abc", Kind: player.SourceOnline, Label: "Studio", Category: player.CatOfficial, StrictOK: true, Score: 500},
		{ID: "yt:live", Kind: player.SourceOnline, Label: "Live", Category: player.CatLive, StrictOK: false, Score: 100},
	}}
	m.audioCatalog = cat
	m.selectedSourceIdx = 2 // user picked the live version deliberately
	m.manualPick = true
	m.strictAudio = true

	// A refreshed catalog (same sources) must keep the manual pick.
	m, _ = m.Update(msgs.AudioCatalogMsg{Catalog: cat, TabID: 0, TabPath: "x.txt", Artist: "", Title: "X"})
	if m.selectedSourceIdx != 2 {
		t.Fatalf("manual pick should survive refresh, got idx %d", m.selectedSourceIdx)
	}
	// If the picked source disappears, fall back to auto-pick (MIDI-safe).
	shrunken := player.AudioCatalog{Sources: []player.AudioSource{
		{ID: "midi", Kind: player.SourceMIDI, Label: "MIDI"},
		{ID: "yt:abc", Kind: player.SourceOnline, Label: "Studio", Category: player.CatOfficial, StrictOK: true, Score: 500},
	}}
	m, _ = m.Update(msgs.AudioCatalogMsg{Catalog: shrunken, TabID: 0, TabPath: "x.txt", Artist: "", Title: "X"})
	if m.selectedSourceIdx != 1 {
		t.Fatalf("missing source should fall back to auto-pick, got idx %d", m.selectedSourceIdx)
	}
}

// TestRejectWrongSource guards the wrong-version feedback loop: w records
// the current source as rejected, re-picks the next candidate, persists the
// rejection, and the picker badges it.
func TestRejectWrongSource(t *testing.T) {
	m := NewViewerModel()
	tab := &model.Tab{Title: "X", Tuning: model.Standard,
		Bars: []model.Bar{{Strings: []model.StringLine{{Segments: []model.Segment{{Char: '0', Value: 0, Position: 0, Width: 1}}}}}}}
	m.LoadTab(tab, "x.txt", 0)
	m.strictAudio = true
	m.audioCatalog = player.AudioCatalog{Sources: []player.AudioSource{
		{ID: "midi", Kind: player.SourceMIDI, Label: "MIDI"},
		{ID: "yt:live", Kind: player.SourceOnline, Label: "Live version", Category: player.CatLive, StrictOK: false, Score: 100},
		{ID: "yt:studio", Kind: player.SourceOnline, Label: "Studio version", Category: player.CatOfficial, StrictOK: true, Score: 500},
	}}
	m.selectedSourceIdx = 2
	m.manualPick = true

	// Reject the studio pick; the next strict-compatible candidate is MIDI.
	m, _ = m.Update(key("w"))
	if m.selectedSourceIdx != 0 {
		t.Fatalf("w should re-pick the next candidate, got idx %d", m.selectedSourceIdx)
	}
	rej := rejectedSources(m.tab)
	if !rej["yt:studio"] {
		t.Fatal("the rejected source should be persisted in metadata")
	}
	if !strings.Contains(m.infoMsg, "Rejected") {
		t.Fatalf("expected a rejection message, got %q", m.infoMsg)
	}
	// Picker badges rejected sources.
	body := renderAudioPickerBody(m.audioCatalog, 0, false, true, 0, rej)
	if !strings.Contains(body, "rejected") {
		t.Fatalf("picker should badge the rejected source:\n%s", body)
	}
}

// TestDriftNudge guards the one-time hint when the recording's tempo
// differs from the tab's.
func TestDriftNudge(t *testing.T) {
	if got := driftNudge(117, 120); got == "" || !strings.Contains(got, "drift") {
		t.Fatalf("3 BPM difference should produce a nudge, got %q", got)
	}
	if got := driftNudge(120, 120); got != "" {
		t.Fatalf("matching tempos must not nudge, got %q", got)
	}
	if got := driftNudge(0, 120); got != "" {
		t.Fatalf("underviable tempo must not nudge, got %q", got)
	}
}

// TestAutoAlignmentApplied guards the alignment integration: a confident
// result sets the tempo and per-source offset; a weak one only hints.
func TestAutoAlignmentApplied(t *testing.T) {
	m := NewViewerModel()
	tab := &model.Tab{Title: "X", Artist: "Y", Tuning: model.Standard,
		Bars: []model.Bar{{Strings: []model.StringLine{{Segments: []model.Segment{{Char: '0', Value: 0, Position: 0, Width: 1}}}}}}}
	m.LoadTab(tab, "x.txt", 0)
	audio := filepath.Join(t.TempDir(), "song.mp3")
	if err := os.WriteFile(audio, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	m.audioCatalog = player.AudioCatalog{Sources: []player.AudioSource{
		{ID: "midi", Kind: player.SourceMIDI, Label: "MIDI"},
		{ID: "local:" + audio, Kind: player.SourceLocal, Label: "song.mp3", Path: audio, Category: player.CatLocal, StrictOK: true},
	}}
	m.selectedSourceIdx = 1
	m.restoreCalibrationForSource()

	m, _ = m.Update(msgs.AlignmentMsg{SourceID: "local:" + audio, BPM: 118, Offset: 3200 * time.Millisecond,
		Confidence: 0.85, Artist: "Y", Title: "X", TabID: 0, TabPath: "x.txt"})
	if m.bpm != 118 {
		t.Fatalf("bpm should be aligned to 118, got %d", m.bpm)
	}
	if m.audioOffset != 3.2 {
		t.Fatalf("offset should be aligned to 3.2s, got %v", m.audioOffset)
	}
	if !strings.Contains(m.infoMsg, "Auto-aligned") {
		t.Fatalf("expected an auto-aligned message, got %q", m.infoMsg)
	}
	if m.tab.Metadata["audio_aligned:local:"+audio] != "1" {
		t.Fatal("source should be marked aligned")
	}

	// A weak result is presented for user confirmation; it never touches
	// the calibration (the never-auto-apply-below-0.6 invariant).
	m, _ = m.Update(msgs.AlignmentCandidatesMsg{
		SourceID: "local:" + audio,
		Candidates: []player.Candidate{{
			Alignment: player.Alignment{BPM: 100, Offset: 0, Confidence: 0.45},
			Coverage:  0.45,
			Partial:   true,
		}},
		Artist: "Y", Title: "X", TabID: 0, TabPath: "x.txt",
	})
	if m.bpm != 118 || m.audioOffset != 3.2 {
		t.Fatalf("presented alignment must not auto-apply: bpm=%d offset=%v", m.bpm, m.audioOffset)
	}
	if !m.showAlignmentConfirm {
		t.Fatal("a weak alignment must be presented for user confirmation")
	}

	// A stale source is ignored entirely.
	m, _ = m.Update(msgs.AlignmentMsg{SourceID: "yt:other", BPM: 140, Offset: 0,
		Confidence: 0.9, Artist: "Y", Title: "X", TabID: 0, TabPath: "x.txt"})
	if m.bpm != 118 {
		t.Fatalf("stale source must be ignored, bpm=%d", m.bpm)
	}
}

// TestHandleAlignmentCandidatesPresentsTop3 guards the present band: a
// candidates message opens the confirm overlay and stores the ranked list
// without applying anything.
func TestHandleAlignmentCandidatesPresentsTop3(t *testing.T) {
	m, audio := bpmTestViewer(t)
	m, _ = m.Update(msgs.AlignmentCandidatesMsg{
		SourceID: "local:" + audio,
		Candidates: []player.Candidate{
			{Alignment: player.Alignment{BPM: 118, Offset: 3200 * time.Millisecond, Confidence: 0.55}, Coverage: 0.6},
			{Alignment: player.Alignment{BPM: 236, Offset: 3200 * time.Millisecond, Confidence: 0.5}, Coverage: 0.5},
		},
		Artist: "Y", Title: "X", TabID: 0, TabPath: "x.txt",
	})
	if !m.showAlignmentConfirm {
		t.Fatal("candidates must open the confirm overlay")
	}
	if len(m.alignmentCandidates) != 2 {
		t.Fatalf("candidates stored = %d, want 2", len(m.alignmentCandidates))
	}
	if m.bpm != 120 {
		t.Fatalf("presenting must not apply, bpm=%d", m.bpm)
	}
}

// TestConfirmAcceptApplies guards the accept key: pressing 1 in the confirm
// overlay applies the first candidate exactly like the auto-apply path and
// closes the overlay.
func TestConfirmAcceptApplies(t *testing.T) {
	m, _ := bpmTestViewer(t)
	onsets := []time.Duration{
		3200 * time.Millisecond, 3705 * time.Millisecond, 4210 * time.Millisecond, 4715 * time.Millisecond,
	}
	m.showAlignmentConfirm = true
	m.alignmentCandidates = []player.Candidate{{
		Alignment: player.Alignment{BPM: 118, Offset: 3200 * time.Millisecond, Confidence: 0.55,
			Onsets: onsets, Strengths: []float64{1, 0.5, 0.5, 0.5}},
		Coverage: 0.6,
	}}
	m, _ = m.Update(key("1"))
	if m.bpm != 118 {
		t.Fatalf("accepting must apply the tempo, got %d", m.bpm)
	}
	if m.audioOffset != 3.2 {
		t.Fatalf("accepting must apply the offset, got %v", m.audioOffset)
	}
	if m.showAlignmentConfirm {
		t.Fatal("accepting must close the confirm overlay")
	}
}

// TestConfirmVariantApplies guards the variant keys: a/b/c/d apply the
// picked candidate with that +- half-beat / +- one-bar offset.
func TestConfirmVariantApplies(t *testing.T) {
	m, _ := bpmTestViewer(t)
	m.showAlignmentConfirm = true
	m.alignmentCandidates = []player.Candidate{{
		Alignment: player.Alignment{BPM: 120, Offset: 3200 * time.Millisecond, Confidence: 0.55},
		Coverage:  0.6,
		Variants: []player.OffsetVariant{
			{Label: "half beat early", Offset: 2950 * time.Millisecond},
			{Label: "half beat late", Offset: 3450 * time.Millisecond},
			{Label: "one bar early", Offset: 1200 * time.Millisecond},
			{Label: "one bar late", Offset: 5200 * time.Millisecond},
		},
	}}
	m, _ = m.Update(key("a"))
	if m.audioOffset != 2.95 {
		t.Fatalf("variant accept must apply the half-beat-early offset, got %v", m.audioOffset)
	}
	if m.bpm != 120 {
		t.Fatalf("variant accept must apply the tempo, got %d", m.bpm)
	}
	if m.showAlignmentConfirm {
		t.Fatal("variant accept must close the overlay")
	}
}

// TestConfirmEscDismisses guards dismissal: esc closes the confirm overlay
// without applying anything.
func TestConfirmEscDismisses(t *testing.T) {
	m, _ := bpmTestViewer(t)
	m.showAlignmentConfirm = true
	m.alignmentCandidates = []player.Candidate{{Alignment: player.Alignment{BPM: 118, Offset: 3200 * time.Millisecond}}}
	m, _ = m.Update(key("esc"))
	if m.showAlignmentConfirm {
		t.Fatal("esc must dismiss the confirm overlay")
	}
	if m.bpm != 120 {
		t.Fatalf("dismissing must not apply, bpm=%d", m.bpm)
	}
}

// bpmTestViewer returns a viewer with a ready local source selected and its
// calibration restored, matching the fixture used by the alignment tests.
func bpmTestViewer(t *testing.T) (ViewerModel, string) {
	t.Helper()
	m := NewViewerModel()
	tab := &model.Tab{Title: "X", Artist: "Y", Tuning: model.Standard,
		Metadata: map[string]string{},
		Bars:     []model.Bar{{Strings: []model.StringLine{{Segments: []model.Segment{{Char: '0', Value: 0, Position: 0, Width: 1}}}}}}}
	m.LoadTab(tab, "x.txt", 0)
	audio := filepath.Join(t.TempDir(), "song.mp3")
	if err := os.WriteFile(audio, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	m.audioCatalog = player.AudioCatalog{Sources: []player.AudioSource{
		{ID: "midi", Kind: player.SourceMIDI, Label: "MIDI"},
		{ID: "local:" + audio, Kind: player.SourceLocal, Label: "song.mp3", Path: audio, Category: player.CatLocal, StrictOK: true},
	}}
	m.selectedSourceIdx = 1
	m.restoreCalibrationForSource()
	return m, audio
}

// TestApplySelectedSourceDeferredProbe guards the async BPM split: the
// state-only apply resolves the source's audio path and returns the command
// that derives the tempo in the background — it must never probe synchronously,
// so the viewer's tempo stays untouched until the BPMDerivedMsg is handled.
func TestApplySelectedSourceDeferredProbe(t *testing.T) {
	m, audio := bpmTestViewer(t)
	before := m.bpm

	cmd := m.applySelectedSourceStateOnly()
	if m.resolvedAudio != audio {
		t.Fatalf("resolvedAudio = %q, want %q", m.resolvedAudio, audio)
	}
	if cmd == nil {
		t.Fatal("state-only apply must return the async BPM derive command")
	}
	if m.bpm != before {
		t.Fatalf("bpm must stay untouched until the derived message is handled, got %d, want %d", m.bpm, before)
	}
}

// TestApplySelectedSourceSkipsProbeWhenBPMKnown guards the command-creation
// guard: a tab that already records a tempo gets no probe command at all.
func TestApplySelectedSourceSkipsProbeWhenBPMKnown(t *testing.T) {
	m, _ := bpmTestViewer(t)
	m.tab.Metadata[model.MetaKeyBPM] = "140"

	if cmd := m.applySelectedSourceStateOnly(); cmd != nil {
		t.Fatal("no probe command when BPM metadata is already set")
	}
}

// TestHandleBPMDerivedAppliesWhenCurrent guards the async apply: a derived
// tempo for the still-current source is applied and clamped.
func TestHandleBPMDerivedAppliesWhenCurrent(t *testing.T) {
	m, audio := bpmTestViewer(t)

	updated, _ := m.Update(msgs.BPMDerivedMsg{SourceID: "local:" + audio, BPM: 132})
	m = updated
	if m.bpm != 132 {
		t.Fatalf("bpm should be derived to 132, got %d", m.bpm)
	}
}

// TestHandleBPMDerivedIgnoresWhenStale guards the stale-source check: a probe
// that finished after the user switched sources must not change the tempo.
func TestHandleBPMDerivedIgnoresWhenStale(t *testing.T) {
	m, _ := bpmTestViewer(t)

	updated, _ := m.Update(msgs.BPMDerivedMsg{SourceID: "yt:other", BPM: 132})
	m = updated
	if m.bpm != 120 {
		t.Fatalf("stale source must be ignored, bpm=%d", m.bpm)
	}
}

// TestHandleBPMDerivedRespectsMetaBPM guards the apply-site guard: a tempo
// already recorded in the tab's metadata wins over the derived value.
func TestHandleBPMDerivedRespectsMetaBPM(t *testing.T) {
	m, audio := bpmTestViewer(t)
	m.bpm = 140 // the tempo the recorded metadata is in effect at
	m.tab.Metadata[model.MetaKeyBPM] = "140"

	updated, _ := m.Update(msgs.BPMDerivedMsg{SourceID: "local:" + audio, BPM: 99})
	m = updated
	if m.bpm != 140 {
		t.Fatalf("recorded BPM metadata must win, got %d, want 140", m.bpm)
	}
}

// alignmentTestViewer returns a viewer with a single local source pointing at
// audio, its calibration restored, ready for alignment.
func alignmentTestViewer(t *testing.T, audio string) (ViewerModel, string) {
	t.Helper()
	m := NewViewerModel()
	tab := &model.Tab{Title: "X", Artist: "Y", Tuning: model.Standard,
		Metadata: map[string]string{},
		Bars:     []model.Bar{{Strings: []model.StringLine{{Segments: []model.Segment{{Char: '0', Value: 0, Position: 0, Width: 1}}}}}}}
	m.LoadTab(tab, "x.txt", 0)
	id := "local:" + audio
	m.audioCatalog = player.AudioCatalog{Sources: []player.AudioSource{
		{ID: "midi", Kind: player.SourceMIDI, Label: "MIDI"},
		{ID: id, Kind: player.SourceLocal, Label: "song.mp3", Path: audio, Category: player.CatLocal, StrictOK: true},
	}}
	m.selectedSourceIdx = 1
	m.restoreCalibrationForSource()
	return m, id
}

// TestIdentityStableForUnchangedFile guards the identity fingerprint: the
// same file and tab produce the same identity on every call, and a different
// file produces a different one.
func TestIdentityStableForUnchangedFile(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.mp3")
	b := filepath.Join(dir, "b.mp3")
	if err := os.WriteFile(a, bytes.Repeat([]byte("A"), 100), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, bytes.Repeat([]byte("B"), 100), 0o644); err != nil {
		t.Fatal(err)
	}
	mA, idA := alignmentTestViewer(t, a)
	mB, idB := alignmentTestViewer(t, b)

	if got, want := identityFor(mA, idA), identityFor(mA, idA); got != want {
		t.Fatalf("same file twice = %q and %q, want equal", got, want)
	}
	if got, want := identityFor(mA, idA), identityFor(mB, idB); got == want {
		t.Fatal("different files must produce different identities")
	}
}

// TestIdentityIncludesDocument guards the document-hash: editing the tab
// (here its title) changes the identity even when the audio file is the same,
// so a stale alignment cannot survive a tab edit.
func TestIdentityIncludesDocument(t *testing.T) {
	audio := filepath.Join(t.TempDir(), "song.mp3")
	if err := os.WriteFile(audio, bytes.Repeat([]byte("A"), 100), 0o644); err != nil {
		t.Fatal(err)
	}
	m, id := alignmentTestViewer(t, audio)
	before := identityFor(m, id)
	m.tab.Title = "Different Title"
	after := identityFor(m, id)
	if before == after {
		t.Fatal("editing the tab title must change the identity")
	}
}

// TestAlignmentInvalidatedWhenAudioChanges guards the alignment identity
// invalidation: an alignment persisted for a file is restored only while the
// file is unchanged. Swapping the audio file (different size/content) for the
// same source must clear the stored tempo map and aligned marker, skip
// restoring the auto anchors, and let the alignment analysis re-run.
func TestAlignmentInvalidatedWhenAudioChanges(t *testing.T) {
	audio := filepath.Join(t.TempDir(), "song.mp3")
	if err := os.WriteFile(audio, bytes.Repeat([]byte("A"), 100), 0o644); err != nil {
		t.Fatal(err)
	}
	m, id := alignmentTestViewer(t, audio)

	// Align: the analysis runs once and the result is persisted (tempo map,
	// aligned marker, identity) under the source's keys.
	if cmd := m.maybeAlignCmd(); cmd == nil {
		t.Fatal("first align must return a command")
	}
	storedIdentity := identityFor(m, id)
	m, _ = m.Update(msgs.AlignmentMsg{
		SourceID: id, BPM: 118, Offset: 3200 * time.Millisecond, Confidence: 0.85,
		Artist: "Y", Title: "X", TabID: 0, TabPath: "x.txt",
		Anchors: []player.SyncPoint{{Bar: 1, Seconds: 1}, {Bar: 2, Seconds: 3}},
	})
	if m.tab.Metadata["tempo_map:"+id] == "" {
		t.Fatal("alignment must persist a tempo map")
	}
	if m.tab.Metadata["audio_identity:"+id] != storedIdentity {
		t.Fatalf("alignment must persist the identity, got %q want %q",
			m.tab.Metadata["audio_identity:"+id], storedIdentity)
	}

	// Swap the file for a different one: same source, different audio.
	if err := os.WriteFile(audio, bytes.Repeat([]byte("B"), 200), 0o644); err != nil {
		t.Fatal(err)
	}

	// Restoring must detect the mismatch and drop the stale alignment.
	m.restoreCalibrationForSource()
	if m.autoAnchors != nil || m.autoActive {
		t.Fatal("stale alignment must not restore auto anchors after the file changed")
	}
	if m.tab.Metadata["tempo_map:"+id] != "" {
		t.Fatal("stale tempo map must be cleared")
	}
	if m.tab.Metadata["audio_aligned:"+id] != "" {
		t.Fatal("stale aligned marker must be cleared")
	}

	// A re-run of the alignment must be allowed (identity mismatch).
	if cmd := m.maybeAlignCmd(); cmd == nil {
		t.Fatal("maybeAlignCmd must re-run after the file changed")
	}
}
