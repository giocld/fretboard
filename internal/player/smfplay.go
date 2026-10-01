package player

import (
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sort"
	"time"

	"fretboard/internal/model"
)

// SMF-player playback: the whole timeline is baked into a Standard MIDI File
// and handed to fluidsynth's built-in player, which schedules notes against
// its own audio output — sample-accurate, independent of TUI tick timing.
// The engine tracks the wall-clock position for the cursor and issues live
// player commands (seek, tempo) over the synth's shell.

// MIDIFileOpts configures SMF playback.
type MIDIFileOpts struct {
	Metronome   bool          // click on every beat (muted live when false)
	CountInBars int           // 0-2 bars of lead-in clicks
	Program     int           // GM program for the music channel; 0 → 25 (steel guitar)
	StartAt     time.Duration // music position to start at (resume/jump)
}

const (
	// Clicks ride dedicated channels so muting the metronome never touches
	// music (and count-in stays audible with the metronome off).
	clickChannel   = 1  // metronome: muted/unmuted live via CC7
	countInChannel = 2  // count-in: always audible
	clickProgram   = 13 // GM Xylophone: short wooden attack, reads as a tick
	clickNoteBeat = 79
	clickNoteBar  = 84
)

// midiTicksAt converts a music position to SMF ticks at the given BPM.
func midiTicksAt(d time.Duration, bpm int) int64 {
	return int64(float64(d.Microseconds()) * float64(bpm) / 125_000.0)
}

// midiTicksDuration converts SMF ticks to a music position at the given BPM.
func midiTicksDuration(ticks int64, bpm int) time.Duration {
	return time.Duration(ticks) * 125_000 * time.Microsecond / time.Duration(bpm)
}

// clickEvent returns a xylophone click on the given channel, released 50 ms
// (at 120 BPM) later — melodic channels hold a note until noteoff.
func clickEvent(ch int, accent bool, tick int64) []Event {
	note, vel := clickNoteBeat, 85
	if accent {
		note, vel = clickNoteBar, 115
	}
	return []Event{
		{Type: NoteOn, Tick: tick, Note: note, Vel: vel, Ch: ch},
		{Type: NoteOff, Tick: tick + ticksPerQuarter/10, Note: note, Ch: ch},
	}
}

// buildSMFEvents assembles the full SMF timeline for a tab: note events in
// performance order (drum tabs routed to channel 9), program changes, baked
// count-in clicks, and metronome clicks aligned to the schedule's beats.
// All music events are shifted past the count-in so tick 0 is the first
// count-in click.
func buildSMFEvents(tab *model.Tab, bpm int, opts MIDIFileOpts) (evts []Event, countInTicks int64, countInDur time.Duration, err error) {
	notes, err := Events(tab, bpm)
	if err != nil {
		return nil, 0, 0, err
	}
	prog := opts.Program
	if prog <= 0 {
		prog = 25
	}
	// Program changes first at tick 0 so same-tick notes pick them up.
	evts = append(evts,
		Event{Type: ProgramChange, Ch: 0, Note: prog},
		Event{Type: ProgramChange, Ch: clickChannel, Note: clickProgram},
		Event{Type: ProgramChange, Ch: countInChannel, Note: clickProgram},
	)
	if opts.CountInBars > 0 {
		countInTicks = int64(opts.CountInBars) * 4 * ticksPerQuarter
		countInDur = time.Duration(opts.CountInBars*4*60) * time.Second / time.Duration(bpm)
		for i := 0; i < opts.CountInBars*4; i++ {
			evts = append(evts, clickEvent(countInChannel, i%4 == 0, int64(i)*ticksPerQuarter)...)
		}
	}
	if opts.Metronome {
		// Click on every quarter-note boundary of every bar in performance
		// order, so the metronome keeps time even when a bar has no note
		// on a particular beat.
		perBar := BarSteps(tab)
		tick := countInTicks
		for _, b := range RepeatOrder(tab) {
			var barTicks int
			for _, s := range perBar[b] {
				barTicks += s.Ticks
			}
			for q := int64(0); q < int64(barTicks); q += ticksPerQuarter {
				evts = append(evts, clickEvent(clickChannel, q == 0, tick+q)...)
			}
			tick += int64(barTicks)
		}
	}
	for _, n := range notes {
		n.Tick += countInTicks
		evts = append(evts, n)
	}
	// SMF deltas require chronological order; stable keeps program changes
	// ahead of same-tick notes and clicks.
	sort.SliceStable(evts, func(i, j int) bool { return evts[i].Tick < evts[j].Tick })
	return evts, countInTicks, countInDur, nil
}

