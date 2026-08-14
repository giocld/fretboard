package player

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

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

func summarizeStderr(msg string) string {
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

	if sf == "" {
		return nil, errors.New(noSoundfontMessage())
	}

	// Try the auto-detected default driver first: fluidsynth picks a
	// working backend itself, so a single spawn succeeds without cycling
	// audio devices. Platform-specific drivers are fallbacks only.
	var candidates []candidate
	for _, driver := range audioDrivers() {
		candidates = append(candidates, candidate{
			bin:    "fluidsynth",
			driver: driver,
			args:   fluidsynthArgs(driver, gain, sf, midPath),
		})
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
	if driver == "default" {
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
