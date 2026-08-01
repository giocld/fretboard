package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/YOUR_USERNAME/fretboard/internal/config"
	"github.com/YOUR_USERNAME/fretboard/internal/library"
	"github.com/YOUR_USERNAME/fretboard/internal/model"
	"github.com/YOUR_USERNAME/fretboard/internal/scraper"
	"github.com/YOUR_USERNAME/fretboard/internal/watcher"
	tea "github.com/charmbracelet/bubbletea"
)

// ShutdownMsg requests a clean shutdown (audio + watcher) followed by quit.
// It is delivered by the signal handler in main so external SIGINT/SIGTERM
// run cleanup against the live model, not a stale copy.
type ShutdownMsg struct{}

type viewType int

const (
	viewHome viewType = iota
	viewLibrary
	viewViewer
	viewSearch
	viewHelp
)

// AppModel is the top-level Bubble Tea model that routes between the landing
// page, library browser, tab viewer, online search view, and help screen.
type AppModel struct {
	view           viewType
	prev           viewType
	home           HomeModel
	library        BrowserModel
	viewer         ViewerModel
	search         SearchModel
	help           HelpModel
	watcher        *watcher.Watcher
	autoImportPath   string
	autoImportWarn   string
	audioSearchPaths []string
	width          int
	height         int
	startupCmd     tea.Cmd
}

// NewApp creates a new TUI app for e2e tests and external callers that don't
// need a real library store or online search.
func NewApp() AppModel {
	return AppModel{
		view:    viewHome,
		home:    NewHomeModel(nil),
		library: NewBrowserModel(nil),
		viewer:  NewViewerModel(),
		search:  NewSearchModel(nil),
		help:    NewHelpModel(),
		width:   80,
		height:  24,
	}
}

// NewAppWithStore creates a new TUI app bound to a library store, optional
// online scraper, and optional auto-import directory.
func NewAppWithStore(store *library.Store, scraper *scraper.Client, autoImportPath string) AppModel {
	return NewAppWithOptions(store, scraper, autoImportPath, nil)
}

func NewAppWithOptions(store *library.Store, client *scraper.Client, autoImportPath string, audioSearchPaths []string) AppModel {
	viewer := NewViewerModel()
	viewer.SetAudioDirs(audioSearchPaths)
	return AppModel{
		view:             viewHome,
		home:             NewHomeModel(store),
		library:          NewBrowserModel(store),
		viewer:           viewer,
		search:           NewSearchModel(client),
		help:             NewHelpModel(),
		autoImportPath:   autoImportPath,
		audioSearchPaths: append([]string(nil), audioSearchPaths...),
		width:            80,
		height:           24,
	}
}

