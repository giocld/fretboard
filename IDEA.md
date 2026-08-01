# fretboard

A TUI guitar tab viewer, player, and library manager — built in Go with Bubble Tea.

## The Gap

There is no `bat`/`less` for guitar tabs in the terminal. Existing tools:
- `tuitar` (Go, 3 stars) — basic vim-style editor, rough audio
- `tfb` (Go, 76 stars) — fretboard reference, one-shot CLI, not a tab tool
- `go-tabs` (Go, 13 stars) — tab *generation* library, not a viewer
- `VITABS` (C, 44 stars) — old vim-inspired editor
- No Go Guitar Pro (.gp3/.gp4/.gp5/.gpx) parser exists (Rust has `slundi/guitarpro`)

---

## Phase 1 — Viewer (MVP)

Goal: `fretboard path/to/tab.txt` opens a scrollable, syntax-highlighted tab.

### Data Model

Represent a tab as a structured type, not raw text:

```go
type Tab struct {
    Title    string
    Artist   string
    Tuning   Tuning         // e.g. ["E","A","D","G","B","e"]
    Bars     []Bar         // ordered sections
    Metadata map[string]string
}

type Tuning []string      // low to high, e.g. ["E","A","D","G","B","e"]

type Bar struct {
    Number   int           // bar number (0 = unnumbered intro)
    Strings  []StringLine  // one per string in the tuning
}

type StringLine struct {
    Segments []Segment     // ordered series of notes/rests in this string within this bar
}

type Segment struct {
    Char     rune          // '-' for rest, digit for fret, 'h','p','b','/','\\' etc.
    Position int           // column index within the bar
}
```

### Parser

Parse ASCII tabs in two passes:

**Pass 1 — Structural:** Split by blank lines into sections. Detect the tab region by counting lines where digits dominate (heuristic: >60% of non-whitespace chars are 0-9 or hyphens). Extract header lines (artist, title, tuning) from lines above the tab region. Count strings to infer tuning.

