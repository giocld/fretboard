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

// writeFailingYtDlp writes a fake yt-dlp that exits with status 1 (search
// failure) and prepends its directory to PATH.
func writeFailingYtDlp(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "yt-dlp")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// writeEmptyYtDlp writes a fake yt-dlp that prints an empty playlist JSON (a
// successful search with zero hits) and prepends its directory to PATH.
func writeEmptyYtDlp(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "yt-dlp")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf '{\"entries\":[]}\\n'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// writeFakeFFmpeg writes a fake ffmpeg that prints the given silencedetect
// log (or nothing for a no-silence run) and prepends its directory to PATH.
func writeFakeFFmpeg(t *testing.T, log string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "ffmpeg")
	script := "#!/bin/sh\ncat <<'FREEOF'\n" + log + "\nFREEOF\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
