//go:build linux

package player

import "os/exec"

// platformFluidDrivers returns fluidsynth audio drivers to try after the
// auto-detected default on Linux.
func platformFluidDrivers() []string {
	return []string{"pulseaudio", "pipewire", "alsa", "jack"}
}

// platformSoundfontDirs returns system-wide soundfont directories on Linux.
func platformSoundfontDirs() []string {
	return []string{"/usr/share/soundfonts", "/usr/share/sounds/sf2"}
}

// platformBinLookup is a no-op on Linux: standard installs are on PATH.
func platformBinLookup(string) (string, error) {
	return "", exec.ErrNotFound
}
