package player

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// ResolveSoundfont returns a usable GM soundfont path, checking config/env and
// common distro install locations.
func ResolveSoundfont() string {
	return findSoundfont()
}

// SynthAvailable reports whether fluidsynth or timidity is on PATH.
func SynthAvailable() bool {
	_, err := lookPath("fluidsynth")
	if err == nil {
		return true
	}
	_, err = lookPath("timidity")
	return err == nil
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
	if len(evts) == 0 {
		return errors.New("no MIDI notes in tab — nothing to play")
	}
	data, err := WriteSMF(evts, bpm)
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

func summarizeStderr(msg string) string {
	if msg == "" {
		return msg
	}
	lines := strings.Split(msg, "\n")
	var useful []string
	for _, line := range lines {
		l := strings.TrimSpace(line)
		if l == "" {
			continue
		}
		lower := strings.ToLower(l)
		if strings.Contains(lower, "error") || strings.Contains(lower, "failed") || strings.Contains(lower, "cannot") {
			useful = append(useful, l)
		}
	}
	if len(useful) == 0 {
		if len(lines) > 6 {
			return strings.Join(lines[len(lines)-6:], "\n")
		}
		return msg
	}
	if len(useful) > 4 {
		useful = useful[len(useful)-4:]
	}
	return strings.Join(useful, "; ")
}

// stderrCollector drains synthesizer stderr without blocking the child process.
// ALSA/Pulse warnings can be voluminous; a blocking pipe fills and silences audio.
type stderrCollector struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (c *stderrCollector) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.buf.Len() < 8192 {
		_, _ = c.buf.Write(p)
	}
	return len(p), nil
}

func (c *stderrCollector) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return strings.TrimSpace(c.buf.String())
}

type candidate struct {
	bin    string
	driver string
	args   []string
}

// processAlive is implemented per platform in process_unix.go and
// process_windows.go: Unix uses kill(pid, 0); Windows queries the process
// exit code because signal-based probes are unsupported there.

// synthCandidates returns synthesizer commands to try in order.
func (s *Synth) synthCandidates(midPath string) ([]candidate, error) {
	sf := s.Soundfont
	if sf == "" {
		sf = findSoundfont()
	}

	gain := fmt.Sprintf("%.2f", float64(s.Volume)/100.0*2.0)
	if s.Volume <= 0 {
		gain = "0.0"
	}

	var candidates []candidate
	if sf != "" {
		// Try the auto-detected default driver first: fluidsynth picks a
		// working backend itself, so a single spawn succeeds without cycling
		// audio devices. Platform-specific drivers are fallbacks only.
		for _, driver := range audioDrivers() {
			candidates = append(candidates, candidate{
				bin:    "fluidsynth",
				driver: driver,
				args:   fluidsynthArgs(driver, gain, sf, midPath),
			})
		}
	} else {
		return nil, errors.New(noSoundfontMessage())
	}

	candidates = append(candidates,
		candidate{bin: "timidity", driver: "timidity", args: []string{"-iA", "-q", midPath}},
		candidate{bin: "timidity", driver: "timidity", args: []string{midPath}},
	)
	return candidates, nil
}

func noSoundfontMessage() string {
	return "no soundfont found — install soundfont-fluid or set FRETBOARD_SOUNDFONT"
}

// fluidsynthArgs builds the fluidsynth command line. "default" omits -a so
// fluidsynth auto-selects a working audio driver.
func fluidsynthArgs(driver, gain, sf, midPath string) []string {
	if driver == "default" || driver == "" {
		return []string{"-ni", "-q", "-g", gain, "-r", "44100", sf, midPath}
	}
	return []string{"-ni", "-q", "-a", driver, "-g", gain, "-r", "44100", sf, midPath}
}

// findSoundfont returns the first existing GM soundfont or "" if none found.
func findSoundfont() string {
	if sf := os.Getenv("FRETBOARD_SOUNDFONT"); sf != "" && fileExists(sf) {
		return sf
	}
	names := []string{"FluidR3_GM.sf2", "GeneralUser_GS.sf2", "default.sf2", "default_gs.sf2"}
	for _, dir := range soundfontSearchDirs() {
		for _, name := range names {
			if p := filepath.Join(dir, name); fileExists(p) {
				return p
			}
		}
	}
	for _, dir := range soundfontSearchDirs() {
		for _, pattern := range []string{"*.sf2", "*.sf3"} {
			if matches, _ := filepath.Glob(filepath.Join(dir, pattern)); len(matches) > 0 {
				return matches[0]
			}
		}
	}
	return ""
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

func expandHome(p string) string {
	if p == "" || p[0] != '~' {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	if len(p) == 1 {
		return home
	}
	return filepath.Join(home, p[1:])
}
