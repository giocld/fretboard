package viewer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fretboard/internal/model"
	"fretboard/internal/player"
	"fretboard/internal/ui/msgs"
	tea "github.com/charmbracelet/bubbletea"
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

func TestTogglePlaybackUsesResolvedOnlineAudio(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "backing.mp3")
	if err := os.WriteFile(path, []byte("fake"), 0644); err != nil {
		t.Fatal(err)
	}

	m := NewViewerModel()
	m.tab = &model.Tab{Title: "Layla", Artist: "Clapton", Bars: []model.Bar{{Strings: []model.StringLine{{}}}}}
	m.selectedSourceIdx = 1
	m.resolvedAudio = path
	m.audioCatalog = player.AudioCatalog{
		Sources: []player.AudioSource{
			{ID: "midi", Kind: player.SourceMIDI, Label: "MIDI"},
			{ID: "yt:abc", Kind: player.SourceOnline, Label: "YouTube", VideoID: "abc"},
		},
	}

	cmd := m.togglePlayback()
	if cmd == nil {
		t.Fatal("expected playback cmd when resolved audio is cached")
	}
}

func TestPlaybackStartIndexFromCursor(t *testing.T) {
	m := NewViewerModel()
	m.tab = &model.Tab{
		Title: "T",
		Bars: []model.Bar{{
			Strings: []model.StringLine{
				{Segments: []model.Segment{{Char: '0', Value: 0, Position: 0, Width: 1}, {Char: '3', Value: 3, Position: 4, Width: 1}}},
				{Segments: []model.Segment{{Char: '-', Value: -1, Position: 0, Width: 1}}},
			},
		}},
	}
	m.cursorBar = 0
	m.cursorCol = 4
	if got := m.playbackStartIndex(); got != 1 {
		t.Fatalf("playbackStartIndex = %d, want 1", got)
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

func TestViewerNavigationStopsPlayback(t *testing.T) {
	m := NewViewerModel()
	m.tab = &model.Tab{Title: "T", Bars: []model.Bar{{}, {}}}
	m.playing = true
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.playing {
		t.Fatal("j should stop playback before scrolling")
	}
	if m.cursorBar != 1 {
		t.Fatalf("cursorBar=%d want 1", m.cursorBar)
	}
}

func TestTogglePlaybackIgnoresWhileFetching(t *testing.T) {
	m := NewViewerModel()
	m.tab = &model.Tab{Title: "T", Bars: []model.Bar{{Strings: []model.StringLine{{}}}}}
	m.fetchingAudio = true
	if cmd := m.togglePlayback(); cmd != nil {
		t.Fatal("togglePlayback should ignore while audio is downloading")
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

// TestLoopSetBeforePlayArmsEngineAtPlaybackStart guards US-6: loop points
// set while paused (no schedule yet) must arm the engine region when playback
// starts — before this, audio-synced playback silently never looped.
func TestLoopSetBeforePlayArmsEngineAtPlaybackStart(t *testing.T) {
	m := NewViewerModel()
	m.LoadTab(sampleTab(), "", 0)
	m.schedule = nil // paused before any playback

	m, _ = m.setLoopPoint(true)  // A at bar 1
	m, _ = m.setLoopPoint(false) // B at bar 2

	if _, _, ok := m.engine.LoopRegion(); ok {
		t.Fatal("engine must not be armed yet: no schedule exists")
	}

	schedule := player.BuildSchedule(m.tab)
	updated, _ := m.Update(msgs.PlaybackStartedMsg{Schedule: schedule, StepIdx: 0, Duration: time.Millisecond, AudioSync: true})
	m = updated
	start, end, ok := m.engine.LoopRegion()
	if !ok {
		t.Fatal("engine loop region must be armed at playback start")
	}
	if end <= start {
		t.Fatalf("loop region [%v, %v] must be non-empty", start, end)
	}
}

// TestLoopClearWithoutScheduleClearsEngine guards the `x` key path: clearing
// the loop while paused must still clear any previously armed engine region.
func TestLoopClearWithoutScheduleClearsEngine(t *testing.T) {
	m := NewViewerModel()
	m.LoadTab(sampleTab(), "", 0)
	m.schedule = player.BuildSchedule(m.tab)
	m, _ = m.setLoopPoint(true)
	m, _ = m.setLoopPoint(false)
	if _, _, ok := m.engine.LoopRegion(); !ok {
		t.Fatal("loop should be armed before clearing")
	}
	m.schedule = nil
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if _, _, ok := m.engine.LoopRegion(); ok {
		t.Fatal("x must clear the engine loop even with no schedule")
	}
}

// TestSyncBarKeyGivesFeedbackWhenUnavailable guards US-7: pressing s outside
// audio-synced playback must explain why it can't anchor, instead of silently
// doing nothing (the footer advertises the key unconditionally).
func TestSyncBarKeyGivesFeedbackWhenUnavailable(t *testing.T) {
	m := NewViewerModel()
	m.LoadTab(sampleTab(), "", 0)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	m = updated
	if m.errMsg == "" {
		t.Fatal("s while paused should show a hint, not a silent no-op")
	}
	if len(m.syncPoints) != 0 {
		t.Fatalf("s must not anchor while paused, got %+v", m.syncPoints)
	}

	// Playing via MIDI synth is still not a real recording: hint again.
	m.errMsg = ""
	m.playing = true
	m.audioSync = false
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	m = updated
	if m.errMsg == "" {
		t.Fatal("s during MIDI playback should show a hint")
	}
	if len(m.syncPoints) != 0 {
		t.Fatalf("s must not anchor during MIDI, got %+v", m.syncPoints)
	}

	// A real audio-synced playback anchors as before.
	m.errMsg = ""
	m.audioSync = true
	m.tabID = 7
	m.cursorBar = 2
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	m = updated
	if len(m.syncPoints) != 1 || m.syncPoints[0].Bar != 3 {
		t.Fatalf("s during audio playback should anchor bar 3, got %+v", m.syncPoints)
	}
	if cmd == nil {
		t.Fatal("anchoring should persist tab prefs")
	}
}

// TestSyncBarKeyFeedbackClearsOnEsc guards the errMsg escape hatch used for
// the sync hint: Esc clears the message without navigating away.
func TestSyncBarKeyFeedbackClearsOnEsc(t *testing.T) {
	m := NewViewerModel()
	m.LoadTab(sampleTab(), "", 0)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if m.errMsg == "" {
		t.Fatal("precondition: hint should be set")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.errMsg != "" {
		t.Fatalf("Esc should clear the hint, got %q", m.errMsg)
	}
}

// TestClearSyncPointsKeyReportsEmpty guards the S key when no anchors exist.
func TestClearSyncPointsKeyReportsEmpty(t *testing.T) {
	m := NewViewerModel()
	m.LoadTab(sampleTab(), "", 0)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("S")})
	if m.errMsg == "" {
		t.Fatal("S with no anchors should say so")
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

// TestLoopStartTimeUsesZeroBasedBarPlusOffset verifies the A-B loop restarts
// at the audio file position of the loop start bar: schedule time of the
// 0-based bar plus the calibrated intro offset. The old code used the 1-based
// bar (one bar late) and dropped the offset (restarting into the intro).
func TestLoopStartTimeUsesZeroBasedBarPlusOffset(t *testing.T) {
	m := NewViewerModel()
	m.schedule = []player.PlaybackStep{
		{Bar: 0, Ticks: 480}, {Bar: 0, Ticks: 480},
		{Bar: 1, Ticks: 480}, {Bar: 1, Ticks: 480},
		{Bar: 2, Ticks: 480}, {Bar: 2, Ticks: 480},
	}
	m.bpm = 120 // 480 ticks = 500ms at 120 BPM
	m.loopStartBar = 2
	m.audioOffset = 20
	// Bar 2 (0-based bar 1) starts at 1s of music; +20s intro = 21s.
	if got := m.loopStartTime(); got != 21*time.Second {
		t.Fatalf("loopStartTime = %v, want 21s", got)
	}
}

// TestSetLoopPointMapsToFileTime ensures the engine loop region is registered
// in audio file time (schedule + offset), matching what the monitor compares
// against Engine.Elapsed().
func TestSetLoopPointMapsToFileTime(t *testing.T) {
	m := NewViewerModel()
	m.schedule = []player.PlaybackStep{
		{Bar: 0, Ticks: 480}, {Bar: 0, Ticks: 480},
		{Bar: 1, Ticks: 480}, {Bar: 1, Ticks: 480},
		{Bar: 2, Ticks: 480}, {Bar: 2, Ticks: 480},
	}
	m.bpm = 120
	m.audioOffset = 20
	m.cursorBar = 1 // user bar 2
	m.loopStartBar = 0
	m.loopEndBar = 0
	updated, _ := m.setLoopPoint(true)
	m = updated
	m.cursorBar = 2 // user bar 3
	updated, _ = m.setLoopPoint(false)
	m = updated
	start, end, ok := m.engine.LoopRegion()
	if !ok {
		t.Fatal("loop region not set")
	}
	if start != 21*time.Second || end != 23*time.Second {
		t.Fatalf("loop region = [%v, %v], want [21s, 23s]", start, end)
	}
}

// TestMidiLoopWrapsAtLastBar guards the loop-end-at-last-bar case: the wrap
// check used to fire only when a step landed beyond the loop end bar, which
// never happens when the loop ends on the tab's final bar — playback stopped
// at the end instead of looping.
func TestMidiLoopWrapsAtLastBar(t *testing.T) {
	m := NewViewerModel()
	m.schedule = []player.PlaybackStep{
		{Bar: 0, Col: 0, Ticks: 480},
		{Bar: 0, Col: 4, Ticks: 480},
		{Bar: 1, Col: 0, Ticks: 480},
		{Bar: 1, Col: 4, Ticks: 480},
	}
	m.playing = true
	m.audioSync = false
	m.loopStartBar = 1
	m.loopEndBar = 2 // loop is the whole 2-bar tab: end bar == last bar
	m.stepIdx = 3    // last step of the schedule
	m.tab = &model.Tab{Tuning: model.Standard, Bars: make([]model.Bar, 2)}

	updated, _ := m.Update(msgs.PlaybackTickMsg{})
	m = updated
	if !m.playing {
		t.Fatal("playback must keep looping instead of stopping at the end")
	}
	if m.stepIdx != 0 {
		t.Fatalf("stepIdx = %d, want 0 (wrapped to loop start)", m.stepIdx)
	}
}

func TestAdjustAudioOffsetPersistsAndRounds(t *testing.T) {	tab := &model.Tab{Title: "T", Artist: "A", Metadata: map[string]string{}}
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

func TestMaxPanOffsetGridAware(t *testing.T) {
	m := NewViewerModel()
	tab := &model.Tab{
		Title:  "Wide",
		Artist: "Test",
		Tuning: model.ParseTuning("EADGBE"),
		Bars: []model.Bar{
			{Number: 1, Strings: []model.StringLine{{Segments: []model.Segment{{Position: 0, Width: 24}}}}},
		},
	}
	m.LoadTab(tab, "", 0)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 86, Height: 30})
	if got := m.maxPanOffset(); got != 0 {
		t.Fatalf("maxPanOffset on wide terminal = %d, want 0 (grid fits)", got)
	}
	tab.Bars[0].Strings[0].Segments[0].Width = 80
	m, _ = m.Update(tea.WindowSizeMsg{Width: 40, Height: 30})
	if got := m.maxPanOffset(); got <= 0 {
		t.Fatalf("maxPanOffset on narrow terminal with wide bar = %d, want > 0", got)
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

func TestSetLoopPointArmsEngine(t *testing.T) {
	m := NewViewerModel()
	m.LoadTab(sampleTab(), "", 0)
	m.schedule = player.BuildSchedule(m.tab)
	m, _ = m.setLoopPoint(true)
	if m.loopStartBar != 1 {
		t.Fatalf("loop start should be bar 1, got %d", m.loopStartBar)
	}
	m, _ = m.setLoopPoint(false)
	if m.loopEndBar != 2 {
		t.Fatalf("loop end should be bar 2, got %d", m.loopEndBar)
	}
	_, _, ok := m.engine.LoopRegion()
	if !ok {
		t.Fatal("engine loop region not armed")
	}
	m, _ = m.setLoopPoint(true) // move start after end
	if m.loopStartBar != 1 || m.loopEndBar != 2 {
		t.Fatalf("loop clamping wrong: start %d end %d", m.loopStartBar, m.loopEndBar)
	}
	m.loopStartBar, m.loopEndBar = 0, 0
	m.engine.SetLoop(0, 0)
	if _, _, ok := m.engine.LoopRegion(); ok {
		t.Fatal("loop region should clear")
	}
}

func TestLayoutToggleSwitchesRenderer(t *testing.T) {
	m := NewViewerModel()
	m.LoadTab(sampleTab(), "", 0)
	if m.linear {
		t.Fatal("default layout should be grid")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	if !m.linear {
		t.Fatal("v should toggle to linear layout")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	if m.linear {
		t.Fatal("v should toggle back to grid")
	}
}

func TestManualNavDisablesFollow(t *testing.T) {
	m := NewViewerModel()
	m.LoadTab(sampleTab(), "", 0)
	m.follow = true
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if m.follow {
		t.Fatal("manual j navigation should disable follow mode")
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

func sampleTab() *model.Tab {
	lines := make([]model.StringLine, 6)
	for i := range lines {
		lines[i] = model.StringLine{Segments: []model.Segment{{Char: '1', Value: 1, Position: 0, Width: 1}}}
	}
	return &model.Tab{
		Title:    "Sample",
		Tuning:   model.Standard,
		Bars:     []model.Bar{{Number: 1, Strings: lines}, {Number: 2, Strings: lines}, {Number: 3, Strings: lines}},
		Metadata: map[string]string{},
	}
}

// TestTrackEndedBanner guards S2.4: an audio file that ends before the tab
// finishes produces an explanatory message with a restart hint.
func TestTrackEndedBanner(t *testing.T) {
	got := trackEndedBanner(4*time.Minute + 12*time.Second)
	for _, want := range []string{"4:12", "Track ended", "Space restarts"} {
		if !strings.Contains(got, want) {
			t.Fatalf("banner %q missing %q", got, want)
		}
	}
	if got := trackEndedBanner(0); !strings.Contains(got, "Track ended") {
		t.Fatalf("unknown-duration banner should still explain, got %q", got)
	}
}
