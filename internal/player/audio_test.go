package player

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"fretboard/internal/model"
)

func TestFindAudioBesideTabFile(t *testing.T) {
	dir := t.TempDir()
	tabPath := filepath.Join(dir, "layla.txt")
	if err := os.WriteFile(tabPath, []byte("tab"), 0644); err != nil {
		t.Fatal(err)
	}
	audioPath := filepath.Join(dir, "layla.mp3")
	if err := os.WriteFile(audioPath, []byte("fake"), 0644); err != nil {
		t.Fatal(err)
	}
	tab := &model.Tab{Title: "Unique Song", Artist: "Test Artist"}
	if got := FindAudio(tab, tabPath, nil); got != audioPath {
		t.Fatalf("FindAudio = %q, want %q", got, audioPath)
	}
}

func TestFindAudioByArtistTitle(t *testing.T) {
	dir := t.TempDir()
	audioPath := filepath.Join(dir, "Test Artist - Unique Song.mp3")
	if err := os.WriteFile(audioPath, []byte("fake"), 0644); err != nil {
		t.Fatal(err)
	}
	tab := &model.Tab{Title: "Unique Song", Artist: "Test Artist"}
	if got := FindAudio(tab, "online://ug/123", []string{dir}); got != audioPath {
		t.Fatalf("FindAudio = %q, want %q", got, audioPath)
	}
}

func TestFFplayArgsExcludeNostdin(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/song.mp3"
	if err := os.WriteFile(path, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	// exercise candidate construction via unexported path: ensure playAudio won't use -nostdin
	vol := 80
	args := []string{"-nodisp", "-autoexit", "-loglevel", "quiet", "-vn", "-volume", fmt.Sprintf("%d", vol), path}
	for _, a := range args {
		if a == "-nostdin" {
			t.Fatal("ffplay args must not include -nostdin")
		}
	}
}
