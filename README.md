# fretboard

A terminal guitar tab viewer, player, and library manager. Search online tabs,
import local files, browse your library, and play back tabs with audio — all
from a keyboard-driven Bubble Tea TUI.

## Features

- **ASCII tab viewer** with syntax highlighting, string colors, and vim-style navigation.
- **Rhythm-aware playback** — infers note durations from column spacing; honors rhythm rows (`| q e e q |`) when present; plays repeats (`|:` `:|`) and 1./2. endings in human reading order.
- **Audio playback** — tries a real backing track first (`ffplay`/`mpv`), then MIDI via `fluidsynth`/`timidity`, with a moving cursor.
- **Online tab search** across four sources — Ultimate Guitar (API + HTML fallback), Songsterr, GuitarTabs.cc, and GuitareTab.com (`o` in library), merged **best-first**: tabs outrank chord sheets, and UG rating/votes (`* 4.9 · 2.1k`) decide the order, so the official version surfaces above covers.
- **Guitar Pro import** for `.gp3`, `.gp4`, `.gp5`, `.gpx` via the bundled `gp-parser` helper (Rust + `guitarpro`).
- **Local library** backed by SQLite (fuzzy search, sort, favorites, delete).
- **Themes** — built-in default, One Dark, and Dracula palettes (`t` anywhere).
- **Import** local files or whole directories from the CLI.
- **Config file** at `~/.config/fretboard/config.json`.

## Quick start

```bash
# View a single tab (.txt or Guitar Pro)
fretboard path/to/tab.txt
fretboard path/to/song.gp5

# Open the library browser
fretboard

# Import a file or a directory of tabs
fretboard import path/to/tab.txt
fretboard import path/to/tabs/

# Search online (open the app and press 'o' in the library)
```

## Requirements

- Go 1.25+
- A MIDI synthesizer for playback:
  - `fluidsynth` + a GM soundfont (set `FRETBOARD_SOUNDFONT` to override), or
  - `timidity`
- Optional: `yt-dlp` for automatic online backing-track lookup
- Optional: `cargo` to build the Guitar Pro parser (`tools/gp-parser`)

## Keybindings

### Library browser

| Key | Action |
|-----|--------|
| `j`/`↓` | Move down |
| `k`/`↑` | Move up |
| `Enter` | Open selected tab |
| `/` | Fuzzy search/filter |
| `s` | Cycle sort order |
| `o` | Online search (UG + Songsterr + GuitarTabs + GuitareTab) |
| `f` | Toggle favorite |
| `d` | Delete tab |
| `r` | Reload library |
| `t` | Cycle theme |
| `q`/`Ctrl+c` | Quit |

### Tab viewer

| Key | Action |
|-----|--------|
| `a` | Pick audio source (local / online / MIDI) |
| `Space` | Play/pause |
| `+`/`-` | BPM up/down |
| `>`/`<` | Playback speed up/down (`r` resets) |
| `m` | Metronome on/off (MIDI playback) |
| `C` | Count-in: 1–2 bars of lead-in clicks |
| `y` | Cycle MIDI instrument (steel/nylon/clean/overdrive/bass) |
| `j`/`k` | Scroll down/up |
| `h`/`l` | Pan left/right |
| `gg` | First bar |
| `G` | Last bar |
| `0-9` + `Enter` | Jump to bar |
| `/` | Search bar number or fret pattern (`n`/`N` next/prev) |
| `T`/`Z` | Transpose ±1 semitone (`R` resets) |
| `e` | Toggle note names instead of fret numbers |
| `v` | Toggle grid / linear layout |
| `f` | Follow-mode auto-scroll |
| `i`/`u`/`x` | Set loop point A / B / clear loop |
| `X` | Export tab to file + clipboard |
| `[`/`]` | Nudge audio sync ±0.5s (`{`/`}` = ±5s, `,`/`.` = ±0.1s, `o` resets) |
| `s`/`S` | Sync current bar to audio / remove last sync anchor |
| `a` | Audio picker with strict badges (`[official]`/`[live]`/..., `*` recommended) |
| `b` | Back to library |
| `t` | Cycle theme |
| `q`/`Ctrl+c` | Quit |

### Syncing with a recording

1. Pick the studio/official source in the picker (`a`) — in strict mode (`strict`)
   live/cover/lesson recordings are excluded from auto-pick.
2. Intros are auto-detected from leading silence (`offset +Xs`, ffmpeg optional);
   fine-tune with `[ ]` (±0.5s) or `, .` (±0.1s), reset with `o`.
3. During playback, jump to a bar you recognize (number + `Enter`), then press
   `s` exactly when you hear it; repeat at 2–3 bars. Anchors build a tempo map
   (`60->120 bpm`) and a drift estimate (`±2.0s`) that keep the playhead synced
   even when the tempo changes.
4. Calibration is stored **per audio source** — switching to another recording
   restores that source's own offset and anchors.

### Online search

| Key | Action |
|-----|--------|
| `/` | Focus search box |
| `Enter` | Search |
| `j`/`k` | Move through results |
| `Enter` | Fetch and open tab |
| `Esc` | Clear / back |

## Configuration

`~/.config/fretboard/config.json`:

```json
{
  "theme": "onedark",
  "ug_delay_ms": 500,
  "volume_percent": 80,
  "auto_fetch_audio": true,
  "strict_audio_selection": true,
  "audio_search_paths": ["/path/to/music"]
}
```

`strict_audio_selection: false` disables studio-lock auto-pick (live/cover
recordings then compete with official ones).

Environment variables:

| Variable | Purpose |
|----------|---------|
| `FRETBOARD_SOUNDFONT` | Path to a GM `.sf2` soundfont for fluidsynth |

Backing tracks: drop `Artist - Title.mp3` in `~/.config/fretboard/audio/` or next to the tab file.
| `FRETBOARD_GP_PARSER` | Path to the `gp-parser` binary (Guitar Pro import) |

## Guitar Pro parser

Build the Rust helper once:

```bash
cd tools/gp-parser && cargo build --release
```

The binary is auto-discovered at `tools/gp-parser/target/release/gp-parser`, or set `FRETBOARD_GP_PARSER`.

## Development

```bash
go test ./...
go test ./... -tags network   # includes network tests (Ultimate Guitar)
```

E2E tests live in the `tests/` module.

```bash
cd tests && go test ./...
```

## Project structure

```
cmd/fretboard/          # CLI entry point
internal/
  config/               # user preferences
  model/                # Tab, Bar, Tuning, Segment domain types
  parser/               # ASCII + Guitar Pro parsers
  player/               # MIDI events, rhythm scheduling, SMF, synth
  library/              # SQLite library CRUD
  scraper/              # UG API/HTML + Songsterr + plain-text tab sites
  tui/                  # Bubble Tea frontend
tools/gp-parser/        # Rust GP -> JSON converter
```

## License

MIT
