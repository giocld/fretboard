package player

import (
	"os"
	"strings"
	"testing"
	"time"

	"fretboard/internal/model"
)

// synthLogCommands returns the fluidsynth command lines captured by the
// fake synth (writeFakeFluidsynth).
func synthLogCommands(t *testing.T, log string) []string {
	t.Helper()
	data, err := os.ReadFile(log)
	if err != nil {
		return nil
	}
	var out []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func startFakeRealtime(t *testing.T, s *Synth) {
	t.Helper()
	s.Soundfont = "fake.sf2"
	s.Volume = 80
	if err := s.StartRealtime(); err != nil {
		t.Fatalf("StartRealtime: %v", err)
	}
}

// waitForSynthLog polls the fake synth's log until it contains needle or
// the timeout elapses (the .cmd fake echoes stdin asynchronously).
func waitForSynthLog(t *testing.T, log, needle string, timeout time.Duration) []string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cmds := synthLogCommands(t, log); len(cmds) > 0 {
			for _, c := range cmds {
				if strings.Contains(c, needle) {
					return cmds
				}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return synthLogCommands(t, log)
}

// TestStartRealtimeSendsSelectedProgram guards S3.3: the configured GM
// program reaches fluidsynth.
func TestStartRealtimeSendsSelectedProgram(t *testing.T) {
	log := writeFakeFluidsynth(t)
	s := NewSynth()
	s.Program = 33 // bass
	startFakeRealtime(t, s)
	defer s.Stop()

	cmds := waitForSynthLog(t, log, "prog 0 33", 3*time.Second)
	found := false
	for _, c := range cmds {
		if c == "prog 0 33" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected 'prog 0 33' in synth log, got %v", cmds)
	}
}

// TestPlayStepMetronomeClicksOnBeats guards S3.1: with the metronome on, a
// step on a beat column clicks (accented on the first beat of the bar) and a
// non-beat step does not.
func TestPlayStepMetronomeClicksOnBeats(t *testing.T) {
	log := writeFakeFluidsynth(t)
	s := NewSynth()
	s.Metronome = true
	startFakeRealtime(t, s)
	defer s.Stop()

	// Bar in 4/4 with two quarter notes at cols 0 and 4 (480 ticks each).
	tab := &model.Tab{
		Tuning: model.Standard,
		Bars: []model.Bar{{
			Strings: []model.StringLine{
				{Segments: []model.Segment{
					{Char: '0', Value: 0, Position: 0, Width: 1},
					{Char: '-', Position: 1}, {Char: '-', Position: 2}, {Char: '-', Position: 3},
					{Char: '3', Value: 3, Position: 4, Width: 1},
					{Char: '-', Position: 5}, {Char: '-', Position: 6},
				}},
			},
		}},
	}
	// First step (col 0) = beat, accented.
	if err := s.PlayStep(tab, PlaybackStep{Bar: 0, Col: 0, Ticks: 480}, 120); err != nil {
		t.Fatal(err)
	}
	// Second step (col 4) = beat too.
	if err := s.PlayStep(tab, PlaybackStep{Bar: 0, Col: 4, Ticks: 480}, 120); err != nil {
		t.Fatal(err)
	}

	cmds := waitForSynthLog(t, log, "noteon 0 37", 3*time.Second)
	var clicks []string
	for _, c := range cmds {
		if strings.HasPrefix(c, "noteon 0 37 ") {
			clicks = append(clicks, c)
		}
	}
	if len(clicks) != 2 {
		t.Fatalf("expected 2 metronome clicks, got %v (cmds %v)", clicks, cmds)
	}
	if !strings.HasSuffix(clicks[0], "120") {
		t.Fatalf("first beat should be accented (vel 120), got %q", clicks[0])
	}
}

// TestPlayStepNoClickOffBeat guards S3.1: with the metronome on, a step that
// is not a beat column (eighth-note tail) does not click.
func TestPlayStepNoClickOffBeat(t *testing.T) {
	log := writeFakeFluidsynth(t)
	s := NewSynth()
	s.Metronome = true
	startFakeRealtime(t, s)
	defer s.Stop()

	tab := &model.Tab{
		Tuning: model.Standard,
		Bars: []model.Bar{{
			Strings: []model.StringLine{
				{Segments: []model.Segment{
					{Char: '0', Value: 0, Position: 0, Width: 1},
					{Char: '-', Position: 1},
					{Char: '3', Value: 3, Position: 2, Width: 1}, // eighth-note tail (240 ticks)
					{Char: '-', Position: 3},
				}},
			},
		}},
	}
	// col 2 is a half-beat; accumulated ticks 240 % 480 != 0 → no click.
	if err := s.PlayStep(tab, PlaybackStep{Bar: 0, Col: 2, Ticks: 240}, 120); err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond) // let any (wrong) click land in the log

	for _, c := range synthLogCommands(t, log) {
		if strings.HasPrefix(c, "noteon 0 37 ") {
			t.Fatalf("off-beat step should not click, got %q", c)
		}
	}
}

// TestCountInPlaysLeadInClicks guards S3.2: a 1-bar count-in emits four
// clicks (accented first beat) before any note, and CountIn is a no-op when
// the realtime synth is not running.
func TestCountInPlaysLeadInClicks(t *testing.T) {
	log := writeFakeFluidsynth(t)
	s := NewSynth()
	startFakeRealtime(t, s)
	defer s.Stop()

	s.CountIn(1, 120)
	clicks := 0
	accents := 0
	for _, c := range synthLogCommands(t, log) {
		if strings.HasPrefix(c, "noteon 0 37 ") {
			clicks++
			if strings.HasSuffix(c, "120") {
				accents++
			}
		}
	}
	if clicks != 4 || accents != 1 {
		t.Fatalf("expected 4 clicks with 1 accent, got %d clicks %d accents", clicks, accents)
	}

	// No-op without a running realtime synth.
	s2 := NewSynth()
	before := len(synthLogCommands(t, log))
	s2.CountIn(2, 120)
	if got := len(synthLogCommands(t, log)); got != before {
		t.Fatalf("CountIn without realtime synth must not send commands, got %d -> %d", before, got)
	}
}

// TestBeatColumns guards the beat derivation: quarters click on every note,
// eighths on every other note, and the first note is always a beat.
func TestBeatColumns(t *testing.T) {
	quarters := model.Bar{Strings: []model.StringLine{{
		Segments: []model.Segment{
			{Char: '0', Value: 0, Position: 0, Width: 1},
			{Char: '-', Position: 1}, {Char: '-', Position: 2}, {Char: '-', Position: 3},
			{Char: '3', Value: 3, Position: 4, Width: 1},
		},
	}}}
	if got := BeatColumns(quarters); len(got) != 2 {
		t.Fatalf("quarter-note bar should have 2 beats, got %v", got)
	}

	eighths := model.Bar{Strings: []model.StringLine{{
		Segments: []model.Segment{
			{Char: '0', Value: 0, Position: 0, Width: 1},
			{Char: '-', Position: 1},
			{Char: '3', Value: 3, Position: 2, Width: 1},
			{Char: '-', Position: 3},
			{Char: '5', Value: 5, Position: 4, Width: 1},
		},
	}}}
	got := BeatColumns(eighths)
	if len(got) != 2 || got[0] != 0 || got[1] != 4 {
		t.Fatalf("eighth-note bar should beat on cols 0 and 4, got %v", got)
	}
}
