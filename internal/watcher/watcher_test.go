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

// TestRecursiveWatcherSeesNestedFiles guards S6.2: tabs dropped into
// subdirectories of the watched root are picked up, and directories created
// after startup get watched too.
func TestRecursiveWatcherSeesNestedFiles(t *testing.T) {
	root := t.TempDir()
	w, err := NewWatcher(root)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.Close()

	// A nested directory created after startup must be watched: the
	// Create-event walk plus the debounced re-walk register it. Poll the
	// watchlist so the test never races the registration.
	nested := filepath.Join(root, "rock", "songs")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	waitForWatch(t, w, nested)

	// File in the now-watched nested directory.
	if err := os.WriteFile(filepath.Join(nested, "a.txt"), []byte("tab"), 0o644); err != nil {
		t.Fatal(err)
	}
	waitForEvent(t, w, "a.txt")

	// A deeper directory created after startup, with a subdirectory created
	// before its parent watch could be active.
	late := filepath.Join(root, "new", "sub")
	if err := os.MkdirAll(late, 0o755); err != nil {
		t.Fatal(err)
	}
	waitForWatch(t, w, late)

	if err := os.WriteFile(filepath.Join(late, "b.txt"), []byte("tab"), 0o644); err != nil {
		t.Fatal(err)
	}
	waitForEvent(t, w, "b.txt")
}

// waitForWatch polls the registered watch list until dir is present (the
// registration happens asynchronously after a directory Create event).
func waitForWatch(t *testing.T, w *Watcher, dir string) {
	t.Helper()
	clean := filepath.Clean(dir)
	done := time.After(5 * time.Second)
	for {
		for _, p := range w.fsnotifyWatchList() {
			if filepath.Clean(p) == clean {
				return
			}
		}
		select {
		case <-done:
			t.Fatalf("directory %s never got watched; watchlist: %v", dir, w.fsnotifyWatchList())
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// waitForEvent waits until an event for the named file arrives or fails the
// test after a timeout.
func waitForEvent(t *testing.T, w *Watcher, name string) {
	t.Helper()
	done := time.After(4 * time.Second)
	for {
		select {
		case ev := <-w.Events:
			if filepath.Base(ev.Path) == name {
				return
			}
		case <-done:
			t.Fatalf("event for %s not seen", name)
		}
	}
}

// TestRecursiveWatcherWatchesPreExistingTree guards the walk: NewWatcher
// registers every existing subdirectory.
func TestRecursiveWatcherWatchesPreExistingTree(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	w, err := NewWatcher(root)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.Close()

	if err := os.WriteFile(filepath.Join(deep, "nested.txt"), []byte("tab"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case ev := <-w.Events:
		if filepath.Base(ev.Path) != "nested.txt" {
			t.Fatalf("unexpected event: %v", ev.Path)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("file in pre-existing deep directory was not seen")
	}
}