// ScheduleTimeAtStep returns the music position of schedule step idx.
// The sum converts in one shot — StepDuration floors zero/short values for
// noteoffs, and a per-step floor would drift every step's start position.
func ScheduleTimeAtStep(schedule []PlaybackStep, idx, bpm int) time.Duration {
	total := 0
	for i := 0; i < idx && i < len(schedule); i++ {
		total += schedule[i].Ticks
	}
	if total <= 0 {
		return 0
	}
	return StepDuration(total, bpm)
}

// ScheduleSpan returns the total music duration of a schedule.
func ScheduleSpan(schedule []PlaybackStep, bpm int) time.Duration {
	return ScheduleTimeAtStep(schedule, len(schedule), bpm)
}

// StartSMF launches fluidsynth in shell mode with midPath loaded into its
// built-in player (which auto-starts). The caller drives the player through
// shell commands; the process stays alive after the file ends.
func (s *Synth) StartSMF(midPath string) error {
	if s.running {
		if err := s.Stop(); err != nil {
			return fmt.Errorf("stop previous playback: %w", err)
		}
	}
	sf := s.Soundfont
	if sf == "" {
		sf = findSoundfont()
	}
	if sf == "" {
		return errors.New(noSoundfontMessage())
	}
	gain := fmt.Sprintf("%.2f", float64(s.Volume)/100.0*2.0)
	if s.Volume <= 0 {
		gain = "0.0"
	}

	var candidates []candidate
	for _, driver := range audioDrivers() {
		candidates = append(candidates, candidate{
			bin:    "fluidsynth",
			driver: driver,
			args:   fluidsynthArgsSMF(driver, gain, sf, midPath),
		})
	}

	var lastErr error
	for _, c := range candidates {
		path, err := lookPath(c.bin)
		if err != nil {
			continue
		}
		cmd := exec.Command(path, c.args...)
		cmd.SysProcAttr = childProcAttr()
		cmd.Stdout = io.Discard
		var stderr stderrCollector
		cmd.Stderr = &stderr
		stdin, err := cmd.StdinPipe()
		if err != nil {
			lastErr = err
			continue
		}
		if err := cmd.Start(); err != nil {
			_ = stdin.Close()
			lastErr = fmt.Errorf("%s %v: %w", path, c.args, err)
			continue
		}
		startReaper(cmd)
		time.Sleep(200 * time.Millisecond)
		if !processAlive(cmd) {
			killProcessTree(cmd)
			_ = stdin.Close()
			msg := stderr.String()
			if msg == "" {
				msg = "process exited immediately (audio backend may be unavailable)"
			}
			lastErr = fmt.Errorf("%s: %s", path, summarizeStderr(msg))
			continue
		}
		s.cmd = cmd
		s.stdin = stdin
		s.realtime = false
		s.smfPlayer = true
		s.running = true
		s.ActiveDriver = c.driver
		s.LastError = ""
		return nil
	}
	if lastErr != nil {
		return fmt.Errorf("MIDI playback failed: %w", lastErr)
	}
	return errors.New("no synthesizer found — install fluidsynth")
}

