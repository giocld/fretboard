//go:build windows

package player

import (
	"os"
	"path/filepath"
	"testing"
)

// writeFakeYtDlp writes a fake yt-dlp.cmd that loops forever inside cmd.exe
// (no child process), so killing the process is clean and immediate.
func writeFakeYtDlp(t *testing.T, _ string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "yt-dlp.cmd")
	script := "@echo off\r\n:loop\r\ngoto loop\r\n"
	if err := os.WriteFile(path, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
