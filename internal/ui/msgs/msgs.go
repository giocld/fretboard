// Package msgs defines the messages flowing between screens and the app
// router. It is a pure data-contract package: no behavior lives here.
package msgs

import (
	"time"

	"fretboard/internal/library"
	"fretboard/internal/model"
	"fretboard/internal/player"
	"fretboard/internal/scraper"
)

// HomeLibraryMsg navigates to the library browser.
type HomeLibraryMsg struct{}

// HomeSearchMsg navigates to online search.
type HomeSearchMsg struct{}

// SearchBackMsg is sent when the user leaves online search.
type SearchBackMsg struct{}

// GoHomeMsg navigates back to the landing page.
type GoHomeMsg struct{}

// ViewLibraryMsg is sent by screens that want to return to the library.
type ViewLibraryMsg struct{}

// ViewHomeMsg is sent by screens that want to return home.
type ViewHomeMsg struct{}

// CloseHelpMsg is sent when the user closes the help screen.
type CloseHelpMsg struct{}

// TabPrefsSaveMsg asks the router to persist viewer metadata changes.
type TabPrefsSaveMsg struct{}

// TabSelectedMsg is sent when the user opens a tab.
type TabSelectedMsg struct {
	ID int64
}

// ShutdownMsg requests a clean shutdown (audio + watcher) followed by quit.
// It is delivered by the signal handler in the CLI entrypoint so external
// SIGINT/SIGTERM run cleanup against the live model, not a stale copy.
type ShutdownMsg struct{}

// TabsLoadedMsg is sent when the library list has been loaded.
type TabsLoadedMsg struct {
	Tabs []library.TabRow
}

// TabsLoadErrorMsg is sent when the library list fails to load.
type TabsLoadErrorMsg struct {
	Err error
}

// BrowserPreviewMsg delivers a rendered preview of a library tab to the
// browser's right-side preview panel. Gen guards against stale loads.
type BrowserPreviewMsg struct {
	TabID   int64
	Title   string
	Preview string
	Err     error
	Gen     int
}

// AutoImportWarnMsg surfaces watcher startup failures.
type AutoImportWarnMsg struct {
	Msg string
}

// SearchPerformedMsg is sent when a search completes.
type SearchPerformedMsg struct {
	Results []scraper.SearchResult
	Err     error
	Gen     int
	More    bool // true when this is a load-more page, merged into the list
}

// TabFetchedMsg is sent when an online tab has been fetched and parsed.
type TabFetchedMsg struct {
	Tab    *model.Tab
	Source scraper.SearchResult
	Gen    int
}

// TabImportErrorMsg is sent when fetching an online tab fails.
type TabImportErrorMsg struct {
	Err error
	Gen int
}

// PlaybackTickMsg is sent by the playback goroutine on each step.
type PlaybackTickMsg struct {
	Bar      int
	Col      int
	StepIdx  int
	Duration time.Duration
}

// PlaybackStartedMsg is sent when audio playback has begun.
type PlaybackStartedMsg struct {
	Schedule  []player.PlaybackStep
	StepIdx   int
	Duration  time.Duration
	AudioSync bool
}

// PlaybackErrorMsg is sent when audio playback fails to start.
type PlaybackErrorMsg struct {
	Err error
}

// PlaybackMonitorMsg checks whether the external synth is still running.
type PlaybackMonitorMsg struct{}

// AudioFetchedMsg is sent when a background audio lookup finishes.
type AudioFetchedMsg struct {
	Path    string
	Err     error
	Artist  string
	Title   string
	TabID   int64
	TabPath string
}

// AudioCatalogMsg delivers ranked audio options for the current tab.
type AudioCatalogMsg struct {
	Catalog player.AudioCatalog
	Err     error
	Artist  string
	Title   string
	TabID   int64
	TabPath string
}

// IntroDetectedMsg delivers an auto-detected leading-silence intro offset
// for the selected audio source. The viewer applies it only if the source is
// still selected and no manual calibration exists.
type IntroDetectedMsg struct {
	SourceID string
	Offset   time.Duration
	Err      error
	Artist   string
	Title    string
	TabID    int64
	TabPath  string
}

// SettingsBackMsg closes the settings screen and returns to the previous
// view.
type SettingsBackMsg struct{}

// OpenSettingsMsg opens the settings screen.
type OpenSettingsMsg struct{}

// HomeSettingsMsg opens the settings screen from the home page.
type HomeSettingsMsg struct{}

// AlignmentMsg delivers an automatic audio-alignment result for a source.
type AlignmentMsg struct {
	SourceID       string
	BPM            int
	Offset         time.Duration
	Confidence     float64
	Artist         string
	Title          string
	TabID          int64
	TabPath        string
	Onsets         []time.Duration    // detected onsets, for the live drift meter
	OnsetStrengths []float64          // normalized onset strengths, aligned with Onsets
	Anchors        []player.SyncPoint // measured bar anchors (auto tempo map)
}
