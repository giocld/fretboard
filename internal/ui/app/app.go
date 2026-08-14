// Package app implements the top-level Bubble Tea router that switches
// between the home, library browser, tab viewer, online search, and help
// screens, and owns the cross-screen state: the library store, the
// auto-import watcher, and the active theme.
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

type viewType int

const (
	viewHome viewType = iota
	viewLibrary
	viewViewer
	viewSearch
	viewHelp
	viewSettings
)

// AppModel is the top-level Bubble Tea model that routes between the landing
// page, library browser, tab viewer, online search view, and help screen.
type AppModel struct {
	view           viewType
	prev           viewType
	store          *library.Store
	home           home.HomeModel
	library        browser.BrowserModel
	viewer         viewer.ViewerModel
	search         search.SearchModel
	help           help.HelpModel
	settings       settings.SettingsModel
	watcher        *watcher.Watcher
	autoImportPath string
	startupCmd     tea.Cmd
}

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

// Init returns the initial command.
func (m AppModel) Init() tea.Cmd {
	var cmds []tea.Cmd
	cmds = append(cmds, m.home.Init())
	if m.store != nil {
		cmds = append(cmds, m.library.Init())
	}
	if m.search.HasClient() {
		cmds = append(cmds, m.search.Init())
	}
	if m.autoImportPath != "" && m.watcher == nil {
		cmds = append(cmds, watcher.StartCmd(m.autoImportPath))
	}
	if m.startupCmd != nil {
		cmds = append(cmds, m.startupCmd)
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

// Update handles top-level routing and global keys.
func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case msgs.ShutdownMsg:
		m.Shutdown()
		return m, tea.Quit

	case tea.WindowSizeMsg:
		m.home, _ = m.home.Update(msg)
		m.library, _ = m.library.Update(msg)
		m.viewer, _ = m.viewer.Update(msg)
		m.search, _ = m.search.Update(msg)
		m.help, _ = m.help.Update(msg)
		m.settings, _ = m.settings.Update(msg)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case kit.KeyQuit2:
			m.Shutdown()
			return m, tea.Quit
		case kit.KeyQuit:
			if m.textInputActive() {
				break
			}
			m.Shutdown()
			return m, tea.Quit
		case "t":
			if m.textInputActive() {
				break
			}
			m.cycleTheme()
			return m, nil
		case "?":
			if m.textInputActive() {
				break
			}
			if m.view == viewHelp {
				m.view = m.prev
				return m, nil
			}
			m.stopPlayback()
			m.prev = m.view
			m.view = viewHelp
			return m, nil
		}

	case msgs.TabSelectedMsg:
		return m, m.openTab(msg.ID)

	case msgs.HomeLibraryMsg:
		m.prev = m.view
		m.view = viewLibrary
		return m, m.library.Init()

	case msgs.HomeSearchMsg:
		m.prev = m.view
		m.search.Reset()
		m.view = viewSearch
		return m, m.search.Init()

	case msgs.HomeSettingsMsg:
		m.prev = m.view
		m.settings = settings.NewSettingsModel()
		m.view = viewSettings
		return m, nil

	case msgs.GoHomeMsg:
		m.stopPlayback()
		m.library.ResetFilter()
		m.view = viewHome
		return m, m.home.Init()

	case msgs.SearchBackMsg:
		m.search.Reset()
		m.view = m.prev
		if m.view == viewHome {
			return m, m.home.Init()
		}
		if m.view == viewLibrary {
			return m, m.library.Init()
		}
		return m, nil

	case msgs.TabImportErrorMsg:
		if !m.search.AcceptsGen(msg.Gen) {
			return m, nil
		}
		m.search.SetBusy(false, false)
		newSearch, cmd := m.search.Update(msg)
		m.search = newSearch
		return m, cmd

	case msgs.TabFetchedMsg:
		if !m.search.AcceptsGen(msg.Gen) {
			return m, nil
		}
		m.search.SetBusy(false, false)
		m.stopPlayback()
		if msg.Tab == nil {
			m.viewer.LoadTab(nil, "", 0)
			m.viewer.SetError("Could not load tab")
			m.view = viewViewer
			return m, nil
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
				return m, m.viewer.BeginAudioFetch(cfg.AutoFetchAudio)
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
			return m, tea.Batch(cmds...)
		}
		return m, nil

	case msgs.ViewLibraryMsg:
		m.stopPlayback()
		m.view = viewLibrary
		return m, m.library.Init()

	case msgs.ViewHomeMsg:
		m.stopPlayback()
		m.library.ResetFilter()
		m.view = viewHome
		return m, m.home.Init()

	case msgs.TabPrefsSaveMsg:
		if m.store != nil && m.viewer.Tab() != nil {
			if path := m.tabFilePath(); path != "" {
				if _, err := m.store.Import(path, m.viewer.Tab()); err != nil {
					m.viewer.SetError("Could not save tab preferences: " + err.Error())
				}
			}
		}
		return m, nil

	case msgs.OpenSettingsMsg:
		m.stopPlayback()
		m.prev = m.view
		m.settings = settings.NewSettingsModel()
		m.view = viewSettings
		return m, nil

	case msgs.SettingsBackMsg:
		// Apply the changed settings live and persist them.
		cfg := m.settings.Config()
		_ = config.Save(cfg)
		kit.SetTheme(cfg.ThemeName)
		m.viewer.SetVolume(cfg.VolumePercent)
		m.viewer.SetStrictAudio(cfg.StrictAudioSelection)
		m.view = m.prev
		if m.view == viewHome {
			return m, m.home.Init()
		}
		if m.view == viewLibrary {
			return m, m.library.Init()
		}
		return m, nil

	case msgs.CloseHelpMsg:
		m.view = m.prev
		return m, nil

	case watcher.FileAddedMsg:
		var cmd tea.Cmd
		if m.watcher != nil {
			cmd = m.watcher.NextEventCmd()
		}
		if m.store != nil {
			if _, err := m.store.ImportFile(msg.Path); err != nil {
				warn := fmt.Sprintf("Auto-import failed for %s: %v", filepath.Base(msg.Path), err)
				m.home.SetAutoImportWarn(warn)
				m.library.SetAutoImportWarn(warn)
				return m, cmd
			}
			m.home.SetAutoImportWarn("")
			m.library.SetAutoImportWarn("")
			return m, tea.Batch(cmd, m.library.Init(), m.home.Init())
		}
		return m, cmd

	case watcher.WatcherStartedMsg:
		if msg.Err != nil {
			warn := fmt.Sprintf("Auto-import disabled: %v", msg.Err)
			m.home.SetAutoImportWarn(warn)
			m.library.SetAutoImportWarn(warn)
			return m, nil
		}
		m.home.SetAutoImportWarn("")
		m.library.SetAutoImportWarn("")
		m.watcher = msg.Watcher
		return m, m.watcher.NextEventCmd()
	}

	switch m.view {
	case viewHome:
		newHome, cmd := m.home.Update(msg)
		m.home = newHome
		return m, cmd
	case viewLibrary:
		newLib, cmd := m.library.Update(msg)
		m.library = newLib
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "o":
				m.prev = m.view
				m.search.Reset()
				m.view = viewSearch
				return m, tea.Batch(cmd, m.search.Init())
			case "S":
				m.prev = m.view
				m.settings = settings.NewSettingsModel()
				m.view = viewSettings
				return m, nil
			}
		}
		return m, cmd
	case viewSearch:
		newSearch, cmd := m.search.Update(msg)
		m.search = newSearch
		return m, cmd
	case viewSettings:
		newSettings, cmd := m.settings.Update(msg)
		m.settings = newSettings
		return m, cmd
	case viewViewer:
		newV, cmd := m.viewer.Update(msg)
		m.viewer = newV
		return m, cmd
	case viewHelp:
		newHelp, cmd := m.help.Update(msg)
		m.help = newHelp
		return m, cmd
	}
	return m, nil
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

// View returns the current view.
func (m AppModel) View() string {
	switch m.view {
	case viewHome:
		return m.home.View()
	case viewLibrary:
		return m.library.View()
	case viewSearch:
		return m.search.View()
	case viewSettings:
		return m.settings.View()
	case viewViewer:
		return m.viewer.View()
	case viewHelp:
		return m.help.View()
	}
	return "loading..."
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
