// Package watcher monitors a directory for new .txt tab files and sends events
// to the TUI for auto-import.
package watcher

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fsnotify/fsnotify"
)

const importDebounce = 400 * time.Millisecond

// FileAddedMsg is sent when a new .txt file appears in the watched directory.
type FileAddedMsg struct {
	Path string
}

// Watcher watches a directory and emits events on a channel.
type Watcher struct {
	Events chan FileAddedMsg
	dir    string
	done   chan struct{}
	fw     *fsnotify.Watcher // registered watch handles (test helper)

	closeOnce sync.Once

	debounceMu sync.Mutex
	debounce   map[string]*time.Timer
	rewalkMu   sync.Mutex
	rewalk     *time.Timer // debounced full-tree re-registration
}

// NewWatcher creates and starts a watcher for dir, including every
// subdirectory (recursive), so tabs dropped anywhere under the tree are
// auto-imported.
func NewWatcher(dir string) (*Watcher, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("watcher: %w", err)
	}
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("watcher: %w", err)
	}
	if err := addRecursive(fw, dir); err != nil {
		fw.Close()
		return nil, fmt.Errorf("watcher: %w", err)
	}
	watcher := &Watcher{
		Events:   make(chan FileAddedMsg),
		dir:      dir,
		done:     make(chan struct{}),
		debounce: make(map[string]*time.Timer),
		fw:       fw,
	}
	go watcher.loop(fw)
	return watcher, nil
}

// addRecursive registers dir and all of its subdirectories with fsnotify.
func addRecursive(fw *fsnotify.Watcher, dir string) error {
	var lastErr error
	err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // unreadable subtree: skip, keep the rest
		}
		if info.IsDir() {
			if err := fw.Add(p); err != nil {
				lastErr = err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if lastErr != nil && len(fw.WatchList()) == 0 {
		return lastErr
	}
	return nil
}

// NextEventCmd returns a tea.Cmd that waits for the next file event.
func (w *Watcher) NextEventCmd() tea.Cmd {
	return func() tea.Msg {
		if w == nil {
			return nil
		}
		select {
		case ev := <-w.Events:
			return ev
		case <-w.done:
			return nil
		}
	}
}

// WatcherStartedMsg is sent when a watcher has been created.
type WatcherStartedMsg struct {
	Watcher *Watcher
	Err     error
}

// StartCmd returns a tea.Cmd that creates a watcher for dir and sends a
// WatcherStartedMsg.
func StartCmd(dir string) tea.Cmd {
	return func() tea.Msg {
		w, err := NewWatcher(dir)
		return WatcherStartedMsg{Watcher: w, Err: err}
	}
}

// Close stops the watcher. It is safe to call multiple times: the TUI shuts
// the watcher down on q/ctrl+c and the CLI cleanup path calls Shutdown again
// on the same model, so Close must not close the done channel twice.
func (w *Watcher) Close() {
	if w == nil {
		return
	}
	w.closeOnce.Do(func() {
		close(w.done)
		w.debounceMu.Lock()
		for _, timer := range w.debounce {
			timer.Stop()
		}
		w.debounce = nil
		w.debounceMu.Unlock()
	})
}

// scheduleRewalk re-registers the whole tree shortly after a directory was
// created. A race exists where a subdirectory is created before its parent
// watch is active, so its Create event is never delivered; the re-walk
// closes that gap.
func (w *Watcher) scheduleRewalk() {
	w.rewalkMu.Lock()
	defer w.rewalkMu.Unlock()
	if w.rewalk == nil {
		w.rewalk = time.AfterFunc(300*time.Millisecond, func() {
			w.rewalkMu.Lock()
			w.rewalk = nil
			w.rewalkMu.Unlock()
			_ = addRecursive(w.fw, w.dir)
		})
	}
}

func (w *Watcher) scheduleImport(path string) {
	w.debounceMu.Lock()
	defer w.debounceMu.Unlock()
	if w.debounce == nil {
		return
	}
	if timer, ok := w.debounce[path]; ok {
		timer.Stop()
	}
	w.debounce[path] = time.AfterFunc(importDebounce, func() {
		select {
		case w.Events <- FileAddedMsg{Path: path}:
		case <-w.done:
		}
		w.debounceMu.Lock()
		delete(w.debounce, path)
		w.debounceMu.Unlock()
	})
}

func (w *Watcher) loop(fw *fsnotify.Watcher) {
	defer fw.Close()
	for {
		select {
		case ev, ok := <-fw.Events:
			if !ok {
				return
			}
			if ev.Op&fsnotify.Create != 0 {
				// A new directory under the tree: watch it (and any nested
				// directories it already contains) so deeper tabs are seen.
				if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
					_ = addRecursive(fw, ev.Name)
					w.scheduleRewalk()
				}
			}
			if ev.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Rename) != 0 && isTabImportPath(ev.Name) {
				w.scheduleImport(ev.Name)
			}
		case _, ok := <-fw.Errors:
			if !ok {
				return
			}
		case <-w.done:
			return
		}
	}
}

// isWatchedPath returns true if path is a .txt file under dir.
func isWatchedPath(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return !strings.HasPrefix(rel, "..") && isTabImportPath(path)
}

func isTabImportPath(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".txt", ".gp3", ".gp4", ".gp5", ".gpx":
		return true
	default:
		return false
	}
}

// fsnotifyWatchList returns the registered watch paths (test helper).
func (w *Watcher) fsnotifyWatchList() []string {
	return w.fw.WatchList()
}
