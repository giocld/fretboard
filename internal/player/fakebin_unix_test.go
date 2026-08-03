//go:build !windows

package player

import (
	"os"
	"path/filepath"
	"testing"
)

// writeFakeYtDlp writes a fake yt-dlp that sleeps for the given number of
// seconds, and prepends its directory to PATH.
func writeFakeYtDlp(t *testing.T, sleep string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "yt-dlp")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexec sleep "+sleep+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
