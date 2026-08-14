package player

import (
	"errors"
	"fmt"
	"io"
	"os/exec"
	"time"

	"fretboard/internal/model"
)

// StartRealtime launches fluidsynth in shell mode for step-synchronized playback.
func (s *Synth) StartRealtime() error {
	if s.running {
		if err := s.Stop(); err != nil {
			return fmt.Errorf("stop previous playback: %w", err)
		}
	}

	sf := s.Soundfont
	if sf == "" {
		sf = findSoundfont()
	}
	if sf == "" {
		return errors.New(noSoundfontMessage())
	}

	gain := fmt.Sprintf("%.2f", float64(s.Volume)/100.0*2.0)
	if s.Volume <= 0 {
		gain = "0.0"
	}

	var candidates []candidate
	for _, driver := range audioDrivers() {
		candidates = append(candidates, candidate{
			bin:    "fluidsynth",
			driver: driver,
			args:   fluidsynthArgsRealtime(driver, gain, sf),
		})
	}

	var lastErr error
	for _, c := range candidates {
		path, err := lookPath(c.bin)
		if err != nil {
			continue
		}
		cmd := exec.Command(path, c.args...)
		cmd.SysProcAttr = childProcAttr()
		var stderr stderrCollector
		cmd.Stderr = &stderr
		stdin, err := cmd.StdinPipe()
		if err != nil {
			lastErr = err
			continue
		}
		if err := cmd.Start(); err != nil {
			_ = stdin.Close()
			lastErr = fmt.Errorf("%s %v: %w", path, c.args, err)
			continue
		}
		startReaper(cmd)
		time.Sleep(200 * time.Millisecond)
		if !processAlive(cmd) {
			killProcessTree(cmd)
			_ = stdin.Close()
			msg := stderr.String()
			if msg == "" {
				msg = "process exited immediately (audio backend may be unavailable)"
			}
			lastErr = fmt.Errorf("%s: %s", path, summarizeStderr(msg))
			continue
		}
		s.cmd = cmd
		s.stdin = stdin
		s.realtime = true
		s.activeNotes = nil
		s.running = true
		s.ActiveDriver = c.driver
		s.LastError = ""
		s.mu.Lock()
		s.playGen++
		s.noteGen = make(map[int]int)
		s.mu.Unlock()
		// GM program (channel 0): the selected instrument, steel guitar by
		// default.
		prog := s.Program
		if prog <= 0 {
			prog = 25
		}
		if err := s.sendRealtime(fmt.Sprintf("prog 0 %d", prog)); err != nil {
			killProcessTree(cmd)
			_ = stdin.Close()
			lastErr = err
			continue
		}
		return nil
	}
	if lastErr != nil {
		return fmt.Errorf("realtime playback failed: %w", lastErr)
	}
	return errors.New("no synthesizer found — install fluidsynth")
}

// fluidsynthArgsRealtime builds the realtime fluidsynth command line.
// "default" omits -a so fluidsynth auto-selects a working audio driver.
func fluidsynthArgsRealtime(driver, gain, sf string) []string {
	if driver == "default" {
		return []string{"-q", "-g", gain, "-r", "44100", sf}
	}
	return []string{"-q", "-a", driver, "-g", gain, "-r", "44100", sf}
}

// PlayStep plays the notes at a schedule step through the realtime synth.
// Notes sustain for step.Sustain ticks; each note is released by a delayed
// noteoff goroutine instead of being cut at the next step boundary. With the
// metronome on, a woodblock click sounds on every beat of the bar (accented
// on the first beat).
func (s *Synth) PlayStep(tab *model.Tab, step PlaybackStep, bpm int) error {
	if !s.realtime || s.stdin == nil {
		return errors.New("realtime synth not running")
	}
	if s.Metronome && step.Bar >= 0 && step.Bar < len(tab.Bars) {
		bar := tab.Bars[step.Bar]
		beats := BeatColumns(bar)
		for i, c := range beats {
			if c == step.Col {
				s.Click(i == 0)
				break
			}
		}
	}
	notes, err := NotesAtStep(tab, step)
	if err != nil {
		return err
	}
	sustainTicks := step.Sustain
	if sustainTicks < 1 {
		sustainTicks = step.Ticks
	}
	if sustainTicks < 1 {
		sustainTicks = ticksPerQuarter / 4
	}
	sustainMs := StepDuration(sustainTicks, bpm)
	for _, n := range notes {
		gen, playGen := s.nextGeneration(n.Note)
		if s.noteActive(n.Note) {
			// Re-articulate: a fresh noteon on a still-ringing pitch would
			// otherwise blend with the previous note (the stale noteoff is
			// discarded by the generation guard).
			if err := s.sendRealtime(fmt.Sprintf("noteoff 0 %d", n.Note)); err != nil {
				return err
			}
		}
		if err := s.sendRealtime(fmt.Sprintf("noteon 0 %d 100", n.Note)); err != nil {
			return err
		}
		s.activeNotes = append(s.activeNotes, n.Note)
		s.scheduleNoteOff(n.Note, gen, playGen, sustainMs)
	}
	return nil
}

