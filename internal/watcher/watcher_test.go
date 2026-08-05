package watcher

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIsTabImportPath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"song.txt", true},
		{"song.gp5", true},
		{"song.gpx", true},
		{"song.mp3", false},
		{"README", false},
	}
	for _, tc := range cases {
		if got := isTabImportPath(tc.path); got != tc.want {
			t.Fatalf("isTabImportPath(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestIsWatchedPath(t *testing.T) {
	dir := "/watch"
	if !isWatchedPath(dir, "/watch/new.txt") {
		t.Fatal("expected txt under watch dir")
	}
	if isWatchedPath(dir, "/other/new.txt") {
		t.Fatal("expected path outside watch dir to be ignored")
	}
}

// TestWatcherCloseIsIdempotent guards against the quit-path panic: the TUI
// shuts the watcher down on q/ctrl+c and the CLI cleanup calls Shutdown on the
// same model afterwards, which used to double-close the done channel.
func TestWatcherCloseIsIdempotent(t *testing.T) {
	w, err := NewWatcher(t.TempDir())
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	// A debounce timer may be armed when Close runs.
	fake := filepath.Join(t.TempDir(), "song.txt")
	if err := os.WriteFile(fake, []byte("tab"), 0644); err != nil {
		t.Fatal(err)
	}
	w.scheduleImport(fake)

	w.Close()
	w.Close() // must not panic
	w.Close()

	// The event loop must observe the closed done channel.
	select {
	case <-w.done:
	case <-time.After(2 * time.Second):
		t.Fatal("done channel was not closed")
	}
}
