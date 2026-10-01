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
		if src.Kind == player.SourceMIDI {
			engine.Synth.Metronome = opts.metronome
			engine.Synth.Program = opts.program
			// The whole timeline is baked into an SMF that fluidsynth's
			// built-in player performs sample-accurately — note timing no
			// longer depends on TUI tick processing. The cursor tracks
			// Elapsed() in the monitor loop instead of driving notes.
			if err := engine.PlayMIDIFile(tab, bpm, player.MIDIFileOpts{
				Metronome:   opts.metronome,
				CountInBars: opts.countIn,
				Program:     opts.program,
				StartAt:     player.ScheduleTimeAtStep(schedule, startIdx, bpm),
			}); err != nil {
				_ = engine.Stop()
				return msgs.PlaybackErrorMsg{Err: err}
			}
			// AudioSync mode: Duration carries the session total (including
			// count-in) so the monitor can detect the natural end — the
			// player process outlives the file.
			return msgs.PlaybackStartedMsg{
				Schedule:  schedule,
				StepIdx:   startIdx,
				Duration:  player.ScheduleSpan(schedule, bpm) + countInDuration(opts.countIn, bpm),
				AudioSync: true,
				Started:   time.Now(),
			}
		}
		ctx := player.PlayContext{TabPath: tabPath, AudioDirs: audioDirs, AllowOnline: false}
		if err := engine.PlaySource(tab, bpm, src, ctx); err != nil {
			return msgs.PlaybackErrorMsg{Err: err}
		}
		// Resume mid-song: PlaySource starts at the file's position 0,
		// so seek to the cursor's mapped audio position before the
		// monitor compares Elapsed() against it. MIDI fallbacks and
		// first-ever plays (resume == 0) ignore the seek.
		if opts.resume > 0 && engine.Mode() == "audio" {
			if err := engine.RestartAt(opts.resume); err != nil {
				return msgs.PlaybackErrorMsg{Err: err}
			}
		}
		if engine.ShutdownRequested() {
			_ = engine.Stop()
			return msgs.PlaybackErrorMsg{Err: errPlaybackStopped}
		}
		synced := syncedFor(engine.Mode())
		dur := 80 * time.Millisecond
		if !synced {
			dur = stepDur(schedule[startIdx].Ticks, bpm)
		}
		return msgs.PlaybackStartedMsg{
			Schedule:  schedule,
			StepIdx:   startIdx,
			Duration:  dur,
			AudioSync: synced,
		}
	}
}

// countInDuration returns the wall duration of a count-in lead-in.
func countInDuration(bars, bpm int) time.Duration {
	if bars <= 0 {
		return 0
	}
	return time.Duration(bars*4*60) * time.Second / time.Duration(bpm)
}

// syncedFor reports whether the engine mode drives the playhead from the
// actual audio (Elapsed()) instead of the tab deadline clock. Audio and
// SMF-player MIDI both qualify: the duration may be unknown at start (no
// ffprobe, duration not yet reported) and must not fall back to the
// deadline clock.
func syncedFor(mode string) bool { return mode == "audio" || mode == "midi" }

// playbackOpts carries the practice-tool settings applied at playback start.
type playbackOpts struct {
	metronome bool
	countIn   int
	program   int
	resume    time.Duration // audio position to seek on start (resume after a pause)
}

func (m ViewerModel) playbackOpts() playbackOpts {
	return playbackOpts{metronome: m.metronome, countIn: m.countIn, program: m.program}
}

func tickCmd(gen uint64, duration time.Duration) tea.Cmd {
	return tea.Tick(duration, func(time.Time) tea.Msg {
		return msgs.PlaybackTickMsg{Gen: gen}
	})
}

// One monitor timer is armed per session; MIDI ticks never add more monitors.
// Audio samples often enough to show sixteenths (125ms at 120 BPM).
func monitorPlaybackCmd(gen uint64, audio bool) tea.Cmd {
	interval := 250 * time.Millisecond
	if audio {
		interval = 30 * time.Millisecond
	}
	return tea.Tick(interval, func(time.Time) tea.Msg {
		return msgs.PlaybackMonitorMsg{Gen: gen}
	})
}

var (
	errNoPlaybackSteps = playerErr("no playable notes in tab")
	errPlaybackStopped = playerErr("playback stopped")
)

type playerErr string

func (e playerErr) Error() string { return string(e) }
