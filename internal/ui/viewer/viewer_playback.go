package viewer

import (
	"time"

	"fretboard/internal/model"
	"fretboard/internal/player"
	"fretboard/internal/ui/msgs"
	tea "github.com/charmbracelet/bubbletea"
)

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
		startIdx = min(max(startIdx, 0), len(schedule)-1)
		if src.Kind == player.SourceOnline && (src.Path == "" || !player.FileExists(src.Path)) {
			path, err := player.EnsureAudioSource(tab, src)
			if err != nil {
				return msgs.PlaybackErrorMsg{Err: err}
			}
			src.Path = path
		}
		step := schedule[startIdx]
		dur := stepDur(step.Ticks, bpm)
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
		synced := syncedFor(engine.Mode())
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

// syncedFor reports whether the engine mode drives the playhead from the
// actual audio (Elapsed()) instead of the tab deadline clock. Audio mode
// alone decides: the duration may be unknown at start (no ffprobe, duration
// not yet reported) and must not fall back to the deadline clock.
func syncedFor(mode string) bool { return mode == "audio" }

// playbackOpts carries the practice-tool settings applied at playback start.
type playbackOpts struct {
	metronome bool
	countIn   int
	program   int
}

func (m ViewerModel) playbackOpts() playbackOpts {
	return playbackOpts{metronome: m.metronome, countIn: m.countIn, program: m.program}
}

func tickCmd(duration time.Duration) tea.Cmd {
	return tea.Tick(duration, func(time.Time) tea.Msg {
		return msgs.PlaybackTickMsg{}
	})
}

// monitorPlaybackCmd polls synth process health while audio may be playing.
func monitorPlaybackCmd(engine *player.Engine) tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(time.Time) tea.Msg {
		return msgs.PlaybackMonitorMsg{}
	})
}

var (
	errNoPlaybackSteps = playerErr("no playable notes in tab")
	errPlaybackStopped = playerErr("playback stopped")
)

type playerErr string

func (e playerErr) Error() string { return string(e) }
