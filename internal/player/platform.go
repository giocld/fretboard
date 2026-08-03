package player

import (
	"os"
	"os/exec"
	"path/filepath"
)

// Platform-specific behavior lives in platform_linux.go, platform_darwin.go,
// platform_windows.go and process_unix.go/process_windows.go. Code in this
// file must run unchanged on every supported OS.

// audioDrivers returns the fluidsynth audio drivers to try, in order, with
// the auto-detected default first and platform fallbacks after it.
func audioDrivers() []string {
	return append([]string{"default"}, platformFluidDrivers()...)
}

// lookPath finds a binary on PATH, falling back to common platform install
// locations when the current process PATH is stale (e.g. chocolatey/scoop).
func lookPath(bin string) (string, error) {
	if path, err := exec.LookPath(bin); err == nil {
		return path, nil
	}
	return platformBinLookup(bin)
}

// soundfontSearchDirs returns directories to search for GM soundfonts,
// most-specific first. System dirs come from the platform, then user dirs.
func soundfontSearchDirs() []string {
	dirs := platformSoundfontDirs()
	if cfg, err := os.UserConfigDir(); err == nil {
		dirs = append(dirs, filepath.Join(cfg, "fretboard"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs,
			filepath.Join(home, ".local", "share", "soundfonts"),
			filepath.Join(home, ".config", "fretboard"),
		)
	}
	return dirs
}
