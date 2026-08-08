package viewer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fretboard/internal/model"
	"fretboard/internal/player"
	"fretboard/internal/ui/msgs"
)

func mixedCatalog(localPath string) player.AudioCatalog {
	return player.AudioCatalog{Sources: []player.AudioSource{
		{ID: "midi", Kind: player.SourceMIDI, Label: "MIDI"},
		{ID: "yt:live", Kind: player.SourceOnline, Label: "Live at Wembley", Category: player.CatLive, Score: 900},
		{ID: "yt:cover", Kind: player.SourceOnline, Label: "Cover", Category: player.CatCover, Score: 800},
		{ID: "yt:official", Kind: player.SourceOnline, Label: "Official Audio", Category: player.CatOfficial, StrictOK: true, Score: 500},
		{ID: "yt:lesson", Kind: player.SourceOnline, Label: "Guitar Lesson", Category: player.CatLesson, Score: 700},
		{ID: "local:path", Kind: player.SourceLocal, Label: "local.mp3", Path: localPath, Category: player.CatLocal, StrictOK: true, Score: 100},
	}}
}

// TestPickStrictAudioSourceIndexSkipsLiveCover guards US-11: strict auto-pick
// must skip higher-scored live/cover/lesson candidates and choose the best
// strict-compatible (official) one.
func TestPickStrictAudioSourceIndexSkipsLiveCover(t *testing.T) {
	cat := mixedCatalog("")
	if got := pickStrictAudioSourceIndex(nil, cat); got != 3 {
		t.Fatalf("strict pick = %d, want official index 3", got)
	}
}

// TestPickStrictPrefersLocal guards US-11: a ready local file always wins
// strict auto-pick (the user chose it deliberately).
func TestPickStrictPrefersLocal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "backing.mp3")
	if err := os.WriteFile(path, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := pickStrictAudioSourceIndex(nil, mixedCatalog(path)); got != 5 {
		t.Fatalf("strict pick = %d, want local index 5", got)
	}
}

// TestPickStrictFallsBackToMidi guards US-11: when every online candidate is
// rejected, strict auto-pick returns MIDI rather than a mismatched recording.
func TestPickStrictFallsBackToMidi(t *testing.T) {
	cat := player.AudioCatalog{Sources: []player.AudioSource{
		{ID: "midi", Kind: player.SourceMIDI},
		{ID: "yt:live", Kind: player.SourceOnline, Category: player.CatLive, StrictOK: false, Score: 900},
	}}
	if got := pickStrictAudioSourceIndex(nil, cat); got != 0 {
		t.Fatalf("strict pick = %d, want MIDI (0)", got)
	}
}

// TestPerSourceCalibrationRestoredOnSourceSwitch guards US-12: the offset and
// anchors are read from the selected source's keys first, then legacy.
func TestPerSourceCalibrationRestoredOnSourceSwitch(t *testing.T) {
	m := NewViewerModel()
	m.audioCatalog = mixedCatalog("")
	m.tab = &model.Tab{
		Title: "T", Artist: "A",
		Metadata: map[string]string{
			"audio_offset":             "1.5",
			"audio_offset:yt:official": "8.5",
			"sync_points":              `[{"bar":1,"seconds":1},{"bar":4,"seconds":30}]`,
			"sync_points:yt:official":  `[{"bar":1,"seconds":8},{"bar":4,"seconds":40}]`,
		},
	}
	m.selectedSourceIdx = 3 // yt:official
	m.restoreCalibrationForSource()
	if m.audioOffset != 8.5 {
		t.Fatalf("offset = %v, want per-source 8.5", m.audioOffset)
	}
	if len(m.syncPoints) != 2 || m.syncPoints[1].Seconds != 40 {
		t.Fatalf("anchors should come from the per-source key, got %+v", m.syncPoints)
	}

	// Switching to MIDI (no per-source key) falls back to the legacy values.
	m.selectedSourceIdx = 0
	m.restoreCalibrationForSource()
	if m.audioOffset != 1.5 {
		t.Fatalf("offset = %v, want legacy 1.5 after switching to MIDI", m.audioOffset)
	}
}

