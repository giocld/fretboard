//go:build windows

package viewer

import (
	"os"
	"path/filepath"
	"testing"
)

// writeFakeMPVTest writes a fake mpv.cmd that prints mpv-style status lines
// (position feedback) in a loop, then prepends its directory to PATH. status
// is the JSON-ish line, e.g. {"pos": 1.5, "dur": 100}.
func writeFakeMPVTest(t *testing.T, status string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "mpv.cmd")
	script := "@echo off\r\n:loop\r\necho " + status + "\r\nping -n 2 127.0.0.1 >nul\r\ngoto loop\r\n"
	if err := os.WriteFile(path, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// writeFakeFluidsynthTest writes a fake fluidsynth.cmd that logs stdin
// commands to synth.log and prepends its directory to PATH. It returns the
// log path so tests can assert on the commands the synth received (hermetic
// copy of the player-package fake so the viewer can drive the real engine
// end to end).
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
