### FILE: synth.go

#### WHAT IT DOES
Takes a list of MIDIEvent and produces actual sound. The simplest path:
generate a .mid file to /tmp and shell out to fluidsynth.

#### STEP-BY-STEP (Option B: MIDI file + external synth)

1. Use a Go MIDI library to write a Standard MIDI File (SMF):
   - `github.com/gomidi/midi` is the most mature option but has a larger API surface.
   - Alternative: write a raw SMF yourself. The format is binary but simple:
     header chunk + track chunk + events with delta times. ~200 lines of Go.

2. Write the MIDI events with delta times (tick differences, not absolute ticks).
   Convert absolute ticks to deltas: `delta[i] = tick[i] - tick[i-1]`.

3. Save to `/tmp/fretboard_playback.mid`.

4. Shell out: `exec.Command("fluidsynth", "-a", "alsa", "-g", "1.0",
   "/usr/share/sounds/sf2/FluidR3_GM.sf2", "/tmp/fretboard_playback.mid").Run()`.

5. Optionally stream: use `cmd.StdoutPipe()` and handle output.

#### REAL-TIME PLAYBACK (Option C — future)

If you want play/pause responsiveness, you need real-time audio:

1. Use `github.com/ebitengine/oto` (low-level audio output) or `github.com/faiface/beep`.
2. Pre-generate a buffer of float32 samples (sine waves at each MIDI frequency).
3. Enqueue samples as playback progresses.
4. A goroutine feeds samples; Bubble Tea sends pause/resume commands via channels.

This is harder but gives instant response. Skip for MVP.

#### GO CONCEPTS
- `os/exec` package — running external commands.
- `io.Writer` — writing binary data.
- `os.CreateTemp` — safe temp file creation.
- Binary encoding with `encoding/binary` (LittleEndian/BigEndian).
- Goroutines and channels for async playback (Phase 2+).

#### GOTCHAS
- `fluidsynth` must be installed on the user's system. Document as a dependency.
  Fall back to `timidity` if not found.
- SoundFont path varies by distro. Provide a `--soundfont` flag.
- MIDI file timing: delta times are in ticks, not milliseconds. The file header
  sets the tempo (microseconds per quarter note).
- Killing a running synth process on quit: use `cmd.Process.Signal(os.Interrupt)`.

#### IF STUCK
- "standard MIDI file format specification" — binary layout
- "golang encoding binary write example"
- "golang os exec run command and wait"
- "fluidsynth command line example"

### FILE: cursor.go

#### WHAT IT DOES
Tracks where the playhead is during playback: which bar, which column.

#### HOW TO THINK ABOUT IT
It's a simple state machine. A goroutine advances the cursor at the tempo rate
and sends position updates to the TUI via Bubble Tea messages.

#### STEP-BY-STEP
1. Define `Cursor struct { Bar, Col int; Playing bool }`.
2. In the playback goroutine:
   ```
   for cursor.Playing {
       time.Sleep(beatDuration)
       cursor.Col++
       if cursor.Col >= columnsInBar(cursor.Bar) {
           cursor.Bar++
           cursor.Col = 0
       }
       // BUG: how does cursor get back to the TUI model?
   }
   ```
3. The TUI model launches the goroutine in its `Init()` and receives cursor
   updates via a channel. Use `tea.Batch(cmd)` to subscribe to channel messages.
4. Define `type TickMsg struct{ Bar, Col int }`. The goroutine sends these
   on a channel. Bubble Tea's `Update` function handles TickMsg.

#### GO CONCEPTS
- Goroutines and channels.
- `tea.Cmd` type: `func() tea.Msg`.
- `time.Ticker` for periodic events (better than time.Sleep in a loop).
- Select statement for play/pause/quit.

#### GOTCHAS
- The goroutine must stop when playback stops or the program quits.
  Use a `done chan struct{}` and `select` on it.
- Sending on a closed channel panics. Close only from the sender.
- The cursor advances in real-time, but the TUI may only render at ~30fps.
  Don't send every tick as a message — send periodic updates or batch them.

#### SKELETON

type TickMsg struct {
    Bar int
    Col int
}

type Player struct {
    cursor   model.Cursor
    ticker   *time.Ticker
    done     chan struct{}
    msgs     chan TickMsg
}

func (p *Player) Start(bpm int, totalBars int) tea.Cmd {
    return func() tea.Msg {
        p.ticker = time.NewTicker(time.Minute / time.Duration(bpm*4)) // quarter note
        p.done = make(chan struct{})
        go func() {
            for {
                select {
                case <-p.ticker.C:
                    p.cursor.Advance()
                    p.msgs <- TickMsg{Bar: p.cursor.Bar, Col: p.cursor.Col}
                case <-p.done:
                    return
                }
            }
        }()
        return <-p.msgs // return first message to start the loop
    }
}
