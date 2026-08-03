//go:build windows

package player

import (
	"os"
	"os/exec"
	"path/filepath"
)

// platformFluidDrivers returns fluidsynth audio drivers to try after the
// auto-detected default on Windows.
func platformFluidDrivers() []string {
	return []string{"dsound", "wasapi", "waveout"}
}

// platformSoundfontDirs returns system-wide soundfont directories on Windows.
// There is no standard location, so user dirs (config, home) are the only
// search paths.
func platformSoundfontDirs() []string {
	return nil
}

// platformBinLookup checks chocolatey and scoop install dirs for binaries
// when the current process PATH is stale.
func platformBinLookup(bin string) (string, error) {
	dirs := []string{
		`C:\ProgramData\chocolatey\bin`,
		filepath.Join(os.Getenv("USERPROFILE"), "scoop", "shims"),
	}
	for _, dir := range dirs {
		for _, ext := range []string{".exe", ".bat", ".cmd"} {
			cand := filepath.Join(dir, bin+ext)
			if fileExists(cand) {
				return cand, nil
			}
		}
	}
	return "", exec.ErrNotFound
}