**Pass 2 — Note extraction:** Walk each tab line character by character. Group columns into bars by detecting the `|` pipe delimiter bars. Within each bar, extract fret numbers and technique characters into `StringLine` segments. Handle technique notation:
- `h` / `p` — hammer-on / pull-off (appears between two fret digits)
- `b` / `r` — bend / release
- `/` / `\` — slide up / down
- `~` — vibrato
- `x` — muted/dead note
- `()` — ghost note (optional, less critical for MVP)

**Edge cases to handle:**
- Tabs with no bar lines (free-form) — treat the whole thing as one bar
- Multi-bar rests (just hyphens spanning several bars) — collapse or skip
- Chords stacked vertically (multiple digits at the same horizontal position across strings)
- Capo notation (`Capo 3`) in the header — adjust displayed fret numbers
- Rhythm markers above the tab (`| q  e  e  q  |`) — parse or ignore for phase 1

### Rendering

Use `lipgloss` for styling. The viewport is a `viewport.Model` from bubbletea.

**Layout:**
- **Top bar** (1 row): filename, detected tuning, bar range visible (`bars 4-8 / 42`)
- **Tab area** (remaining rows - 2): the tablature itself
- **Bottom bar** (1 row): command line / status

**String rendering:**
Each string gets a distinct color via lipgloss adaptive colors:
```
e (high) → brightest
B       → slightly dimmer
G       →
D       →
A       →
E (low) → dimmest
```

Also highlight fret numbers differently from technique characters and hyphens.

**Scrolling:**
- Vertical: offset into `Bars` array. Show as many bars as fit in the viewport height / 2 (string rows + spacer between bars).
- Horizontal: within a long bar, offset the visible columns. Use `h` / `l` to pan left/right when a bar is wider than the terminal.

### Keybindings (vim-style, Normal mode)

| Key     | Action                        |
|---------|-------------------------------|
| `j`/`k` | Scroll down/up one bar        |
| `h`/`l` | Scroll left/right within bar  |
| `gg`    | Jump to first bar             |
| `G`     | Jump to last bar              |
| `Ctrl+d`/`Ctrl+u` | Half-page down/up   |
| `/`     | Enter search (search bar numbers or fret patterns) |
| `n`/`N` | Next/previous search result   |
| `0`     | Jump to bar N (type number, hit enter) |
| `:`     | Enter command mode (quit, open, help) |
| `q`/`Esc` | Quit                       |

### Implementation Order

1. **`internal/parser/`** — ASCII tab parser (no rendering). Unit test with 10+ real-world tabs covering edge cases.
2. **`internal/model/`** — `Tab`, `Bar`, `StringLine`, `Segment` types.
3. **`internal/tui/`** — Bubble Tea model, viewport, basic render loop. Start with monochrome, add lipgloss styling after.
4. **`cmd/fretboard/`** — Main entry point. `fretboard <file>` loads, parses, renders.
5. Search (`/`) and bar jumping (`0`) as final touches.

### Tests

Collect 15-20 real ASCII tabs from Ultimate Guitar, Songsterr, and random `.txt` files. Cover:
- Standard format (artist/title header, tuning, `|` bar delimiters)
- Free-form (no bars, just hyphens and digits)
- Multi-instrument (bass tab below guitar tab — skip bass for MVP)
- Capo notation
- Non-standard tunings (Drop D, Open G, DADGAD)
- Chords stacked vs single-note lines
- Empty tabs / edge cases

---

## Phase 2 — Player

Goal: hit spacebar, hear the tab play with a moving cursor.

### How It Works

1. **MIDI generation:** Walk the `Bars` array sequentially. For each `Segment` with a fret digit, compute the MIDI note number (`openStringMIDI + fret`). If multiple strings have notes at the same position, they're a chord — same timestamp, multiple note-on events. Technique characters affect note duration/velocity (staccato `x`, legato `h`/`p`).

2. **Timing:** User sets BPM. Default assumption is 4/4 time, one number position = one beat (or sub-beat depending on density). This is the trickiest part — ASCII tabs rarely encode precise rhythm. Two approaches:
   - **Naive:** Treat every vertical column as an equal-duration step. Simple, works for roughly-even tabs.
   - **Rhythm-aware:** Parse rhythm markers above the tab (`| q  e  e  q  |`) where present. Fall back to naive otherwise.

3. **Audio output:**
   - **Option A (simplest):** Shell out to `fluidsynth` or `timidity`. Generate a `.mid` file to `/tmp`, pipe it through the synth. Slight latency from disk I/O but zero audio code to write.
   - **Option B (native):** Use a Go MIDI synth lib like `github.com/gomidi/midi` or `github.com/insomniacslk/fluentmidi`. Generate and write MIDI bytes to stdout, pipe to external player. Still avoids real-time audio code.
   - **Option C (real-time):** Oto/vorbis/beep-based audio. Generate waveform samples from MIDI note frequencies, enqueue them into a real-time audio buffer. More work but allows instant play/pause.

   Start with **Option B** — generate MIDI, play via `fluidsynth`.

### Cursor Tracking

During playback, a goroutine advances through bars/positions at the current tempo. It sends `tickMsg` to the bubbletea model, which updates the cursor position. The renderer highlights the current position (invert colors or an underline).

### Playback Controls

| Key           | Action                  |
|---------------|-------------------------|
| `Space`       | Play / Pause            |
| `+`/`-`       | BPM up/down by 5        |
| `[` / `]`     | Mark loop start / end   |
| `\`            | Toggle loop on/off       |
| `m`           | Metronome on/off        |
| `←` / `→`    | Skip bar backward/forward |

### Loop Implementation

Two ints: `loopStart` and `loopEnd` (bar indices). When `playing` is true and loop is enabled, the tick goroutine resets position to `loopStart` when it crosses `loopEnd`.

---

## Phase 3 — Library

Goal: `fretboard` with no args opens a library browser. Manage your tabs like a music player manages songs.

### Storage

SQLite via `modernc.org/sqlite` (pure Go, no CGo). Schema:

```sql
CREATE TABLE tabs (
    id        INTEGER PRIMARY KEY,
    filepath  TEXT NOT NULL,
    title     TEXT,
    artist    TEXT,
    tuning    TEXT,        -- JSON array
    difficulty INTEGER,    -- 1-5, user-assigned
    tags      TEXT,        -- JSON array: ["rock","solo","intermediate"]
    added_at  DATETIME,
    last_played DATETIME,
    play_count INTEGER DEFAULT 0,
    favorite  BOOLEAN DEFAULT 0
);
```

On import, the parser extracts title/artist/tuning from the tab file. User fills in difficulty and tags manually.

### Browser UI

```
┌─ fretboard ──────────────────────────────────┐
│ 🔍 Search: _______________                    │
│──────────────────────────────────────────────│
│  ★ Sultans of Swing        Dire Straits     │
│    Little Wing             Jimi Hendrix      │
│  ★ Stairway to Heaven      Led Zeppelin      │
│    Black Dog               Led Zeppelin      │
│    Hotel California        Eagles            │
│──────────────────────────────────────────────│
│ 42 tabs  │  Sort: recent  │  :help  :quit    │
└──────────────────────────────────────────────┘
```

**Features:**
- Fuzzy search across title, artist, tags (use `github.com/junegame/` or `github.com/sahilm/fuzzy`)
- Sort by: recent, alphabetical, most played, difficulty
- `Enter` to open a tab (switches to viewer view)
- `d` to delete, `e` to edit metadata, `f` to toggle favorite
- `i` to import a file or directory of tabs (recursive scan)

### Ultimate-Guitar Integration

Use `Pilfer/ultimate-guitar-scraper` to:
- `:search <query>` — search UG, show results in a list view, `Enter` fetches and saves locally
- `:import <url>` — import a specific UG tab by URL
- Cache results in SQLite, show "From Ultimate-Guitar" badge in browser

Be mindful of rate limits. Add a `--ug-delay` flag.

### File Watch

Use `fsnotify` to watch the tab directory. New `.txt` files dropped in are auto-imported and parsed. Feels like a music library that just works.

---

## Phase 4 — Guitar Pro Support

Goal: open `.gp3`, `.gp4`, `.gp5`, `.gpx` files natively.

### The Problem

No Go library parses Guitar Pro files. The format is binary and complex (multi-track, precise rhythm, effects, lyrics, etc.). Writing a parser from scratch for all versions is months of work.

### Options

**Option A — CGo bridge to `libguitarpro` (C library):**
Pros: full format support, battle-tested. Cons: CGo complicates cross-compilation, static binaries get harder.

**Option B — Rust FFI to `slundi/guitarpro`:**
Write a thin Rust wrapper around `slundi/guitarpro` that reads a `.gp5` file and emits JSON to stdout. Call it as a subprocess from Go. No FFI, no CGo. The Rust binary is a separate crate, compiled once and shipped alongside `fretboard`.

**Option C — Write a Go parser from scratch:**
Only realistic for one format version (e.g., `.gp5`). The older formats (.gp3/.gp4) are simpler but poorly documented. `.gpx` is XML-based (easiest to parse but most complex music semantics).

**Option D — Convert first, then view:**
Use `tuxguitar` CLI to batch-convert `.gp5` → MusicXML or MIDI, then parse that. Ugly but works as a stopgap.

**Recommendation:** Start with **Option B** (Rust subprocess). Write a `gp-parser` crate that wraps `slundi/guitarpro`, outputs a well-defined JSON schema. `fretboard` calls `gp-parser <file.gp5>` and loads the JSON. The JSON schema matches a superset of the internal `Tab` model:

```json
{
  "title": "...",
  "artist": "...",
  "tracks": [
    {
      "name": "Lead Guitar",
      "tuning": ["E","A","D","G","B","e"],
      "bars": [
        {
          "time_signature": [4,4],
          "voices": [
            {
              "notes": [
                {"string": 1, "fret": 5, "duration": 0.25, "velocity": 100, "technique": "hammer-on"},
                ...
              ]
            }
          ]
        }
      ]
    }
  ]
}
```

### Display Strategy

Multi-track GP files: show a track selector (`1: Lead Guitar`, `2: Rhythm`, `3: Bass`). Tab between tracks. Render the selected track as if it were an ASCII tab, with rhythm information rendered as note durations above/below the string lines.

This is a significant engineering effort. Happy to defer to a later phase or even a separate project (`gp-parser` as a standalone Go library).

---

## Project Structure

```
fretboard/
├── cmd/
│   └── fretboard/
│       └── main.go              # entry point, flag parsing
├── internal/
│   ├── model/
│   │   ├── tab.go               # Tab, Bar, StringLine, Segment types
│   │   └── tuning.go            # Tuning type, standard tunings, semitone math
│   ├── parser/
│   │   ├── ascii.go             # ASCII tab parser (Pass 1 + Pass 2)
│   │   ├── ascii_test.go
│   │   └── testdata/            # real-world tab fixtures
│   ├── player/
│   │   ├── midi.go              # MIDI note generation from Tab
│   │   ├── synth.go             # shell out to fluidsynth/timidity
│   │   └── cursor.go            # playback cursor state machine
│   ├── library/
│   │   ├── db.go                # SQLite schema, migrations, CRUD
│   │   ├── import.go            # file system scanner, watcher
│   │   └── scraper.go           # Ultimate-Guitar integration
│   └── tui/
│       ├── app.go               # top-level bubbletea Model
│       ├── viewer.go            # tab viewer viewport
│       ├── browser.go           # library browser view
│       ├── search.go            # search bar component
│       ├── statusbar.go         # bottom status/command bar
│       ├── styles.go            # lipgloss theme definitions
│       └── keymap.go            # keybinding definitions
├── go.mod
├── go.sum
└── README.md
```

### Dependencies (tentative)

```
github.com/charmbracelet/bubbletea
github.com/charmbracelet/lipgloss
github.com/charmbracelet/bubbles     (viewport, textinput, etc.)
modernc.org/sqlite                   (pure Go SQLite)
github.com/sahilm/fuzzy              (fuzzy search)
github.com/fsnotify/fsnotify         (file watching)
github.com/Pilfer/ultimate-guitar-scraper
gopkg.in/djherbis/times.v1           (optional: file timestamps for sorting)
```

---

## UX Sketch

```
┌─ fretboard ──────────────────────────────────────────────┐
│ File: sultans-of-swing.txt     Tuning: E Standard         │
│──────────────────────────────────────────────────────────│
│  1  e│───────────────────────────────────────────────────│
│  2  B│───────3──────────────3────────────────────────────│
│  3  G│─────0───0──────────0───0──────────────────────────│
│  4  D│───0───────0──────2───────0────────────────────────│
│  5  A│──────────────3────────────────────────────────────│
│  6  E│───────────────────────────────────────────────────│
│                          ^ cursor at bar 3, beat 2        │
│──────────────────────────────────────────────────────────│
│ ▶ Playing  │ BPM: 120 │ Loop: bars 1-8 │ Vol: 80%        │
│──────────────────────────────────────────────────────────│
│ :help  :open  :play  :loop  :tempo  :quit                │
└──────────────────────────────────────────────────────────┘
```
