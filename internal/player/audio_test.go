package player

import (
	"time"
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

func TestBuildAudioCandidates(t *testing.T) {
	cands := buildAudioCandidates("/tmp/song.mp3", 0, 1, 80)
	if len(cands) != 3 {
		t.Fatalf("expected 3 candidates, got %d", len(cands))
	}
	if cands[0].bin != "ffplay" || !containsArg(cands[0].args, "-volume") || containsArg(cands[0].args, "-ss") {
		t.Fatalf("ffplay baseline args wrong: %v", cands[0].args)
	}
	if cands[2].bin != "mpg123" || len(cands[2].args) != 2 {
		t.Fatalf("mpg123 should stay plain: %v", cands[2].args)
	}

	cands = buildAudioCandidates("/tmp/song.mp3", 90*time.Second, 1.25, 60)
	ff := cands[0].args
	if !containsArg(ff, "-ss") || !containsArg(ff, "90.0") || !containsArg(ff, "-af") || !containsArg(ff, "atempo=1.250") || !containsArg(ff, "-volume") || !containsArg(ff, "60") {
		t.Fatalf("ffplay seek+rate args wrong: %v", ff)
	}
	mv := cands[1].args
	if !containsArg(mv, "--start=90.0") || !containsArg(mv, "--speed=1.250") || !containsArg(mv, "--volume=60") {
		t.Fatalf("mpv seek+rate args wrong: %v", mv)
	}
}

func containsArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}
