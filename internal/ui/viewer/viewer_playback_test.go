package viewer

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"fretboard/internal/model"
	"fretboard/internal/player"
	"fretboard/internal/ui/msgs"
	tea "github.com/charmbracelet/bubbletea"
)

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

func TestTogglePlaybackIgnoresWhileFetching(t *testing.T) {
	m := NewViewerModel()
	m.tab = &model.Tab{Title: "T", Bars: []model.Bar{{Strings: []model.StringLine{{}}}}}
	m.fetchingAudio = true
	if cmd := m.togglePlayback(); cmd != nil {
		t.Fatal("togglePlayback should ignore while audio is downloading")
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

// TestSyncedFor guards the flagship sync predicate: audio mode alone must
// arm audio sync. The old predicate (`Mode()=="audio" && AudioDuration()>0`)
// fell back to the tab deadline clock when the duration was unknown, silently
// desyncing the cursor from the recording.
func TestSyncedFor(t *testing.T) {
	cases := []struct {
		mode string
		want bool
	}{
		{"audio", true},
		{"midi", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := syncedFor(tc.mode); got != tc.want {
			t.Fatalf("syncedFor(%q) = %v, want %v", tc.mode, got, tc.want)
		}
	}
}

// TestStartPlaybackSyncIgnoresUnknownDuration guards the flagship fix through
// the real playback-start handler: an audio-mode start arms audio sync even
// when the engine reports no duration yet (AudioDuration()==0, e.g. ffprobe
// absent). The handler must keep the cursor on the Elapsed() monitor path and
// never arm the tab deadline clock.
func TestStartPlaybackSyncIgnoresUnknownDuration(t *testing.T) {
	m := NewViewerModel()
	m.LoadTab(sampleTab(), "", 0)
	if dur := m.engine.AudioDuration(); dur != 0 {
		t.Fatalf("precondition: duration must be unknown (0), got %v", dur)
	}
	schedule := player.BuildSchedule(m.tab)
	if len(schedule) == 0 {
		t.Fatal("precondition: sample tab must produce a schedule")
	}
	// This is the exact PlaybackStartedMsg startPlaybackCmd now emits for
	// audio mode: AudioSync=true regardless of the reported duration.
	updated, _ := m.Update(msgs.PlaybackStartedMsg{
		Schedule: schedule, StepIdx: 0, Duration: time.Millisecond, AudioSync: true,
	})
	m = updated
	if !m.audioSync {
		t.Fatal("audio mode with unknown duration must arm audio sync")
	}
	if !m.playing {
		t.Fatal("playback must be running after start")
	}
	// The cursor is driven by handlePlaybackMonitor/Elapsed: the step
	// deadline clock is only armed for the non-synced MIDI path.
	if !m.stepClock.Deadline().IsZero() {
		t.Fatalf("audio-synced playback must not arm the step deadline clock, got %v", m.stepClock.Deadline())
	}
}

// TestLoopRestartUsesWarpedPosition guards F9: the A-B loop restarts at the
// anchor-warped audio position of the loop start bar, not the naive
// schedule@BPM position. Anchors that stretch the map (bar 2 of the tab at
// 10s instead of 1s) must move the restart point; with no anchors the warped
// position equals the naive loopStartTime formula.
func TestLoopRestartUsesWarpedPosition(t *testing.T) {
	m := NewViewerModel()
	m.schedule = []player.PlaybackStep{
		{Bar: 0, Ticks: 480}, {Bar: 0, Ticks: 480},
		{Bar: 1, Ticks: 480}, {Bar: 1, Ticks: 480},
		{Bar: 2, Ticks: 480}, {Bar: 2, Ticks: 480},
		{Bar: 3, Ticks: 480}, {Bar: 3, Ticks: 480},
	}
	m.bpm = 120 // two 480-tick steps per bar: 1s per bar
	m.loopStartBar = 2
	m.loopEndBar = 3
	m.audioOffset = 0
	naive := m.loopStartTime() // ScheduleTimeAtBar(schedule, 1) = 1s

	// Bar 2 (0-based 1) anchored at 10s and bar 4 (0-based 3) at 30s: the
	// segment stretches 2s of score over 20s of audio.
	m.syncPoints = []player.SyncPoint{{Bar: 2, Seconds: 10}, {Bar: 4, Seconds: 30}}

	got := m.loopRestartPos()
	tm := player.NewTimeMapper(m.schedule, syncPointsZeroBased(m.syncPoints), m.bpm)
	wantStart, _ := tm.WarpedLoopTimes(1, 3)
	if got != wantStart {
		t.Fatalf("loopRestartPos = %v, want TimeMapper warped start %v", got, wantStart)
	}
	if got == naive {
		t.Fatalf("warped restart %v must differ from naive %v when anchors stretch the map", got, naive)
	}
	if got != 10*time.Second {
		t.Fatalf("loopRestartPos = %v, want 10s", got)
	}

	// No anchors: the warped path degenerates to the naive formula.
	m2 := NewViewerModel()
	m2.schedule = m.schedule
	m2.bpm = 120
	m2.loopStartBar = 2
	m2.loopEndBar = 3
	m2.audioOffset = 20
	if got, want := m2.loopRestartPos(), m2.loopStartTime(); got != want {
		t.Fatalf("no-anchor loopRestartPos = %v, want naive loopStartTime %v", got, want)
	}
}

// TestResumeMapsViaTimeMapper guards F9 resume: the audio position to resume
// from a mid-tab cursor maps through the TimeMapper's anchors, and the naive
// (unanchored) position differs when the anchors warp the map.
func TestResumeMapsViaTimeMapper(t *testing.T) {
	m := NewViewerModel()
	m.schedule = []player.PlaybackStep{
		{Bar: 0, Col: 0, Ticks: 480}, {Bar: 0, Col: 4, Ticks: 480},
		{Bar: 1, Col: 0, Ticks: 480}, {Bar: 1, Col: 4, Ticks: 480},
		{Bar: 2, Col: 0, Ticks: 480}, {Bar: 2, Col: 4, Ticks: 480},
	}
	m.bpm = 120
	m.cursorBar = 1 // 0-based bar 2, mid-segment col 4
	m.cursorCol = 4
	m.syncPoints = []player.SyncPoint{{Bar: 1, Seconds: 10}, {Bar: 3, Seconds: 22}}
	m.autoAnchors = []player.SyncPoint{{Bar: 2, Seconds: 16}}

	got := m.resumeAudioPos()
	points := syncPointsZeroBased(player.MergeAnchors(m.syncPoints, m.autoAnchors))
	want := player.NewTimeMapper(m.schedule, points, m.bpm).ResumePos(m.cursorBar, m.cursorCol)
	if got != want {
		t.Fatalf("resumeAudioPos = %v, want TimeMapper.ResumePos %v", got, want)
	}
	naive := player.NewTimeMapper(m.schedule, nil, m.bpm).ResumePos(m.cursorBar, m.cursorCol)
	if got == naive {
		t.Fatalf("resume %v must differ from the naive position %v when anchors warp the map", got, naive)
	}
	if got != 19*time.Second {
		t.Fatalf("resumeAudioPos = %v, want 19s (mid-segment interpolation)", got)
	}

	// No anchors: resume falls back to the naive schedule position.
	m2 := NewViewerModel()
	m2.schedule = m.schedule
	m2.bpm = 120
	m2.cursorBar = 2
	m2.cursorCol = 0
	m2.audioOffset = 20
	if got := m2.resumeAudioPos(); got != 22*time.Second {
		t.Fatalf("no-anchor resumeAudioPos = %v, want 22s (2s of score + 20s offset)", got)
	}
}

// TestApplyLoopRegionWarped guards F9: the engine A-B loop region is armed
// with the anchor-warped audio times, so the loop wraps on the recording's
// timeline rather than the tab's schedule@BPM line.
func TestApplyLoopRegionWarped(t *testing.T) {
	m := NewViewerModel()
	m.schedule = []player.PlaybackStep{
		{Bar: 0, Ticks: 480}, {Bar: 0, Ticks: 480},
		{Bar: 1, Ticks: 480}, {Bar: 1, Ticks: 480},
		{Bar: 2, Ticks: 480}, {Bar: 2, Ticks: 480},
		{Bar: 3, Ticks: 480}, {Bar: 3, Ticks: 480},
	}
	m.bpm = 120
	m.loopStartBar = 2
	m.loopEndBar = 3
	m.syncPoints = []player.SyncPoint{{Bar: 2, Seconds: 10}, {Bar: 4, Seconds: 30}}

	m.applyLoopRegion()
	start, end, ok := m.engine.LoopRegion()
	if !ok {
		t.Fatal("loop region not armed")
	}
	if start != 10*time.Second || end != 30*time.Second {
		t.Fatalf("warped loop region = [%v, %v], want [10s, 30s]", start, end)
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
