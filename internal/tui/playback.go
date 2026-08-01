package tui

import (
	"time"

	"github.com/YOUR_USERNAME/fretboard/internal/model"
	"github.com/YOUR_USERNAME/fretboard/internal/player"
	tea "github.com/charmbracelet/bubbletea"
)

// AudioFetchedMsg is sent when a background audio lookup finishes.
type AudioFetchedMsg struct {
	Path    string
	Err     error
	Artist  string
	Title   string
	TabID   int64
	TabPath string
}

// fetchAudioCmd looks up backing audio in the background.
func fetchAudioCmd(tab *model.Tab, tabPath string, tabID int64, audioDirs []string, allowOnline bool) tea.Cmd {
	return func() tea.Msg {
		if tab == nil {
			return AudioFetchedMsg{}
		}
		cat, err := player.BuildAudioCatalog(tab, tabPath, audioDirs, allowOnline)
		if err != nil || len(cat.Sources) == 0 {
			return AudioFetchedMsg{Err: err, Artist: tab.Artist, Title: tab.Title, TabID: tabID, TabPath: tabPath}
		}
		idx := pickAudioSourceIndex(tab, cat)
		src := cat.Sources[idx]
		path := src.Path
		if src.Kind == player.SourceOnline && (path == "" || !player.OnlineAudioAvailable()) {
			var derr error
			path, derr = player.EnsureAudioSource(tab, src)
			if derr != nil {
				return AudioFetchedMsg{Err: derr, Artist: tab.Artist, Title: tab.Title, TabID: tabID, TabPath: tabPath}
			}
		}
		if src.Kind == player.SourceMIDI {
			return AudioFetchedMsg{Artist: tab.Artist, Title: tab.Title, TabID: tabID, TabPath: tabPath}
		}
		return AudioFetchedMsg{Path: path, Artist: tab.Artist, Title: tab.Title, TabID: tabID, TabPath: tabPath}
	}
}

// PlaybackTickMsg is sent by the playback goroutine on each step.
type PlaybackTickMsg struct {
	Bar      int
	Col      int
	StepIdx  int
	Duration time.Duration
}

// startPlaybackCmd launches playback for the selected audio source.
func startPlaybackCmd(engine *player.Engine, tab *model.Tab, bpm int, tabPath string, audioDirs []string, src player.AudioSource, startIdx int) tea.Cmd {
	return func() tea.Msg {
		if engine.ShutdownRequested() {
			return PlaybackErrorMsg{Err: errPlaybackStopped}
		}
		schedule := player.BuildSchedule(tab)
		if len(schedule) == 0 {
			return PlaybackErrorMsg{Err: errNoPlaybackSteps}
		}
		if startIdx < 0 {
			startIdx = 0
		}
		if startIdx >= len(schedule) {
			startIdx = len(schedule) - 1
		}
		if src.Kind == player.SourceOnline && (src.Path == "" || !player.FileExists(src.Path)) {
			path, err := player.EnsureAudioSource(tab, src)
			if err != nil {
				return PlaybackErrorMsg{Err: err}
			}
			src.Path = path
		}
		step := schedule[startIdx]
		dur := time.Duration(player.StepDuration(step.Ticks, bpm)) * time.Millisecond
		if src.Kind == player.SourceMIDI {
			if err := engine.StartMIDIRealtime(); err != nil {
				return PlaybackErrorMsg{Err: err}
			}
			if err := engine.PlayMIDIStep(tab, step, bpm); err != nil {
				_ = engine.Stop()
				return PlaybackErrorMsg{Err: err}
			}
		} else {
			ctx := player.PlayContext{TabPath: tabPath, AudioDirs: audioDirs, AllowOnline: false}
			if err := engine.PlaySource(tab, bpm, src, ctx); err != nil {
				return PlaybackErrorMsg{Err: err}
			}
		}
		if engine.ShutdownRequested() {
			_ = engine.Stop()
			return PlaybackErrorMsg{Err: errPlaybackStopped}
		}
		synced := engine.Mode() == "audio" && engine.AudioDuration() > 0
		if synced {
			dur = 80 * time.Millisecond
		}
		return PlaybackStartedMsg{
			Schedule:  schedule,
			StepIdx:   startIdx,
			Duration:  dur,
			AudioSync: synced,
		}
	}
}

// tickCmd returns a command that waits for the next playback tick.
func tickCmd(duration time.Duration) tea.Cmd {
	return tea.Tick(duration, func(time.Time) tea.Msg {
		return PlaybackTickMsg{}
	})
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

// beatStep returns the duration of one 16th-note step at the given BPM.
func beatStep(bpm int) time.Duration {
	if bpm <= 0 {
		bpm = 120
	}
	return time.Duration(60_000/bpm/4) * time.Millisecond
}

// PlaybackMonitorMsg checks whether the external synth is still running.
type PlaybackMonitorMsg struct{}

// monitorPlaybackCmd polls synth process health while audio may be playing.
func monitorPlaybackCmd(engine *player.Engine) tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(time.Time) tea.Msg {
		return PlaybackMonitorMsg{}
	})
}

var (
	errNoPlaybackSteps = playerErr("no playable notes in tab")
	errAudioNotReady   = playerErr("audio still downloading — wait or press a to pick a source")
	errPlaybackStopped = playerErr("playback stopped")
)

type playerErr string

func (e playerErr) Error() string { return string(e) }
