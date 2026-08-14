package player

import (
	"errors"
	"fmt"
	"io"
	"time"
)

// fluidsynthArgsRealtime builds the realtime fluidsynth command line.
// "default" omits -a so fluidsynth auto-selects a working audio driver.
func fluidsynthArgsRealtime(driver, gain, sf string) []string {
	if driver == "default" {
		return []string{"-q", "-g", gain, "-r", "44100", sf}
	}
	return []string{"-q", "-a", driver, "-g", gain, "-r", "44100", sf}
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
