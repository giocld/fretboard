package player

import (
	"errors"
	"os/exec"
	"time"

	"fretboard/internal/model"
)

// PlayContext carries per-tab playback hints.
type PlayContext struct {
	TabPath     string
	AudioDirs   []string
	AllowOnline bool
}

// Engine plays backing audio when available and falls back to MIDI synth.
type Engine struct {
	*Synth
	audioCmd      *exec.Cmd
	mode          string
	audioPath     string
	playbackStart time.Time
	audioDuration time.Duration
	shutdown      bool
}

// NewEngine creates a playback engine with MIDI synth fallback.
func NewEngine() *Engine {
	return &Engine{Synth: NewSynth()}
}

// Mode reports the active backend: "audio", "midi", or "".
func (e *Engine) Mode() string {
	return e.mode
}

// AudioPath returns the backing track path when playing real audio.
func (e *Engine) AudioPath() string {
	return e.audioPath
}

// AudioDuration returns the length of the active backing track.
func (e *Engine) AudioDuration() time.Duration {
	return e.audioDuration
}

// Elapsed returns time since backing audio started.
func (e *Engine) Elapsed() time.Duration {
	if e.mode != "audio" || e.playbackStart.IsZero() {
		return 0
	}
	return time.Since(e.playbackStart)
}

// Shutdown stops playback and blocks new starts until the app exits.
func (e *Engine) Shutdown() {
	e.shutdown = true
	_ = e.Stop()
}

// ShutdownRequested reports whether the engine has been shut down.
func (e *Engine) ShutdownRequested() bool {
	return e.shutdown
}

func (e *Engine) checkShutdown() error {
	if e.shutdown {
		return errors.New("playback shut down")
	}
	return nil
}

// Play tries a backing-track file first, then online audio, then MIDI synth.
func (e *Engine) Play(tab *model.Tab, bpm int, ctx PlayContext) error {
	if err := e.checkShutdown(); err != nil {
		return err
	}
	if bpm <= 0 {
		bpm = TabBPM(tab)
	}
	path, err := ResolveAudio(tab, ctx.TabPath, ctx.AudioDirs, ctx.AllowOnline)
	if err == nil && path != "" {
		return e.playAudioFile(path)
	}
	return e.playMIDI(tab, bpm)
}

// PlaySource plays a specific catalog entry.
func (e *Engine) PlaySource(tab *model.Tab, bpm int, src AudioSource, ctx PlayContext) error {
	if err := e.checkShutdown(); err != nil {
		return err
	}
	if bpm <= 0 {
		bpm = TabBPM(tab)
	}
	switch src.Kind {
	case SourceMIDI:
		return e.playMIDI(tab, bpm)
	case SourceLocal:
		if src.Path == "" || !fileExists(src.Path) {
			return e.playMIDI(tab, bpm)
		}
		return e.playAudioFile(src.Path)
	case SourceOnline:
		path := src.Path
		if path == "" || !fileExists(path) {
			var err error
			path, err = EnsureAudioSource(tab, src)
			if err != nil {
				return err
			}
		}
		return e.playAudioFile(path)
	default:
		return e.Play(tab, bpm, ctx)
	}
}

func (e *Engine) StartMIDIRealtime() error {
	e.audioDuration = 0
	e.playbackStart = time.Time{}
	e.mode = "midi"
	e.audioPath = ""
	return e.Synth.StartRealtime()
}

func (e *Engine) PlayMIDIStep(tab *model.Tab, step PlaybackStep, bpm int) error {
	return e.Synth.PlayStep(tab, step, bpm)
}
func (e *Engine) playMIDI(tab *model.Tab, bpm int) error {
	e.audioDuration = 0
	e.playbackStart = time.Time{}
	e.mode = "midi"
	e.audioPath = ""
	if err := e.Synth.Play(tab, bpm); err != nil {
		e.mode = ""
		return err
	}
	return nil
}

func (e *Engine) playAudioFile(path string) error {
	dur, err := ProbeDuration(path)
	if err != nil {
		dur = 0
	}
	if err := e.playAudio(path); err != nil {
		return err
	}
	e.audioDuration = dur
	e.playbackStart = time.Now()
	return nil
}

// Stop halts audio or MIDI playback.
func (e *Engine) Stop() error {
	e.stopAudio()
	e.mode = ""
	e.audioPath = ""
	e.playbackStart = time.Time{}
	e.audioDuration = 0
	return e.Synth.Stop()
}

// Running reports whether playback is active.
func (e *Engine) Running() bool {
	if e.mode == "audio" {
		return e.audioRunning()
	}
	return e.Synth.Running()
}

// PlaybackEnded reports whether external playback has finished.
func (e *Engine) PlaybackEnded() bool {
	return !e.Running()
}
