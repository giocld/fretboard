// This file holds the message handlers and private helpers that back the
// top-level router in app.go.
package app

import (
	"fmt"
	"path/filepath"
	"strings"

	"fretboard/internal/config"
	"fretboard/internal/library"
	"fretboard/internal/model"
	"fretboard/internal/scraper"
	"fretboard/internal/ui/browser"
	"fretboard/internal/ui/help"
	"fretboard/internal/ui/home"
	"fretboard/internal/ui/kit"
	"fretboard/internal/ui/msgs"
	"fretboard/internal/ui/search"
	"fretboard/internal/ui/settings"
	"fretboard/internal/ui/viewer"
	"fretboard/internal/watcher"
	tea "github.com/charmbracelet/bubbletea"
)

// NewApp creates a new TUI app for e2e tests and external callers that don't
// need a real library store or online search.
func NewApp() AppModel {
	return AppModel{
		view:     viewHome,
		home:     home.NewHomeModel(nil),
		library:  browser.NewBrowserModel(nil),
		viewer:   viewer.NewViewerModel(),
		search:   search.NewSearchModel(nil),
		help:     help.NewHelpModel(),
		settings: settings.NewSettingsModel(),
	}
}

// NewAppWithOptions creates a new TUI app bound to a library store, optional
// online scraper, optional auto-import directory, and audio search paths.
func NewAppWithOptions(store *library.Store, client *scraper.Client, autoImportPath string, audioSearchPaths []string) AppModel {
	v := viewer.NewViewerModel()
	v.SetAudioDirs(audioSearchPaths)
	return AppModel{
		view:           viewHome,
		store:          store,
		home:           home.NewHomeModel(store),
		library:        browser.NewBrowserModel(store),
		viewer:         v,
		search:         search.NewSearchModel(client),
		help:           help.NewHelpModel(),
		settings:       settings.NewSettingsModel(),
		autoImportPath: autoImportPath,
	}
}

func (m AppModel) textInputActive() bool {
	if m.view == viewSearch && m.search.IsInputActive() {
		return true
	}
	if m.view == viewLibrary && m.library.IsSearchActive() {
		return true
	}
	return false
}

// tabFilePath returns the on-disk path of the live tab. The viewer's in-memory
// path can be empty when the tab came from online search, so fall back to the
// library row for the persisted file.
func (m *AppModel) tabFilePath() string {
	path := strings.TrimSpace(m.viewer.TabPath())
	if path == "" && m.viewer.TabID() > 0 {
		if row, err := m.store.GetRow(m.viewer.TabID()); err == nil && row != nil {
			path = row.Filepath
		}
	}
	return path
}

func (m *AppModel) stopPlayback() {
	m.viewer.StopPlayback()
}

func (m *AppModel) openTab(id int64) tea.Cmd {
	if m.store == nil {
		m.viewer.SetError("Library is not available")
		m.view = viewViewer
		return nil
	}
	tab, err := m.store.Get(id)
	if err != nil || tab == nil {
		m.viewer.LoadTab(nil, "", 0)
		if err != nil {
			m.viewer.SetError(fmt.Sprintf("Could not open tab: %v", err))
		} else {
			m.viewer.SetError("Could not open tab")
		}
		m.view = viewViewer
		return nil
	}
	path := ""
	if row, err := m.store.GetRow(id); err == nil && row != nil {
		path = row.Filepath
	}
	_ = m.store.RecordPlay(id)
	m.stopPlayback()
	m.viewer.LoadTab(tab, path, id)
	m.view = viewViewer
	cfg, _ := config.Load()
	return tea.Batch(m.viewer.BeginAudioFetch(cfg.AutoFetchAudio), m.library.Init(), m.home.Init())
}

func (m *AppModel) cycleTheme() {
	names := kit.ThemeNames()
	current := kit.CurrentTheme().Name
	idx := 0
	for i, n := range names {
		if n == current {
			idx = (i + 1) % len(names)
			break
		}
	}
	kit.SetTheme(names[idx])
	cfg, _ := config.Load()
	cfg.ThemeName = names[idx]
	_ = config.Save(cfg)
}

// RestoreSession opens the last-used tab at its saved cursor position and
// settings; returns a startup command (nil when there is nothing to resume).
func (m *AppModel) RestoreSession() tea.Cmd {
	s := config.LoadSession()
	if s.TabID <= 0 || m.store == nil {
		return nil
	}
	cmd := m.openTab(s.TabID)
	if m.viewer.Tab() != nil {
		if s.Bar >= 1 {
			m.viewer.SetCursorBar(s.Bar - 1)
		}
		if s.BPM >= 40 && s.BPM <= 300 {
			m.viewer.SetBPM(s.BPM)
		}
		m.viewer.SetLinear(s.Linear)
	}
	return cmd
}

// SetStartupCmd queues a command to run on the first Init.
func (m *AppModel) SetStartupCmd(cmd tea.Cmd) {
	m.startupCmd = cmd
}