// PlayMIDIFile bakes the tab into an SMF and plays it through fluidsynth's
// built-in player. Returns immediately; the cursor tracks Engine.Elapsed().
func (e *Engine) PlayMIDIFile(tab *model.Tab, bpm int, opts MIDIFileOpts) error {
	if err := e.checkShutdown(); err != nil {
		return err
	}
	if tab != nil && len(tab.Tuning) == 0 {
		tab.Tuning = model.Standard
	}
	if bpm <= 0 {
		bpm = 120
	}
	evts, countInTicks, countInDur, err := buildSMFEvents(tab, bpm, opts)
	if err != nil {
		return err
	}
	if len(evts) == 0 {
		return errors.New("no MIDI notes in tab — nothing to play")
	}
	data, err := WriteTabSMF(evts, bpm, tab)
	if err != nil {
		return fmt.Errorf("write smf: %w", err)
	}
	midPath, err := e.Synth.writeMidTemp(data)
	if err != nil {
		return err
	}

	e.beginMIDI()
	if err := e.Synth.StartSMF(midPath); err != nil {
		e.mode = ""
		return err
	}
	// The file auto-plays from 0; queue stop → seek → continue so playback
	// begins exactly at the requested position (the commands run back-to-
	// back once the shell is ready, so the false start is inaudible).
	startTick := countInTicks + midiTicksAt(opts.StartAt, bpm)
	_ = e.Synth.sendRealtime("player_stop")
	_ = e.Synth.sendRealtime(fmt.Sprintf("player_seek %d", startTick))
	if !opts.Metronome {
		_ = e.Synth.sendRealtime(fmt.Sprintf("cc %d 7 0", clickChannel))
	}
	_ = e.Synth.sendRealtime("player_cont")
	e.midiBase = opts.StartAt
	e.midiWall = time.Now()
	e.midiBPM = bpm
	e.midiCountIn = countInDur
	e.midiCountInTicks = countInTicks
	e.playbackStart = e.midiWall
	return nil
}

// midiCalibrate re-anchors the wall-clock position tracker after a seek or
// tempo change: the position is known exactly at that instant.
func (e *Engine) midiCalibrate(pos time.Duration) {
	e.midiBase = pos
	e.midiWall = time.Now()
	e.midiCountIn = 0
}

// MIDISeek jumps the SMF player to the music position pos.
func (e *Engine) MIDISeek(pos time.Duration) error {
	if !e.Synth.smfPlayer {
		return errors.New("live seek not supported by this synth")
	}
	if pos < 0 {
		pos = 0
	}
	tick := e.midiCountInTicks + midiTicksAt(pos, e.midiBPM)
	// fluidsynth's player_seek is relative to the current playhead, not an
	// absolute tick. Stop resets the playhead to the beginning, then seek
	// forward to the desired absolute tick and continue.
	_ = e.Synth.sendRealtime("player_stop")
	if err := e.Synth.sendRealtime(fmt.Sprintf("player_seek %d", tick)); err != nil {
		return err
	}
	if err := e.Synth.sendRealtime("player_cont"); err != nil {
		return err
	}
	e.midiCalibrate(pos)
	return nil
}

// MIDISetTempo changes the playback tempo live — fluidsynth's player
// retimes itself, no restart.
func (e *Engine) MIDISetTempo(bpm int) error {
	if !e.Synth.smfPlayer {
		return errors.New("live tempo not supported by this synth")
	}
	pos := e.MIDIPosition()
	if err := e.Synth.sendRealtime(fmt.Sprintf("player_tempo_bpm %d", bpm)); err != nil {
		return err
	}
	e.midiBPM = bpm
	e.midiCalibrate(pos)
	return nil
}

// MIDISetClicks mutes or unmutes the baked metronome channel.
func (e *Engine) MIDISetClicks(on bool) error {
	if !e.Synth.smfPlayer {
		return errors.New("metronome toggle not supported by this synth")
	}
	vol := 0
	if on {
		vol = 127
	}
	return e.Synth.sendRealtime(fmt.Sprintf("cc %d 7 %d", clickChannel, vol))
}

// MIDIPosition returns the current music position (excluding count-in).
func (e *Engine) MIDIPosition() time.Duration {
	if e.midiWall.IsZero() {
		return e.midiBase
	}
	since := time.Since(e.midiWall)
	if since < e.midiCountIn {
		return e.midiBase
	}
	return e.midiBase + since - e.midiCountIn
}

// MIDILive reports whether MIDI playback runs through the controllable SMF
// player (as opposed to the one-shot synth fallback).
func (e *Engine) MIDILive() bool { return e.Synth.smfPlayer }