// TestAdjustAudioOffsetWritesPerSourceKey guards US-12: nudges persist under
// the current source's key and mirror the legacy key.
func TestAdjustAudioOffsetWritesPerSourceKey(t *testing.T) {
	m := NewViewerModel()
	m.tab = &model.Tab{Title: "T", Artist: "A", Metadata: map[string]string{}}
	m.audioCatalog = mixedCatalog("")
	m.selectedSourceIdx = 3 // yt:official
	updated, _ := m.adjustAudioOffset("]")
	m = updated
	if m.tab.Metadata["audio_offset:yt:official"] != "0.5" {
		t.Fatalf("per-source offset key = %q, want 0.5", m.tab.Metadata["audio_offset:yt:official"])
	}
	if m.tab.Metadata["audio_offset"] != "0.5" {
		t.Fatalf("legacy offset key = %q, want 0.5", m.tab.Metadata["audio_offset"])
	}
}

// TestAdjustAudioOffsetFineNudge guards US-14's 0.1 s nudge keys.
func TestAdjustAudioOffsetFineNudge(t *testing.T) {
	m := NewViewerModel()
	m.tab = &model.Tab{Title: "T", Artist: "A", Metadata: map[string]string{}}
	m, _ = m.adjustAudioOffset(",")
	if m.audioOffset != -0.1 {
		t.Fatalf(", nudge = %v, want -0.1", m.audioOffset)
	}
	m, _ = m.adjustAudioOffset(".")
	if m.audioOffset != 0 {
		t.Fatalf(". nudge = %v, want 0", m.audioOffset)
	}
}

// TestSyncPointsWrittenPerSource guards US-12: anchors persist under the
// current source's key.
func TestSyncPointsWrittenPerSource(t *testing.T) {
	m := NewViewerModel()
	m.tab = &model.Tab{Title: "T", Artist: "A", Metadata: map[string]string{}}
	m.audioCatalog = mixedCatalog("")
	m.selectedSourceIdx = 3 // yt:official
	m.syncPoints = []player.SyncPoint{{Bar: 2, Seconds: 12}}
	m.saveSyncPoints()
	raw := m.tab.Metadata["sync_points:yt:official"]
	if raw == "" || !strings.Contains(raw, `"bar":2`) {
		t.Fatalf("per-source sync key = %q, want bar 2 anchor", raw)
	}
	if m.tab.Metadata["sync_points"] != raw {
		t.Fatal("legacy sync key should mirror the per-source value")
	}
}

// TestIntroDetectedMsgAppliesOffset guards US-14: an auto-detected intro is
// applied only to an uncalibrated source and marked so it never re-runs.
func TestIntroDetectedMsgAppliesOffset(t *testing.T) {
	m := NewViewerModel()
	tab := &model.Tab{Title: "T", Artist: "A", Metadata: map[string]string{}}
	m.tab = tab
	m.tabID = 7
	m.audioCatalog = mixedCatalog("")
	m.selectedSourceIdx = 3 // yt:official
	updated, _ := m.Update(msgs.IntroDetectedMsg{
		SourceID: "yt:official", Offset: 3 * time.Second, TabID: 7, Artist: "A", Title: "T",
	})
	m = updated
	if m.audioOffset != 3 {
		t.Fatalf("offset = %v, want 3", m.audioOffset)
	}
	if m.tab.Metadata["audio_offset:yt:official"] != "3.0" {
		t.Fatalf("per-source offset = %q, want 3.0", m.tab.Metadata["audio_offset:yt:official"])
	}
	if m.tab.Metadata["audio_offset_auto:yt:official"] != "1" {
		t.Fatal("auto-detection marker must be set")
	}
	if m.infoMsg == "" {
		t.Fatal("the auto-detection should announce itself")
	}
}

// TestIntroDetectedMsgIgnoresCalibrated guards US-14: manual calibration
// wins over auto-detection.
func TestIntroDetectedMsgIgnoresCalibrated(t *testing.T) {
	m := NewViewerModel()
	m.tab = &model.Tab{Title: "T", Artist: "A", Metadata: map[string]string{}}
	m.tabID = 7
	m.audioCatalog = mixedCatalog("")
	m.selectedSourceIdx = 3
	m.audioOffset = 2.5
	updated, _ := m.Update(msgs.IntroDetectedMsg{
		SourceID: "yt:official", Offset: 3 * time.Second, TabID: 7, Artist: "A", Title: "T",
	})
	m = updated
	if m.audioOffset != 2.5 {
		t.Fatalf("manual offset must not be overwritten, got %v", m.audioOffset)
	}
}

