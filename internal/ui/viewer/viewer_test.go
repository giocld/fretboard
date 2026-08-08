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
	// The deadline clock was started when playback began; the tick that
	// fires now belongs to the step after the last one — which the loop
	// wrap sends back to bar 1.
	m.stepClock.Start(stepDur(m.schedule[3].Ticks, m.bpm))

	updated, _ := m.Update(msgs.PlaybackTickMsg{})
	m = updated
	if !m.playing {
		t.Fatal("playback must keep looping instead of stopping at the end")
	}
	if m.stepIdx != 0 {
		t.Fatalf("stepIdx = %d, want 0 (wrapped to loop start)", m.stepIdx)
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

// writeFakeFluidsynthTest writes a fake fluidsynth.cmd that logs stdin
// commands to synth.log (hermetic copy of the player-package fake so the
// viewer can drive the real engine end to end).
func writeFakeFluidsynthTest(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fluidsynth.cmd")
	log := filepath.Join(dir, "synth.log")
	script := "@echo off\r\nsetlocal enabledelayedexpansion\r\n:loop\r\nset \"line=\"\r\nset /p line=\r\nif not defined line goto loop\r\necho !line!>> \"" + log + "\"\r\ngoto loop\r\n"
	if err := os.WriteFile(path, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

func key(k string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}

// TestPracticeKeysDriveEndToEndPlayback guards S3: pressing m (metronome),
// C (count-in) and y (instrument) through the real key handlers produces a
// MIDI playback session in which the fake fluidsynth receives the selected
// program and a metronome click on the first beat.
func TestPracticeKeysDriveEndToEndPlayback(t *testing.T) {
	log := writeFakeFluidsynthTest(t)
	m := NewViewerModel()
	tab := &model.Tab{
		Title:  "Practice",
		Artist: "Test",
		Tuning: model.Standard,
		Bars: []model.Bar{{
			Strings: []model.StringLine{
				{Segments: []model.Segment{
					{Char: '0', Value: 0, Position: 0, Width: 1},
					{Char: '-', Position: 1}, {Char: '-', Position: 2}, {Char: '-', Position: 3},
					{Char: '3', Value: 3, Position: 4, Width: 1},
				}},
			},
		}},
	}
	m.LoadTab(tab, "practice.txt", 0)
	m.engine.Soundfont = "fake.sf2"
	m.engine.Volume = 80

	// m: metronome on. C: 1-bar count-in. y twice: steel -> nylon.
	var cmd tea.Cmd
	m, cmd = m.Update(key("m"))
	if cmd != nil {
		t.Fatal("m should not return a cmd")
	}
	m, cmd = m.Update(key("C"))
	if !m.metronome || m.countIn != 1 {
		t.Fatalf("m=%v countIn=%d after keys", m.metronome, m.countIn)
	}
	m, _ = m.Update(key("y"))
	m, _ = m.Update(key("y"))
	if m.program != 24 {
		t.Fatalf("two y presses should reach nylon (24), got %d", m.program)
	}
	if got := programLabel(m.program); got != "nylon" {
		t.Fatalf("programLabel(24) = %q", got)
	}

	// Space: start playback (MIDI source is the default). The count-in
	// blocks the command for ~2 s, then PlaybackStartedMsg arrives.
	m, cmd = m.Update(key(" "))
	if cmd == nil {
		t.Fatal("Space should return a playback cmd")
	}
	msg := cmd()
	started, ok := msg.(msgs.PlaybackStartedMsg)
	if !ok {
		t.Fatalf("expected PlaybackStartedMsg, got %T (%v)", msg, msg)
	}
	_ = started
	// Feed the message back through Update like the tea loop does.
	m, _ = m.Update(msg)
	if !m.playing || m.engine.Mode() != "midi" {
		t.Fatalf("should be playing midi: playing=%v mode=%q", m.playing, m.engine.Mode())
	}
	m.StopPlayback()

	// The fake synth saw the program and a click on the first beat.
	deadline := time.Now().Add(3 * time.Second)
	var cmds []string
	for time.Now().Before(deadline) {
		data, _ := os.ReadFile(log)
		cmds = strings.Split(strings.TrimSpace(string(data)), "\n")
		if len(cmds) > 0 && cmds[0] != "" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	joined := strings.Join(cmds, "\n")
	if !strings.Contains(joined, "prog 0 24") {
		t.Fatalf("expected 'prog 0 24' in synth log, got %q", joined)
	}
	if !strings.Contains(joined, "noteon 0 37 120") {
		t.Fatalf("expected an accented first-beat click, got %q", joined)
	}
}

// TestPracticeKeyStateAndStatus guards the status row reflects the practice
// tool state.
func TestPracticeKeyStateAndStatus(t *testing.T) {
	m := NewViewerModel()
	tab := &model.Tab{Title: "X", Artist: "Y", Tuning: model.Standard,
		Bars: []model.Bar{{Strings: []model.StringLine{{Segments: []model.Segment{{Char: '0', Value: 0, Position: 0, Width: 1}}}}}}}
	m.LoadTab(tab, "x.txt", 0)
	m, _ = m.Update(key("m"))
	m, _ = m.Update(key("C"))
	m, _ = m.Update(key("y"))
	view := m.View()
	for _, want := range []string{"metronome", "count-in", "steel"} {
		if !strings.Contains(view, want) {
			t.Fatalf("status row missing %q:\n%s", want, view)
		}
	}
	// C cycles 1 -> 2 -> 0.
	m, _ = m.Update(key("C"))
	if m.countIn != 2 {
		t.Fatalf("countIn should cycle to 2, got %d", m.countIn)
	}
	m, _ = m.Update(key("C"))
	if m.countIn != 0 {
		t.Fatalf("countIn should cycle back to 0, got %d", m.countIn)
	}
}

// TestExportKeyWritesFile guards S4.3 in the viewer: X exports the loaded
// tab to a plain-ASCII file in the working directory.
func TestExportKeyWritesFile(t *testing.T) {
	m := NewViewerModel()
	tab := &model.Tab{Title: "Viewer Export", Artist: "A", Tuning: model.Standard,
		Bars: []model.Bar{{Strings: []model.StringLine{{Segments: []model.Segment{{Char: '0', Value: 0, Position: 0, Width: 1}}}}}}}
	m.LoadTab(tab, "v.txt", 0)

	oldwd, _ := os.Getwd()
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	m, _ = m.Update(key("X"))
	data, err := os.ReadFile(filepath.Join(dir, "Viewer Export.txt"))
	if err != nil {
		t.Fatalf("export file missing: %v (info=%q)", err, m.infoMsg)
	}
	if !strings.Contains(string(data), "Viewer Export") {
		t.Fatalf("export content wrong:\n%s", data)
	}
}

// TestTransposeKeysShiftDisplayAndPlayback guards S5.2: T/Z adjust the
// session transpose, the display tab shifts frets, playback uses the
// transposed tab, and R resets.
func TestTransposeKeysShiftDisplayAndPlayback(t *testing.T) {
	m := NewViewerModel()
	tab := &model.Tab{Title: "X", Artist: "Y", Tuning: model.Standard,
		Bars: []model.Bar{{Strings: []model.StringLine{
			{Segments: []model.Segment{{Char: '3', Value: 3, Position: 0, Width: 1}}},
		}}}}
	m.LoadTab(tab, "x.txt", 0)

	m, _ = m.Update(key("T"))
	m, _ = m.Update(key("T"))
	if m.transpose != 2 {
		t.Fatalf("transpose = %d, want 2", m.transpose)
	}
	display := m.displayTab()
	if display == m.tab {
		t.Fatal("display tab should be a transposed copy")
	}
	if got := display.Bars[0].Strings[0].Segments[0].Value; got != 5 {
		t.Fatalf("display fret = %d, want 5", got)
	}
	// Playback schedule comes from the transposed tab.
	sched := player.BuildSchedule(m.displayTab())
	if len(sched) == 0 {
		t.Fatal("empty schedule from transposed tab")
	}
	// Status row shows the transpose.
	m, _ = m.Update(key("R"))
	if m.transpose != 0 {
		t.Fatalf("R should reset transpose, got %d", m.transpose)
	}
	if m.displayTab() != m.tab {
		t.Fatal("after reset the original tab renders again")
	}
}

// TestSearchInTab guards S5.1: / opens the search, patterns match fret
// digits, n/N cycle matches, Enter jumps and closes.
func TestSearchInTab(t *testing.T) {
	m := NewViewerModel()
	tab := &model.Tab{Title: "X", Tuning: model.Standard, Bars: []model.Bar{
		{Number: 1, Strings: []model.StringLine{{Segments: []model.Segment{
			{Char: '0', Value: 0, Position: 0, Width: 1},
			{Char: '-', Position: 1},
			{Char: '3', Value: 3, Position: 2, Width: 1},
			{Char: '-', Position: 3},
			{Char: '5', Value: 5, Position: 4, Width: 1},
		}}}},
		{Number: 2, Strings: []model.StringLine{{Segments: []model.Segment{
			{Char: '3', Value: 3, Position: 0, Width: 1},
		}}}},
	}}
	m.LoadTab(tab, "x.txt", 0)

	m, _ = m.Update(key("/"))
	if !m.searchActive {
		t.Fatal("/ should open the search box")
	}
	// Type "35": matches bar 1 (digits 035), not bar 2 (3).
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("35")})
	if len(m.searchMatches) != 1 || m.searchMatches[0].bar != 0 {
		t.Fatalf("matches = %+v, want one match in bar 1", m.searchMatches)
	}
	// Type "3": matches both bars; n/N cycle.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	if len(m.searchMatches) != 2 {
		t.Fatalf("two bars contain a 3, got %+v", m.searchMatches)
	}
	m, _ = m.Update(key("n"))
	if m.searchIdx != 1 || m.cursorBar != m.searchMatches[1].bar {
		t.Fatalf("n should move to match 2: idx=%d bar=%d", m.searchIdx, m.cursorBar)
	}
	m, _ = m.Update(key("N"))
	if m.searchIdx != 0 {
		t.Fatalf("N should wrap back to match 1, got %d", m.searchIdx)
	}
	// Bar-number search: "2" jumps to bar 2.
	m, _ = m.Update(key("esc"))
	m, _ = m.Update(key("/"))
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	if len(m.searchMatches) != 1 || m.searchMatches[0].bar != 1 {
		t.Fatalf("bar-number search should match bar 2, got %+v", m.searchMatches)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.searchActive {
		t.Fatal("Enter should close the search")
	}
	if m.cursorBar != 1 {
		t.Fatalf("Enter should jump to bar 2, cursor at %d", m.cursorBar)
	}
}

// TestNoteNamesKey guards S5.3: e toggles the note-name view and the status
// row announces it.
func TestNoteNamesKey(t *testing.T) {
	m := NewViewerModel()
	tab := &model.Tab{Title: "X", Tuning: model.Standard,
		Bars: []model.Bar{{Strings: []model.StringLine{{Segments: []model.Segment{{Char: '3', Value: 3, Position: 0, Width: 1}}}}}}}
	m.LoadTab(tab, "x.txt", 0)
	m, _ = m.Update(key("e"))
	if !m.showNotes {
		t.Fatal("e should enable the note-name view")
	}
	if !strings.Contains(m.View(), "notes") {
		t.Fatalf("status should mention notes:\n%s", m.View())
	}
	m, _ = m.Update(key("e"))
	if m.showNotes {
		t.Fatal("e should toggle notes back off")
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

// TestSearchMatchesSectionNames guards G2.3: typing a section name in the
// in-tab search jumps to that section's first bar.
func TestSearchMatchesSectionNames(t *testing.T) {
	m := NewViewerModel()
	tab := &model.Tab{Title: "X", Tuning: model.Standard, Bars: []model.Bar{
		{Number: 1, Section: "Intro", Strings: []model.StringLine{{Segments: []model.Segment{{Char: '0', Value: 0, Position: 0, Width: 1}}}}},
		{Number: 2, Section: "Intro", Strings: []model.StringLine{{Segments: []model.Segment{{Char: '0', Value: 0, Position: 0, Width: 1}}}}},
		{Number: 3, Section: "Chorus", Strings: []model.StringLine{{Segments: []model.Segment{{Char: '5', Value: 5, Position: 0, Width: 1}}}}},
	}}
	m.LoadTab(tab, "x.txt", 0)

	m, _ = m.Update(key("/"))
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("chorus")})
	if len(m.searchMatches) != 1 || m.searchMatches[0].bar != 2 {
		t.Fatalf("section search should match the chorus first bar, got %+v", m.searchMatches)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.cursorBar != 2 {
		t.Fatalf("Enter should jump to the chorus, cursor at %d", m.cursorBar)
	}
	// Status row shows the current section.
	if !strings.Contains(m.View(), "Chorus") {
		t.Fatalf("status should name the current section:\n%s", m.View())
	}
}

// TestPerformanceModeToggles guards G3.1: P swaps the tab body for the
// performance view (section + progress) and toggles back.
func TestPerformanceModeToggles(t *testing.T) {
	m := NewViewerModel()
	tab := &model.Tab{Title: "X", Tuning: model.Standard, Bars: []model.Bar{
		{Number: 1, Section: "Intro", Strings: []model.StringLine{{Segments: []model.Segment{{Char: '0', Value: 0, Position: 0, Width: 1}}}}},
		{Number: 2, Section: "Chorus", Strings: []model.StringLine{{Segments: []model.Segment{{Char: '5', Value: 5, Position: 0, Width: 1}}}}},
	}}
	m.LoadTab(tab, "x.txt", 0)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})

	m, _ = m.Update(key("P"))
	if !m.perfMode {
		t.Fatal("P should enable performance mode")
	}
	view := m.View()
	for _, want := range []string{"Intro", "bar 1 / 2", "50%", "perf"} {
		if !strings.Contains(view, want) {
			t.Fatalf("performance view missing %q:\n%s", want, view)
		}
	}
	m, _ = m.Update(key("P"))
	if m.perfMode {
		t.Fatal("P should toggle performance mode off")
	}
}

// TestPracticeTimerAccumulatesAndPersists guards G3.2: playback time banks
// into practice_seconds metadata when playback stops, and survives a tab
// reload (it comes back from the metadata).
func TestPracticeTimerAccumulatesAndPersists(t *testing.T) {
	m := NewViewerModel()
	tab := &model.Tab{Title: "X", Tuning: model.Standard,
		Bars: []model.Bar{{Strings: []model.StringLine{{Segments: []model.Segment{{Char: '0', Value: 0, Position: 0, Width: 1}}}}}}}
	m.LoadTab(tab, "x.txt", 0)

	// Simulate a 3-second playback session.
	m.playing = true
	m.practiceStart = time.Now().Add(-3 * time.Second)
	m.StopPlayback()

	raw := strings.TrimSpace(m.tab.Metadata["practice_seconds"])
	if raw == "" {
		t.Fatal("practice_seconds should be persisted after playback stops")
	}
	total := m.practiceTotal()
	if total < 3 {
		t.Fatalf("practice total should include the session, got %d", total)
	}

	// A second session banks on top of the first.
	m.LoadTab(m.tab, "x.txt", 0)
	if m.practiceTotal() < 3 {
		t.Fatalf("practice total should survive reload, got %d", m.practiceTotal())
	}
	m.playing = true
	m.practiceStart = time.Now().Add(-2 * time.Second)
	m.StopPlayback()
	if m.practiceTotal() < 5 {
		t.Fatalf("second session should accumulate, got %d", m.practiceTotal())
	}
}

// TestMouseWheelMovesCursor guards G5.1: wheel messages scroll the viewer
// like j/k.
func TestMouseWheelMovesCursor(t *testing.T) {
	m := NewViewerModel()
	tab := &model.Tab{Title: "X", Tuning: model.Standard, Bars: []model.Bar{
		{Number: 1, Strings: []model.StringLine{{Segments: []model.Segment{{Char: '0', Value: 0, Position: 0, Width: 1}}}}},
		{Number: 2, Strings: []model.StringLine{{Segments: []model.Segment{{Char: '0', Value: 0, Position: 0, Width: 1}}}}},
		{Number: 3, Strings: []model.StringLine{{Segments: []model.Segment{{Char: '0', Value: 0, Position: 0, Width: 1}}}}},
	}}
	m.LoadTab(tab, "x.txt", 0)

	m, _ = m.Update(tea.MouseMsg{Type: tea.MouseWheelDown})
	if m.cursorBar != 1 {
		t.Fatalf("wheel down should move to bar 2, got %d", m.cursorBar)
	}
	m, _ = m.Update(tea.MouseMsg{Type: tea.MouseWheelUp})
	if m.cursorBar != 0 {
		t.Fatalf("wheel up should move back to bar 1, got %d", m.cursorBar)
	}
}

// TestMidiTickLoopDeadlines guards the deadline-clock rework: repeated ticks
// advance the schedule and the clock deadline rolls forward by each step's
// duration — never re-based from "now", so processing time cannot drift the
// beat.
func TestMidiTickLoopDeadlines(t *testing.T) {
	m := NewViewerModel()
	sched := []player.PlaybackStep{
		{Bar: 0, Col: 0, Ticks: 480},
		{Bar: 0, Col: 4, Ticks: 480},
		{Bar: 1, Col: 0, Ticks: 240},
		{Bar: 1, Col: 2, Ticks: 240},
		{Bar: 1, Col: 4, Ticks: 480},
	}
	m.schedule = sched
	m.stepIdx = 0
	m.playing = true
	m.audioSync = false
	m.tab = &model.Tab{Tuning: model.Standard, Bars: make([]model.Bar, 2)}
	m.stepClock.Start(stepDur(sched[0].Ticks, m.bpm))
	base := m.stepClock.Deadline()

	want := []int{1, 2, 3, 4}
	for i, wantIdx := range want {
		m, _ = m.Update(msgs.PlaybackTickMsg{})
		if m.stepIdx != wantIdx {
			t.Fatalf("tick %d: stepIdx = %d, want %d", i, m.stepIdx, wantIdx)
		}
	}
	// The deadline is exactly the cumulative duration from the base, not
	// re-anchored to now.
	// The deadline is exactly the cumulative duration of the steps the four
	// ticks played (1..4) — the base already includes step 0 — and it is
	// never re-anchored to now.
	wantDur := stepDur(sched[1].Ticks, m.bpm) + stepDur(sched[2].Ticks, m.bpm) +
		stepDur(sched[3].Ticks, m.bpm) + stepDur(sched[4].Ticks, m.bpm)
	if m.stepClock.Deadline().Sub(base) != wantDur {
		t.Fatalf("deadline advanced by %v, want %v", m.stepClock.Deadline().Sub(base), wantDur)
	}

	// Natural end: the next tick stops playback.
	m, _ = m.Update(msgs.PlaybackTickMsg{})
	if m.playing {
		t.Fatal("playback should stop after the last step")
	}
}

// TestBpmChangeRebasesClockWithoutRestart guards the BPM re-base: in MIDI
// mode, + re-bases the clock and keeps the session (no stop/start).
func TestBpmChangeRebasesClockWithoutRestart(t *testing.T) {
	m := NewViewerModel()
	sched := []player.PlaybackStep{{Bar: 0, Col: 0, Ticks: 480}, {Bar: 0, Col: 4, Ticks: 480}}
	m.schedule = sched
	m.stepIdx = 0
	m.playing = true
	m.audioSync = false
	m.tab = &model.Tab{Tuning: model.Standard, Bars: make([]model.Bar, 1)}
	m.stepClock.Start(stepDur(sched[0].Ticks, m.bpm))
	oldBPM := m.bpm

	m, cmd := m.Update(key("+"))
	if !m.playing {
		t.Fatal("BPM change must not stop MIDI playback")
	}
	if cmd != nil {
		t.Fatal("BPM change must not restart playback (no cmd)")
	}
	if m.bpm != oldBPM+5 {
		t.Fatalf("bpm should increase by 5, got %d", m.bpm)
	}
	// The clock re-based to roughly one step at the new tempo from now.
	if d := m.stepClock.Until(); d > stepDur(sched[0].Ticks, m.bpm)+10*time.Millisecond || d < stepDur(sched[0].Ticks, m.bpm)-10*time.Millisecond {
		t.Fatalf("clock should re-base to one step at the new tempo, got %v", d)
	}
	// The schedule and cursor are untouched — the session continues.
	if len(m.schedule) != 2 || m.stepIdx != 0 {
		t.Fatalf("session state must survive a BPM change: %+v", m.schedule)
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
