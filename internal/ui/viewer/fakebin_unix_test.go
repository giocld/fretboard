//go:build !windows

package viewer

import (
	"os"
	"path/filepath"
	"testing"
)

// writeFakeMPVTest writes a fake mpv shell script that prints mpv-style
// status lines (position feedback) in a loop, then prepends its directory to
// PATH. status is the JSON-ish line, e.g. {"pos": 1.5, "dur": 100}.
func writeFakeMPVTest(t *testing.T, status string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "mpv")
	script := "#!/bin/sh\nwhile true; do echo '" + status + "'; sleep 0.2; done\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// writeFakeFluidsynthTest writes a fake fluidsynth shell script that echoes
// every stdin command line to synth.log and prepends its directory to PATH.
// It returns the log path so tests can assert on the commands the synth
// received (hermetic copy of the player-package fake so the viewer can drive
// the real engine end to end).
func writeFakeFluidsynthTest(t *testing.T) string {
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