// TestIntroDetectedMsgIgnoresStaleSource guards US-14: a probe from a source
// the user has switched away from must not apply.
func TestIntroDetectedMsgIgnoresStaleSource(t *testing.T) {
	m := NewViewerModel()
	m.tab = &model.Tab{Title: "T", Artist: "A", Metadata: map[string]string{}}
	m.tabID = 7
	m.audioCatalog = mixedCatalog("")
	m.selectedSourceIdx = 4 // yt:lesson
	updated, _ := m.Update(msgs.IntroDetectedMsg{
		SourceID: "yt:official", Offset: 3 * time.Second, TabID: 7, Artist: "A", Title: "T",
	})
	m = updated
	if m.audioOffset != 0 {
		t.Fatalf("stale source probe must not apply, got %v", m.audioOffset)
	}
}

// TestRenderAudioPickerShowsBadges guards US-11: the picker renders category
// badges and marks strict-rejected candidates.
func TestRenderAudioPickerShowsBadges(t *testing.T) {
	cat := player.AudioCatalog{Sources: []player.AudioSource{
		{ID: "midi", Kind: player.SourceMIDI, Label: "MIDI"},
		{ID: "yt:official", Kind: player.SourceOnline, Label: "Sultans of Swing", Category: player.CatOfficial, StrictOK: true},
		{ID: "yt:live", Kind: player.SourceOnline, Label: "Sultans of Swing Live", Category: player.CatLive, StrictOK: false},
	}}
	rendered := renderAudioPickerBody(cat, 0, false, true, 1, nil)
	if !strings.Contains(rendered, "[official]") {
		t.Fatalf("picker should badge official sources, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "[live]") {
		t.Fatalf("picker should badge live sources, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "not studio") {
		t.Fatalf("strict mode should mark rejected sources, got:\n%s", rendered)
	}
}

// tempoMapSchedule has 4 uniform quarter-note steps per bar (480 ticks).
func tempoMapSchedule() []player.PlaybackStep {
	var steps []player.PlaybackStep
	for bar := 0; bar < 4; bar++ {
		for i := 0; i < 4; i++ {
			steps = append(steps, player.PlaybackStep{Bar: bar, Col: i, Ticks: 480})
		}
	}
	return steps
}

// TestTempoMapAndQuality guard the panel indicators for US-13: with three
// anchors, tempoMap reports the spanned BPM range and syncQuality the drift.
func TestTempoMapAndQuality(t *testing.T) {
	m := NewViewerModel()
	m.schedule = tempoMapSchedule()
	// Bars 1-3 at 120 BPM (4 quarters = 2s per bar): 0s->4s. Bars 3-4 at
	// 60 BPM (4 quarters = 4s): 4s->8s.
	m.syncPoints = []player.SyncPoint{{Bar: 1, Seconds: 0}, {Bar: 3, Seconds: 4}, {Bar: 4, Seconds: 8}}
	rng, ok := m.tempoMap()
	if !ok {
		t.Fatal("tempoMap should derive a range from 3 anchors")
	}
	if rng[0] != 60 || rng[1] != 120 {
		t.Fatalf("tempoMap range = [%d, %d], want [60, 120]", rng[0], rng[1])
	}
	q, ok := m.syncQuality()
	if !ok {
		t.Fatal("syncQuality should be derivable from 3 anchors")
	}
	// The base segment implies 120 BPM; bar 4's anchor is 4s late vs the
	// 2s the 120 BPM schedule predicts -> drift of 2s.
	if q < 1.9 || q > 2.1 {
		t.Fatalf("syncQuality = %.2f, want ~2.0", q)
	}
}

// TestTempoMapNeedsTwoAnchors guards the indicator thresholds.
func TestTempoMapNeedsTwoAnchors(t *testing.T) {
	m := NewViewerModel()
	m.schedule = tempoMapSchedule()
	m.syncPoints = []player.SyncPoint{{Bar: 1, Seconds: 0}}
	if _, ok := m.tempoMap(); ok {
		t.Fatal("one anchor cannot build a tempo map")
	}
	if _, ok := m.syncQuality(); ok {
		t.Fatal("one anchor cannot estimate drift")
	}
}