// Shutdown stops audio and releases background resources. The live tab's
// metadata (calibration, practice time) is persisted on the way out, and the
// session (tab + cursor + settings) is saved for the next start.
func (m *AppModel) Shutdown() {
	m.viewer.StopPlayback()
	m.viewer.ShutdownAudio()
	if m.store != nil && m.viewer.Tab() != nil {
		if path := m.tabFilePath(); path != "" {
			_, _ = m.store.Import(path, m.viewer.Tab())
		}
	}
	if m.viewer.Tab() != nil {
		_ = config.SaveSession(config.Session{
			TabID:   m.viewer.TabID(),
			TabPath: m.viewer.TabPath(),
			Bar:     m.viewer.CursorBar() + 1,
			BPM:     m.viewer.BPM(),
			Linear:  m.viewer.Linear(),
		})
	}
	if m.watcher != nil {
		m.watcher.Close()
	}
}

// LoadViewerTab is a helper for tests and external callers to load a tab into
// the viewer without going through the browser.
func (m *AppModel) LoadViewerTab(tab *model.Tab, tabPath string) tea.Cmd {
	m.stopPlayback()
	m.viewer.LoadTab(tab, tabPath, 0)
	m.view = viewViewer
	cfg, _ := config.Load()
	m.startupCmd = m.viewer.BeginAudioFetch(cfg.AutoFetchAudio)
	return m.startupCmd
}

// SetVolume sets the synthesizer volume (0-100).
func (m *AppModel) SetVolume(v int) {
	m.viewer.SetVolume(v)
}

// SetStrictAudio toggles studio-lock audio selection.
func (m *AppModel) SetStrictAudio(v bool) {
	m.viewer.SetStrictAudio(v)
}

// SetSoundfont sets the path to the soundfont used by the synthesizer.
func (m *AppModel) SetSoundfont(path string) {
	m.viewer.SetSoundfont(path)
}

// handleTabImportError forwards tab import errors to the search view.
func (m *AppModel) handleTabImportError(msg msgs.TabImportErrorMsg) tea.Cmd {
	if !m.search.AcceptsGen(msg.Gen) {
		return nil
	}
	m.search.SetBusy(false, false)
	newSearch, cmd := m.search.Update(msg)
	m.search = newSearch
	return cmd
}

// handleTabFetched imports and opens a tab fetched from online search.
func (m *AppModel) handleTabFetched(msg msgs.TabFetchedMsg) tea.Cmd {
	if !m.search.AcceptsGen(msg.Gen) {
		return nil
	}
	m.search.SetBusy(false, false)
	m.stopPlayback()
	if msg.Tab == nil {
		m.viewer.LoadTab(nil, "", 0)
		m.viewer.SetError("Could not load tab")
		m.view = viewViewer
		return nil
	}
	var cmds []tea.Cmd
	tabPath := search.OnlineTabPath(msg.Source)
	tabID := int64(0)
	if m.store != nil {
		id, err := m.store.Import(tabPath, msg.Tab)
		if err != nil {
			m.viewer.LoadTab(msg.Tab, tabPath, 0)
			m.viewer.SetError(fmt.Sprintf("Tab opened but not saved: %v", err))
			m.view = viewViewer
			cfg, _ := config.Load()
			return m.viewer.BeginAudioFetch(cfg.AutoFetchAudio)
		}
		tabID = id
		_ = m.store.RecordPlay(id)
		if row, rowErr := m.store.GetRow(id); rowErr == nil && row != nil {
			tabPath = row.Filepath
		}
		cmds = append(cmds, m.library.Init(), m.home.Init())
	}
	m.viewer.LoadTab(msg.Tab, tabPath, tabID)
	m.view = viewViewer
	cfg, _ := config.Load()
	cmds = append(cmds, m.viewer.BeginAudioFetch(cfg.AutoFetchAudio))
	if len(cmds) > 0 {
		return tea.Batch(cmds...)
	}
	return nil
}

// handleFileAdded auto-imports a newly detected file.
func (m *AppModel) handleFileAdded(msg watcher.FileAddedMsg) tea.Cmd {
	var cmd tea.Cmd
	if m.watcher != nil {
		cmd = m.watcher.NextEventCmd()
	}
	if m.store != nil {
		if _, err := m.store.ImportFile(msg.Path); err != nil {
			warn := fmt.Sprintf("Auto-import failed for %s: %v", filepath.Base(msg.Path), err)
			m.home.SetAutoImportWarn(warn)
			m.library.SetAutoImportWarn(warn)
			return cmd
		}
		m.home.SetAutoImportWarn("")
		m.library.SetAutoImportWarn("")
		return tea.Batch(cmd, m.library.Init(), m.home.Init())
	}
	return cmd
}

// handleWatcherStarted records the running watcher and clears warnings.
func (m *AppModel) handleWatcherStarted(msg watcher.WatcherStartedMsg) tea.Cmd {
	if msg.Err != nil {
		warn := fmt.Sprintf("Auto-import disabled: %v", msg.Err)
		m.home.SetAutoImportWarn(warn)
		m.library.SetAutoImportWarn(warn)
		return nil
	}
	m.home.SetAutoImportWarn("")
	m.library.SetAutoImportWarn("")
	m.watcher = msg.Watcher
	return m.watcher.NextEventCmd()
}
