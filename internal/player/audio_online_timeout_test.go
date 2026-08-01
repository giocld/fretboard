package player

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/YOUR_USERNAME/fretboard/internal/model"
)

func writeFakeYtDlp(t *testing.T, sleep string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "yt-dlp")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexec sleep "+sleep+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestYTSearchTimeoutKillsHangingYtDlp(t *testing.T) {
	writeFakeYtDlp(t, "60")

	old := ytSearchTimeout
	ytSearchTimeout = 1 * time.Second
	defer func() { ytSearchTimeout = old }()

	done := make(chan error, 1)
	go func() {
		_, err := ytSearch("test query", 5)
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error from hanging yt-dlp")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("ytSearch did not return within deadline; child process not killed")
	}
}

func TestDownloadYouTubeAudioTimeoutKillsHangingYtDlp(t *testing.T) {
	writeFakeYtDlp(t, "60")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	old := ytDownloadTimeout
	ytDownloadTimeout = 1 * time.Second
	defer func() { ytDownloadTimeout = old }()

	done := make(chan error, 1)
	go func() {
		_, err := DownloadYouTubeAudio(&model.Tab{Artist: "Test", Title: "Track"}, "abc123")
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error from hanging yt-dlp download")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("DownloadYouTubeAudio did not return within deadline; child process not killed")
	}
}
