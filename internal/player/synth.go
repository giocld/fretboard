package player

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"fretboard/internal/model"
)

// Synth plays generated MIDI events through an external synthesizer.
// It writes a temporary SMF file and shells out to fluidsynth or timidity.
type Synth struct {
	cmd          *exec.Cmd
	stdin        io.WriteCloser
	running      bool
	realtime     bool
	activeNotes  []int
	Volume       int // 0-100
	Soundfont    string
	ActiveDriver string
	LastError    string
	// Practice helpers (realtime MIDI only).
	Metronome bool // click on every beat (GM 37 woodblock)
	Program   int  // GM program for channel 0; 0 = default (25, steel guitar)

	mu      sync.Mutex
	noteGen map[int]int
	playGen uint64
}

// clickNote is the GM woodblock used for metronome and count-in clicks.
const clickNote = 37

// NewSynth creates a stopped synth.
func NewSynth() *Synth {
	return &Synth{Volume: 80}
}

// Play generates MIDI from tab and starts playback.
func (s *Synth) Play(tab *model.Tab, bpm int) error {
	if s.running {
		if err := s.Stop(); err != nil {
			return fmt.Errorf("stop previous playback: %w", err)
		}
	}
	if tab != nil && len(tab.Tuning) == 0 {
		tab.Tuning = model.Standard
	}
	evts, err := Events(tab, bpm)
	if err != nil {
		return fmt.Errorf("generate events: %w", err)
	}
	// Earliest shared point before SMF serialization: fluidsynth and
	// timidity both read the same .mid file, so humanizing here covers
	// every external synth path (realtime mode drives fluidsynth from
	// tab notes directly and stays untouched). No-op unless HumanizeMIDI.
	evts = HumanizeEvents(evts)
	if len(evts) == 0 {
		return errors.New("no MIDI notes in tab — nothing to play")
	}
	// Drum tabs route to GM channel 10 (WriteTabSMF maps string indices to
	// percussion pitches); anything else is byte-identical to WriteSMF.
	data, err := WriteTabSMF(evts, bpm, tab)
	if err != nil {
		return fmt.Errorf("write smf: %w", err)
	}
	midPath := filepath.Join(os.TempDir(), "fretboard_playback.mid")
	if err := os.WriteFile(midPath, data, 0644); err != nil {
		return fmt.Errorf("write mid file: %w", err)
	}

	candidates, err := s.synthCandidates(midPath)
	if err != nil {
		return err
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
		cmd.Stdout = io.Discard
		if err := cmd.Start(); err != nil {
			lastErr = fmt.Errorf("%s %v: %w", path, c.args, err)
			continue
		}
		startReaper(cmd)
		time.Sleep(150 * time.Millisecond)
		if !processAlive(cmd) {
			killProcessTree(cmd)
			msg := stderr.String()
			if msg == "" {
				msg = "process exited immediately (audio backend may be unavailable)"
			}
			lastErr = fmt.Errorf("%s: %s", path, summarizeStderr(msg))
			continue
		}
		s.cmd = cmd
		s.running = true
		s.ActiveDriver = c.driver
		s.LastError = ""
		return nil
	}
	if lastErr != nil {
		return fmt.Errorf("playback failed: %w", lastErr)
	}
	return errors.New("no synthesizer found — install fluidsynth")
}

// Stop kills the running synthesizer process.
func (s *Synth) Stop() error {
	if s.realtime {
		s.noteOffActive()
		s.stopRealtime()
	}
	if s.cmd == nil || s.cmd.Process == nil {
		s.running = false
		s.ActiveDriver = ""
		return nil
	}
	killProcessTree(s.cmd)
	s.running = false
	s.cmd = nil
	s.ActiveDriver = ""
	return nil
}

// Running reports whether playback is active.
func (s *Synth) Running() bool {
	if s.cmd == nil || s.cmd.Process == nil {
		return false
	}
	if !processAlive(s.cmd) {
		s.running = false
		if s.LastError == "" {
			s.LastError = "audio engine stopped unexpectedly"
		}
		s.cmd = nil
		return false
	}
	return s.running
}

// PlaybackEnded reports whether the external synth process has finished.
func (s *Synth) PlaybackEnded() bool {
	return !s.Running()
}

// FileExists reports whether path exists on disk.
func FileExists(p string) bool {
	return fileExists(p)
}

func fileExists(p string) bool {
	if p == "" {
		return false
	}
	_, err := os.Stat(p)
	return err == nil
}
