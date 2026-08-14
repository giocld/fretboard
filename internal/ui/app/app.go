// Package app implements the top-level Bubble Tea router that switches
// between the home, library browser, tab viewer, online search, and help
// screens, and owns the cross-screen state: the library store, the
// auto-import watcher, and the active theme.
package app

import (
	"fretboard/internal/config"
	"fretboard/internal/library"
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
		return m, m.handleTabImportError(msg)

	case msgs.TabFetchedMsg:
		return m, m.handleTabFetched(msg)

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
		return m, m.handleFileAdded(msg)

	case watcher.WatcherStartedMsg:
		return m, m.handleWatcherStarted(msg)
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
