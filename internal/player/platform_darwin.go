//go:build darwin

package player

import "os/exec"

// platformFluidDrivers returns fluidsynth audio drivers to try after the
// auto-detected default on macOS.
func platformFluidDrivers() []string {
	return []string{"coreaudio"}
}

// platformSoundfontDirs returns system-wide soundfont directories on macOS
// (Homebrew installs the fluid-synth soundfont alongside the app).
func platformSoundfontDirs() []string {
	return []string{"/opt/homebrew/share/soundfonts", "/usr/local/share/soundfonts"}
}

// platformBinLookup is a no-op on macOS: Homebrew bins are on PATH.
func platformBinLookup(string) (string, error) {
	return "", exec.ErrNotFound
}