// Init returns the initial command.
func (m AppModel) Init() tea.Cmd {
	var cmds []tea.Cmd
	cmds = append(cmds, m.home.Init())
	if m.library.store != nil {
		cmds = append(cmds, m.library.Init())
	}
	if m.search.client != nil {
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
	case ShutdownMsg:
		m.Shutdown()
		return m, tea.Quit

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.home, _ = m.home.Update(msg)
		m.library, _ = m.library.Update(msg)
		m.viewer, _ = m.viewer.Update(msg)
		m.search, _ = m.search.Update(msg)
		m.help, _ = m.help.Update(msg)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case KeyQuit2:
			m.Shutdown()
			return m, tea.Quit
		case KeyQuit:
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

	case TabSelectedMsg:
		return m, m.openTab(msg.ID)

	case HomeLibraryMsg:
		m.prev = m.view
		m.view = viewLibrary
		return m, m.library.Init()

	case HomeSearchMsg:
		m.prev = m.view
		m.search.Reset()
		m.view = viewSearch
		return m, m.search.Init()

	case GoHomeMsg:
		m.stopPlayback()
		m.library.resetFilter()
		m.view = viewHome
		return m, m.home.Init()

	case SearchBackMsg:
		m.search.Reset()
		m.view = m.prev
		if m.view == viewHome {
			return m, m.home.Init()
		}
		if m.view == viewLibrary {
			return m, m.library.Init()
		}
		return m, nil

	case TabImportErrorMsg:
		if msg.Gen != m.search.reqGen {
			return m, nil
		}
		m.search.loading = false
		m.search.importing = false
		newSearch, cmd := m.search.Update(msg)
		m.search = newSearch
		return m, cmd

	case TabFetchedMsg:
		if msg.Gen != m.search.reqGen {
			return m, nil
		}
		m.search.loading = false
		m.search.importing = false
		m.stopPlayback()
		if msg.Tab == nil {
			m.viewer.LoadTab(nil, "", 0)
			m.viewer.errMsg = "Could not load tab"
			m.view = viewViewer
			return m, nil
		}
		var cmds []tea.Cmd
		tabPath := OnlineTabPath(msg.Source)
		tabID := int64(0)
		if m.library.store != nil {
			id, err := m.library.store.Import(tabPath, msg.Tab)
			if err != nil {
				m.viewer.LoadTab(msg.Tab, tabPath, 0)
				m.viewer.errMsg = fmt.Sprintf("Tab opened but not saved: %v", err)
				m.view = viewViewer
				cfg, _ := config.Load()
				return m, m.viewer.BeginAudioFetch(cfg.AutoFetchAudio)
			}
			tabID = id
			_ = m.library.store.RecordPlay(id)
			if row, rowErr := m.library.store.GetRow(id); rowErr == nil && row != nil {
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

	case viewLibraryMsg:
		m.stopPlayback()
		m.view = viewLibrary
		return m, m.library.Init()

	case viewHomeMsg:
		m.stopPlayback()
		m.library.resetFilter()
		m.view = viewHome
		return m, m.home.Init()

	case TabPrefsSaveMsg:
		if m.library.store != nil && m.viewer.tab != nil {
			path := strings.TrimSpace(m.viewer.tabPath)
			if path == "" && m.viewer.tabID > 0 {
				if row, err := m.library.store.GetRow(m.viewer.tabID); err == nil && row != nil {
					path = row.Filepath
				}
			}
			if path != "" {
				if _, err := m.library.store.Import(path, m.viewer.tab); err != nil {
					m.viewer.errMsg = "Could not save tab preferences: " + err.Error()
				}
			}
		}
		return m, nil

	case CloseHelpMsg:
		m.view = m.prev
		return m, nil

	case watcher.FileAddedMsg:
		var cmd tea.Cmd
		if m.watcher != nil {
			cmd = m.watcher.NextEventCmd()
		}
		if m.library.store != nil {
			if _, err := m.library.store.ImportFile(msg.Path); err != nil {
				warn := fmt.Sprintf("Auto-import failed for %s: %v", filepath.Base(msg.Path), err)
				m.autoImportWarn = warn
				m.home.autoImportWarn = warn
				m.library.autoImportWarn = warn
				return m, cmd
			}
			m.autoImportWarn = ""
			m.home.autoImportWarn = ""
			m.library.autoImportWarn = ""
			return m, tea.Batch(cmd, m.library.Init(), m.home.Init())
		}
		return m, cmd

	case watcher.WatcherStartedMsg:
		if msg.Err != nil {
			warn := fmt.Sprintf("Auto-import disabled: %v", msg.Err)
			m.autoImportWarn = warn
			m.home.autoImportWarn = warn
			m.library.autoImportWarn = warn
			return m, nil
		}
		m.autoImportWarn = ""
		m.home.autoImportWarn = ""
		m.library.autoImportWarn = ""
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
		if key, ok := msg.(tea.KeyMsg); ok && key.String() == "o" {
			m.prev = m.view
			m.search.Reset()
			m.view = viewSearch
			return m, tea.Batch(cmd, m.search.Init())
		}
		return m, cmd
	case viewSearch:
		newSearch, cmd := m.search.Update(msg)
		m.search = newSearch
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

// Shutdown stops audio and releases background resources.
func (m *AppModel) Shutdown() {
	m.viewer.stopPlayback()
	m.viewer.engine.Shutdown()
	if m.watcher != nil {
		m.watcher.Close()
	}
}

func (m AppModel) textInputActive() bool {
	if m.view == viewSearch && m.search.inputActive {
		return true
	}
	if m.view == viewLibrary && m.library.searchActive {
		return true
	}
	return false
}

func (m *AppModel) stopPlayback() {
	m.viewer.stopPlayback()
}

func (m *AppModel) openTab(id int64) tea.Cmd {
	if m.library.store == nil {
		m.viewer.errMsg = "Library is not available"
		m.view = viewViewer
		return nil
	}
	tab, err := m.library.store.Get(id)
	if err != nil || tab == nil {
		m.viewer.LoadTab(nil, "", 0)
		if err != nil {
			m.viewer.errMsg = fmt.Sprintf("Could not open tab: %v", err)
		} else {
			m.viewer.errMsg = "Could not open tab"
		}
		m.view = viewViewer
		return nil
	}
	path := ""
	if row, err := m.library.store.GetRow(id); err == nil && row != nil {
		path = row.Filepath
	}
	_ = m.library.store.RecordPlay(id)
	m.stopPlayback()
	m.viewer.LoadTab(tab, path, id)
	m.view = viewViewer
	cfg, _ := config.Load()
	return tea.Batch(m.viewer.BeginAudioFetch(cfg.AutoFetchAudio), m.library.Init(), m.home.Init())
}


func (m *AppModel) cycleTheme() {
	names := ThemeNames()
	idx := 0
	for i, n := range names {
		if n == CurrentTheme().Name {
			idx = (i + 1) % len(names)
			break
		}
	}
	SetTheme(names[idx])
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
	if v < 0 {
		v = 0
	}
	if v > 100 {
		v = 100
	}
	m.viewer.engine.Volume = v
}

// SetSoundfont sets the path to the soundfont used by the synthesizer.
func (m *AppModel) SetSoundfont(path string) {
	m.viewer.engine.Soundfont = path
}

// _ prevents unused import.
var _ = model.Standard
