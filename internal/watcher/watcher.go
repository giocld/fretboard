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

	debounceMu sync.Mutex
	debounce   map[string]*time.Timer
}

// NewWatcher creates and starts a watcher for dir.
func NewWatcher(dir string) (*Watcher, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("watcher: %w", err)
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("watcher: %w", err)
	}
	if err := w.Add(dir); err != nil {
		w.Close()
		return nil, fmt.Errorf("watcher: %w", err)
	}
	watcher := &Watcher{
		Events:   make(chan FileAddedMsg),
		dir:      dir,
		done:     make(chan struct{}),
		debounce: make(map[string]*time.Timer),
	}
	go watcher.loop(w)
	return watcher, nil
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

// Close stops the watcher.
func (w *Watcher) Close() {
	if w == nil {
		return
	}
	close(w.done)
	w.debounceMu.Lock()
	for _, timer := range w.debounce {
		timer.Stop()
	}
	w.debounce = nil
	w.debounceMu.Unlock()
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
			if ev.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Rename) != 0 && isTabImportPath(ev.Name) {
				w.scheduleImport(ev.Name)
			}
		case err, ok := <-fw.Errors:
			if !ok {
				return
			}
			_ = err
		case <-w.done:
			return
		}
	}
}

// IsWatchedPath returns true if path is a .txt file under dir.
func IsWatchedPath(dir, path string) bool {
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
