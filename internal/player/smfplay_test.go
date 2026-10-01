package player

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fretboard/internal/model"
)

// writeFakeSynth is a hermetic fluidsynth that echoes stdin lines to a log.
func writeFakeSynth(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fluidsynth")
	log := filepath.Join(dir, "synth.log")
	script := "#!/bin/sh\nwhile read line; do echo \"$line\" >> \"" + log + "\"; done\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

func smfTestTab() *model.Tab {
	return &model.Tab{
		Title: "T", Artist: "A", Tuning: model.Standard,
		Bars: []model.Bar{
			{Strings: []model.StringLine{{Segments: []model.Segment{
				{Char: '0', Value: 0, Position: 0, Width: 1},
				{Char: '-', Position: 1}, {Char: '-', Position: 2}, {Char: '-', Position: 3},
				{Char: '3', Value: 3, Position: 4, Width: 1},
			}}}},
			{Strings: []model.StringLine{{Segments: []model.Segment{
				{Char: '5', Value: 5, Position: 0, Width: 1},
			}}}},
		},
	}
}

func logLines(t *testing.T, log string) []string {
	t.Helper()
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatalf("read synth log: %v", err)
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

func waitForLog(t *testing.T, log string) []string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(log); err == nil && len(data) > 0 {
			return strings.Split(strings.TrimSpace(string(data)), "\n")
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("synth log stayed empty")
	return nil
}

// waitForLogContaining polls until the fake synth has echoed substr.
func waitForLogContaining(t *testing.T, log, substr string) string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		data, _ := os.ReadFile(log)
		if strings.Contains(string(data), substr) {
			return string(data)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("synth log never contained %q", substr)
	return ""
}

func TestBuildSMFEventsCountInAndClicks(t *testing.T) {
	tab := smfTestTab()
	evts, countInTicks, countInDur, err := buildSMFEvents(tab, 120, MIDIFileOpts{Metronome: true, CountInBars: 1})
	if err != nil {
		t.Fatal(err)
	}
	if countInTicks != 1920 {
		t.Fatalf("countInTicks = %d, want 1920 (1 bar)", countInTicks)
	}
	if countInDur != 2*time.Second {
		t.Fatalf("countInDur = %v, want 2s", countInDur)
	}
	// Music notes must be shifted past the count-in.
	var noteOns []Event
	for _, e := range evts {
		if e.Type == NoteOn && e.Ch == 0 {
			noteOns = append(noteOns, e)
		}
	}
	if len(noteOns) == 0 {
		t.Fatal("no music note events")
	}
	if noteOns[0].Tick != 1920 {
		t.Fatalf("first music note tick = %d, want 1920 (after count-in)", noteOns[0].Tick)
	}
	// Program change for the music channel precedes everything at tick 0.
	if evts[0].Type != ProgramChange || evts[0].Ch != 0 || evts[0].Tick != 0 {
		t.Fatalf("first event = %+v, want ch0 program change at tick 0", evts[0])
	}
	// Count-in clicks land on the count-in channel, one per beat, accented
	// on the bar's first beat.
	clicks := 0
	for _, e := range evts {
		if e.Type == NoteOn && e.Ch == countInChanel {
			clicks++
			if e.Note != clickNoteBar && e.Note != clickNoteBeat {
				t.Fatalf("count-in click note = %d", e.Note)
			}
		}
	}
	if clicks != 4 {
		t.Fatalf("count-in clicks = %d, want 4 (one bar)", clicks)
	}
	// Metronome clicks ride their own channel and align to quarter beats.
	beats := 0
	for _, e := range evts {
		if e.Type == NoteOn && e.Ch == clickChannel {
			beats++
			tick := e.Tick - countInTicks
			if tick%ticksPerQuarter != 0 {
				t.Fatalf("metronome click at tick %d, not on a quarter", tick)
			}
		}
	}
	if beats == 0 {
		t.Fatal("no metronome clicks baked")
	}
}

func TestBuildSMFEventsNoMetronomeNoClicks(t *testing.T) {
	evts, ticks, dur, err := buildSMFEvents(smfTestTab(), 120, MIDIFileOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if ticks != 0 || dur != 0 {
		t.Fatalf("count-in = %d/%v, want 0", ticks, dur)
	}
	for _, e := range evts {
		if (e.Type == NoteOn || e.Type == NoteOff) && (e.Ch == clickChannel || e.Ch == countInChanel) {
			t.Fatalf("click event without metronome/count-in: %+v", e)
		}
	}
}

func TestMidiTickTimeRoundTrip(t *testing.T) {
	for _, bpm := range []int{60, 90, 120, 137, 200} {
		d := 3*time.Second + 500*time.Millisecond
		ticks := midiTicksAt(d, bpm)
		back := midiTicksDuration(ticks, bpm)
		if diff := back - d; diff < -time.Millisecond || diff > time.Millisecond {
			t.Fatalf("bpm %d: %v -> %d ticks -> %v", bpm, d, ticks, back)
		}
	}
}

func TestScheduleTimeAtStepNoFloorDrift(t *testing.T) {
	schedule := []PlaybackStep{
		{Bar: 0, Ticks: 1440, Onset: 0},
		{Bar: 0, Ticks: 480, Onset: 1440},
		{Bar: 1, Ticks: 1920, Onset: 0},
	}
	// Step 0 is exactly 0 — StepDuration's 16th-note floor must not apply.
	if got := ScheduleTimeAtStep(schedule, 0, 120); got != 0 {
		t.Fatalf("step 0 time = %v, want 0", got)
	}
	if got := ScheduleTimeAtStep(schedule, 2, 120); got != 2*time.Second {
		t.Fatalf("step 2 time = %v, want 2s (1440+480 ticks at 120)", got)
	}
	if got := ScheduleSpan(schedule, 120); got != 4*time.Second {
		t.Fatalf("span = %v, want 4s", got)
	}
}

func TestPlayMIDIFileLifecycle(t *testing.T) {
	log := writeFakeSynth(t)
	e := NewEngine()
	e.Synth.Soundfont = "fake.sf2"
	tab := smfTestTab()

	// Metronome off → the click channel is muted via CC at start.
	if err := e.PlayMIDIFile(tab, 120, MIDIFileOpts{CountInBars: 1}); err != nil {
		t.Fatalf("PlayMIDIFile: %v", err)
	}
	if e.Mode() != "midi" || !e.MIDILive() {
		t.Fatalf("mode=%q live=%v", e.Mode(), e.MIDILive())
	}
	cmds := waitForLog(t, log)
	joined := strings.Join(cmds, "\n")
	if !strings.Contains(joined, "player_stop") || !strings.Contains(joined, "player_cont") {
		t.Fatalf("missing player start commands: %q", joined)
	}
	if !strings.Contains(joined, "player_seek 1920") {
		t.Fatalf("expected seek past 1-bar count-in, got %q", joined)
	}
	if !strings.Contains(joined, "cc 1 7 0") {
		t.Fatalf("expected click channel muted, got %q", joined)
	}

	// Position is clamped to the start position while the count-in runs.
	if got := e.MIDIPosition(); got != 0 {
		t.Fatalf("position during count-in = %v, want 0", got)
	}

	// Live tempo change reaches the shell and recalibrates.
	if err := e.MIDISetTempo(90); err != nil {
		t.Fatalf("MIDISetTempo: %v", err)
	}
	waitForLogContaining(t, log, "player_tempo_bpm 90")

	// RestartAt in midi mode is a live seek (A-B loop path).
	if err := e.RestartAt(5 * time.Second); err != nil {
		t.Fatalf("RestartAt: %v", err)
	}
	waitForLogContaining(t, log, "player_seek")
	// Seek recalibrates: position reports the seek target immediately.
	if got := e.MIDIPosition(); got < 5*time.Second || got > 5*time.Second+100*time.Millisecond {
		t.Fatalf("position after seek = %v, want ~5s", got)
	}

	// Metronome toggle unmutes the click channel.
	if err := e.MIDISetClicks(true); err != nil {
		t.Fatalf("MIDISetClicks: %v", err)
	}
	waitForLogContaining(t, log, "cc 1 7 127")

	// Position advances with the wall clock after the count-in.
	e.midiWall = time.Now().Add(-3 * time.Second) // simulate elapsed time
	if got := e.MIDIPosition(); got < 8*time.Second || got > 8*time.Second+100*time.Millisecond {
		t.Fatalf("position = %v, want ~8s (5s seek + 3s elapsed)", got)
	}

	if err := e.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if e.MIDILive() {
		t.Fatal("MIDILive after Stop")
	}
}

func TestPlayMIDIFileStartAtZeroSeek(t *testing.T) {
	log := writeFakeSynth(t)
	e := NewEngine()
	e.Synth.Soundfont = "fake.sf2"
	if err := e.PlayMIDIFile(smfTestTab(), 120, MIDIFileOpts{StartAt: 2 * time.Second}); err != nil {
		t.Fatalf("PlayMIDIFile: %v", err)
	}
	// No count-in: seek straight to the start position (2s = 4 quarters =
	// 1920 ticks at 480/quarter). Metronome off → click channel muted.
	joined := strings.Join(waitForLog(t, log), "\n")
	if !strings.Contains(joined, "player_seek 1920") {
		t.Fatalf("expected seek to tick 1920, got %q", joined)
	}
	if !strings.Contains(joined, "cc 1 7 0") {
		t.Fatalf("metronome is off; the click channel must be muted: %q", joined)
	}
	_ = e.Stop()
}
