package viewer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fretboard/internal/model"
	"fretboard/internal/ui/msgs"
	tea "github.com/charmbracelet/bubbletea"
)

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

// realignSetup returns a viewer with a tab loaded, a real local audio file
// on disk, and that source marked already-aligned — the state W/F9 realign
// is expected to reset.
func realignSetup(t *testing.T) ViewerModel {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "backing.mp3")
	if err := os.WriteFile(path, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := NewViewerModel()
	tab := &model.Tab{Title: "X", Artist: "Y", Tuning: model.Standard,
		Bars: []model.Bar{{Strings: []model.StringLine{{Segments: []model.Segment{{Char: '0', Value: 0, Position: 0, Width: 1}}}}}}}
	m.LoadTab(tab, "x.txt", 0)
	m.audioCatalog = mixedCatalog(path)
	m.selectedSourceIdx = 5 // local:path
	m.alignedSources = map[string]bool{"local:path": true}
	return m
}

// TestF9RealignRerunsAlignment guards the F9 realign shortcut: like W, it
// clears the already-aligned marker for the current source and kicks off a
// fresh alignment. An already-aligned source normally makes maybeAlignCmd
// return nil, so a non-nil cmd here proves the marker was cleared and the
// alignment is re-running.
func TestF9RealignRerunsAlignment(t *testing.T) {
	m := realignSetup(t)
	m, cmd := m.Update(key("f9"))
	if cmd == nil {
		t.Fatal("F9 should return an alignment cmd (marker must have been cleared)")
	}
	if m.infoMsg != "Re-running audio alignment..." {
		t.Fatalf("infoMsg = %q, want the realign hint", m.infoMsg)
	}
}

// TestWRealignRerunsAlignment guards W keeps re-running alignment after the
// shared realign helper was extracted.
func TestWRealignRerunsAlignment(t *testing.T) {
	m := realignSetup(t)
	m, cmd := m.Update(key("W"))
	if cmd == nil {
		t.Fatal("W should return an alignment cmd (marker must have been cleared)")
	}
	if m.infoMsg != "Re-running audio alignment..." {
		t.Fatalf("infoMsg = %q, want the realign hint", m.infoMsg)
	}
}

// TestF9RealignWithoutSource guards F9 without a loaded audio catalog shows
// the same no-source error W does.
func TestF9RealignWithoutSource(t *testing.T) {
	m := NewViewerModel()
	tab := &model.Tab{Title: "X", Tuning: model.Standard,
		Bars: []model.Bar{{Strings: []model.StringLine{{Segments: []model.Segment{{Char: '0', Value: 0, Position: 0, Width: 1}}}}}}}
	m.LoadTab(tab, "x.txt", 0)
	m.alignedSources = nil

	m, _ = m.Update(key("f9"))
	if m.errMsg != "No audio source to align" {
		t.Fatalf("errMsg = %q, want %q", m.errMsg, "No audio source to align")
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
