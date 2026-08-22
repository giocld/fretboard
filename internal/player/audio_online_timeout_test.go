//go:build !noytdlp

package player

import (
	"testing"
	"time"

	"fretboard/internal/model"
	"fretboard/internal/testutil"
)

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
	testutil.WithConfigDir(t, func(string) {})

	old := ytDownloadTimeout
	ytDownloadTimeout = 1 * time.Second
	defer func() { ytDownloadTimeout = old }()

	done := make(chan error, 1)
	go func() {
		_, err := DownloadYouTubeAudio(&model.Tab{Artist: "Test", Title: "Track"}, "abc123", 0)
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
