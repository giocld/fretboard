package viewer

import (
	"time"

	"fretboard/internal/model"
	"fretboard/internal/player"
	"fretboard/internal/ui/msgs"
	tea "github.com/charmbracelet/bubbletea"
)

// fetchAudioCmd looks up backing audio in the background.
func fetchAudioCmd(tab *model.Tab, tabPath string, tabID int64, audioDirs []string, allowOnline bool) tea.Cmd {
	return func() tea.Msg {
		if tab == nil {
			return msgs.AudioFetchedMsg{}
		}
		cat, err := player.BuildAudioCatalog(tab, tabPath, audioDirs, allowOnline)
		if err != nil || len(cat.Sources) == 0 {
			return msgs.AudioFetchedMsg{Err: err, Artist: tab.Artist, Title: tab.Title, TabID: tabID, TabPath: tabPath}
		}
		idx := pickAudioSourceIndex(tab, cat)
		src := cat.Sources[idx]
		path := src.Path
		if src.Kind == player.SourceOnline && (path == "" || !player.OnlineAudioAvailable()) {
			var derr error
			path, derr = player.EnsureAudioSource(tab, src)
			if derr != nil {
				return msgs.AudioFetchedMsg{Err: derr, Artist: tab.Artist, Title: tab.Title, TabID: tabID, TabPath: tabPath}
			}
		}
		if src.Kind == player.SourceMIDI {
			return msgs.AudioFetchedMsg{Artist: tab.Artist, Title: tab.Title, TabID: tabID, TabPath: tabPath}
		}
		return msgs.AudioFetchedMsg{Path: path, Artist: tab.Artist, Title: tab.Title, TabID: tabID, TabPath: tabPath}
	}
}

// startPlaybackCmd launches playback for the selected audio source,
// applying the practice-tool settings (metronome, count-in, program).
func startPlaybackCmd(engine *player.Engine, tab *model.Tab, bpm int, tabPath string, audioDirs []string, src player.AudioSource, startIdx int, opts playbackOpts) tea.Cmd {
	return func() tea.Msg {
		if engine.ShutdownRequested() {
			return msgs.PlaybackErrorMsg{Err: errPlaybackStopped}
		}
		schedule := player.BuildSchedule(tab)
		if len(schedule) == 0 {
			return msgs.PlaybackErrorMsg{Err: errNoPlaybackSteps}
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
				return msgs.PlaybackErrorMsg{Err: err}
			}
			src.Path = path
		}
		step := schedule[startIdx]
		dur := time.Duration(player.StepDuration(step.Ticks, bpm)) * time.Millisecond
		if src.Kind == player.SourceMIDI {
			engine.Synth.Metronome = opts.metronome
			engine.Synth.Program = opts.program
			if err := engine.StartMIDIRealtime(); err != nil {
				return msgs.PlaybackErrorMsg{Err: err}
			}
			// Lead-in clicks before the first tab note; blocks for the
			// count-in duration inside this command's goroutine.
			if opts.countIn > 0 {
				engine.Synth.CountIn(opts.countIn, bpm)
			}
			if err := engine.PlayMIDIStep(tab, step, bpm); err != nil {
				_ = engine.Stop()
				return msgs.PlaybackErrorMsg{Err: err}
			}
		} else {
			ctx := player.PlayContext{TabPath: tabPath, AudioDirs: audioDirs, AllowOnline: false}
			if err := engine.PlaySource(tab, bpm, src, ctx); err != nil {
				return msgs.PlaybackErrorMsg{Err: err}
			}
		}
		if engine.ShutdownRequested() {
			_ = engine.Stop()
			return msgs.PlaybackErrorMsg{Err: errPlaybackStopped}
		}
		synced := engine.Mode() == "audio" && engine.AudioDuration() > 0
		if synced {
			dur = 80 * time.Millisecond
		}
		return msgs.PlaybackStartedMsg{
			Schedule:  schedule,
			StepIdx:   startIdx,
			Duration:  dur,
			AudioSync: synced,
		}
	}
}

// playbackOpts carries the practice-tool settings applied at playback start.
type playbackOpts struct {
	metronome bool
	countIn   int
	program   int
}

// playbackOpts snapshots the current practice-tool state.
func (m ViewerModel) playbackOpts() playbackOpts {
	return playbackOpts{metronome: m.metronome, countIn: m.countIn, program: m.program}
}

// tickCmd returns a command that waits for the next playback tick.
func tickCmd(duration time.Duration) tea.Cmd {
	return tea.Tick(duration, func(time.Time) tea.Msg {
		return msgs.PlaybackTickMsg{}
	})
}

// beatStep returns the duration of one 16th-note step at the given BPM.
func beatStep(bpm int) time.Duration {
	if bpm <= 0 {
		bpm = 120
	}
	return time.Duration(60_000/bpm/4) * time.Millisecond
}

// monitorPlaybackCmd polls synth process health while audio may be playing.
func monitorPlaybackCmd(engine *player.Engine) tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(time.Time) tea.Msg {
		return msgs.PlaybackMonitorMsg{}
	})
}

var (
	errNoPlaybackSteps = playerErr("no playable notes in tab")
	errAudioNotReady   = playerErr("audio still downloading — wait or press a to pick a source")
	errPlaybackStopped = playerErr("playback stopped")
)

type playerErr string

func (e playerErr) Error() string { return string(e) }