// Click sounds a metronome click (GM woodblock) with a short release.
// Accented clicks are louder and land on the first beat of a bar.
func (s *Synth) Click(accent bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stdin == nil || !s.realtime {
		return
	}
	vel := 80
	if accent {
		vel = 120
	}
	_ = s.sendRealtimeLocked(fmt.Sprintf("noteon 0 %d %d", clickNote, vel))
	gen := s.noteGen[clickNote]
	s.noteGen[clickNote] = gen + 1
	playGen := s.playGen
	go func() {
		time.Sleep(70 * time.Millisecond)
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.playGen != playGen || s.noteGen[clickNote] != gen+1 || s.stdin == nil {
			return
		}
		_ = s.sendRealtimeLocked(fmt.Sprintf("noteoff 0 %d", clickNote))
	}()
}

// CountIn plays countInBars bars of metronome clicks (4/4 assumed) at the
// given BPM and blocks until done — used before the first tab step so the
// musician gets a lead-in. A no-op when countInBars <= 0 or the realtime
// synth is not running.
func (s *Synth) CountIn(countInBars, bpm int) {
	if countInBars <= 0 || !s.realtime || s.stdin == nil {
		return
	}
	if bpm <= 0 {
		bpm = 120
	}
	clicks := countInBars * 4
	interval := time.Duration(60_000/bpm) * time.Millisecond
	for i := 0; i < clicks; i++ {
		s.Click(i%4 == 0)
		time.Sleep(interval)
	}
}

func (s *Synth) noteActive(pitch int) bool {
	for _, p := range s.activeNotes {
		if p == pitch {
			return true
		}
	}
	return false
}

// NotesAtStep returns MIDI notes sounding at the given playback step.
func NotesAtStep(tab *model.Tab, step PlaybackStep) ([]note, error) {
	if tab == nil {
		return nil, errors.New("nil tab")
	}
	if len(tab.Tuning) == 0 {
		return nil, errors.New("tab has no tuning")
	}
	if step.Bar < 0 || step.Bar >= len(tab.Bars) {
		return nil, nil
	}
	bar := tab.Bars[step.Bar]
	notes, _ := collectNotesAt(tab.Tuning, bar.Strings, step.Col)
	return notes, nil
}

func (s *Synth) sendRealtime(cmd string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sendRealtimeLocked(cmd)
}

func (s *Synth) sendRealtimeLocked(cmd string) error {
	if s.stdin == nil {
		return errors.New("realtime synth stdin closed")
	}
	if _, err := io.WriteString(s.stdin, cmd+"\n"); err != nil {
		return err
	}
	return nil
}

// nextGeneration bumps the per-pitch generation under lock so any older
// scheduled noteoff for the same pitch becomes stale before the new noteon
// is sent, and returns the current playback epoch so the noteoff goroutine
// knows which session it belongs to.
func (s *Synth) nextGeneration(pitch int) (int, uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.noteGen == nil {
		s.noteGen = make(map[int]int)
	}
	s.noteGen[pitch]++
	return s.noteGen[pitch], s.playGen
}

// scheduleNoteOff waits sustainMs then releases the note, but only if the
// pitch is still the same generation and the same playback epoch is running,
// so a re-trigger or a restarted synth is never silenced by a stale noteoff.
func (s *Synth) scheduleNoteOff(pitch, gen int, playGen uint64, sustainMs int64) {
	go func() {
		time.Sleep(time.Duration(sustainMs) * time.Millisecond)
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.playGen != playGen || s.noteGen[pitch] != gen || s.stdin == nil {
			return
		}
		_ = s.sendRealtimeLocked(fmt.Sprintf("noteoff 0 %d", pitch))
	}()
}

func (s *Synth) noteOffActive() {
	for _, n := range s.activeNotes {
		_ = s.sendRealtime(fmt.Sprintf("noteoff 0 %d", n))
	}
	s.activeNotes = nil
}

func (s *Synth) stopRealtime() {
	s.mu.Lock()
	if s.stdin != nil {
		_ = s.sendRealtimeLocked("quit")
		_ = s.stdin.Close()
		s.stdin = nil
	}
	s.playGen++
	s.activeNotes = nil
	s.mu.Unlock()
	s.realtime = false
}
