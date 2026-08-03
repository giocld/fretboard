// Package testutil provides cross-platform test helpers for redirecting
// user configuration to temporary directories.
package testutil

import (
	"runtime"
	"testing"
)

// WithConfigDir points the OS-specific user config dir at a fresh temp dir
// and calls fn with that temp dir as the base. Uses the same env vars Go's
// os.UserConfigDir honors on each platform so tests never touch the real
// user configuration.
func WithConfigDir(t *testing.T, fn func(base string)) {
	t.Helper()
	base := t.TempDir()
	switch runtime.GOOS {
	case "windows":
		t.Setenv("APPDATA", base)
	case "darwin":
		t.Setenv("HOME", base)
	default:
		t.Setenv("XDG_CONFIG_HOME", base)
	}
	fn(base)
}
