//go:build windows

package player

import (
	"os"
	"path/filepath"
	"strings"
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

// writeFailingYtDlp writes a fake yt-dlp.cmd that exits with status 1 (search
// failure) and prepends its directory to PATH.
func writeFailingYtDlp(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "yt-dlp.cmd")
	script := "@echo off\r\nexit /b 1\r\n"
	if err := os.WriteFile(path, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// writeEmptyYtDlp writes a fake yt-dlp.cmd that prints an empty playlist JSON
// (a successful search with zero hits) and prepends its directory to PATH.
func writeEmptyYtDlp(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "yt-dlp.cmd")
	script := "@echo off\r\necho {\"entries\":[]}\r\n"
	if err := os.WriteFile(path, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// writeFakeFFmpeg writes a fake ffmpeg.cmd that echoes the given
// silencedetect log (or nothing for a no-silence run) and prepends its
// directory to PATH.
func writeFakeFFmpeg(t *testing.T, log string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "ffmpeg.cmd")
	var sb strings.Builder
	sb.WriteString("@echo off\r\n")
	for _, line := range splitLines(log) {
		sb.WriteString("echo " + strings.ReplaceAll(line, "|", "^|") + "\r\n")
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
}

// writeFakeFluidsynth writes a fake fluidsynth.cmd that echoes every stdin
// command line to synth.log and prepends its directory to PATH. It returns
// the log path so tests can assert on the commands the synth received.
func writeFakeFluidsynth(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fluidsynth.cmd")
	log := filepath.Join(dir, "synth.log")
	script := "@echo off\r\n" +
		"setlocal enabledelayedexpansion\r\n" +
		":loop\r\n" +
		"set \"line=\"\r\n" +
		"set /p line=\r\n" +
		"if not defined line goto loop\r\n" +
		"echo !line!>> \"" + log + "\"\r\n" +
		"goto loop\r\n"
	if err := os.WriteFile(path, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}
