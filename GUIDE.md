# fretboard — developer guide

This document explains how the codebase is structured and how to navigate it.
It is the companion to AGENTS.md (workflow rules) and BUGS.md (bug tracker).

## Repository layout

```
cmd/fretboard/        Thin main: os.Exit(cli.Run(...))
internal/
  cli/                Flag parsing, config load, subcommands (import, test-audio),
                      TUI bootstrap, post-run cleanup. All non-TUI output goes
                      through injected stdout/stderr so it is testable.
  model/              Pure domain types: Tab, Bar, Segment, metadata keys
                      (MetaKey*), BPM helpers, tuning helpers. No I/O.
  parser/             ASCII / Guitar Pro text → model.Tab. Dedupes BPM tuning
                      normalization via model helpers.
  library/            SQLite-backed store: tabs CRUD, import, search, sort,
                      favorites, play counts. Schema migrations live here.
  player/             Audio engine: Synth (fluidsynth/timidity), audio-catalog
                      resolution (local dirs + online), realtime MIDI playback
                      with sustain, playback scheduling, cursor/audio sync.
  scraper/            Ultimate Guitar (API + HTML fallback) and Songsterr
                      clients; shared rate limiting (ratelimit.go).
  watcher/            fsnotify auto-import watcher for the configured import dir.
  config/             ~/.config/fretboard/config.json load/save.
  ui/                 The Bubble Tea frontend (see below).
tests/                Separate Go module: e2e tests + shared helpers.
```

## The ui/ package layout

The TUI follows the screen-per-model pattern with a central router. Each
package has a narrow job:

| package | job |
|---------|-----|
| `ui/app` | The router (`AppModel`). Owns the view stack, the library store, the watcher, and the theme. Switches between screens and handles cross-screen keys (`q`, `t`, `?`). |
| `ui/home` | Landing page: stats, recent tabs, quick actions, auto-import warning. |
| `ui/browser` | Library browser: list, filter (`/`), sort, favorite, delete-with-confirmation, right-side tab preview. |
| `ui/search` | Online search (UG/Songsterr) with query box, results, import flow. |
| `ui/viewer` | Tab viewer + playback: page-layout rendering, key handling, BPM, jump/pan, audio fetch, audio-sync offset calibration (`[ ]` ±0.5s, `{ }` ±5s, `o` reset, persisted per tab). Split into `viewer.go` (state/keys), `viewer_playback.go` (playback lifecycle commands), `viewer_audio.go` (audio picker + catalog). |
| `ui/help` | Help screen. |
| `ui/kit` | Presentational kit: styles (exported vars), themes, chrome (panels, footer, status bar), tab rendering helpers, key constants, truncate. No app logic. |
| `ui/msgs` | Pure data-contract: every message type shared between screens and the router. Screens never import each other — they communicate through `msgs`. |

## Message flow

Everything is a message:

```
user presses 'j' → tea.KeyMsg → AppModel.Update (router)
    → forwards to the active screen's Update (e.g. browser)
    → screen updates its state, returns optional cmds
    → View() re-renders the screen string
```

Cross-screen transitions are messages defined in `ui/msgs`:

- `HomeLibraryMsg` / `HomeSearchMsg` — home → router → navigate.
- `TabSelectedMsg{ID}` — browser → router → `openTab` (stop audio, load tab,
  navigate to viewer, kick off audio fetch).
- `ViewLibraryMsg` / `ViewHomeMsg` — viewer's `b`/`H` keys.
- `TabFetchedMsg` / `TabImportErrorMsg` — search result import results. The
  router checks the request generation (`search.AcceptsGen`) so stale results
  are dropped, then imports into the store and navigates to the viewer.
- `Playback*Msg`, `AudioFetchedMsg`, `AudioCatalogMsg` — viewer-internal flows
  that travel through the router's message loop.

Commands (`tea.Cmd`) that do async work (DB queries, network, audio) live with
the screen that owns them — never in `msgs`.

## Rules the codebase follows

1. **Screens don't import each other.** `home`, `browser`, `search`, `viewer`,
   `help` only import `kit`, `msgs`, and domain packages. Only `app` imports
   every screen.
2. **The router pokes screens through exported API only.** `StopPlayback`,
   `SetError`, `IsSearchActive`, `AcceptsGen`, `SetAutoImportWarn`, etc. Never
   reach into unexported fields of another package.
3. **Domain code has no I/O, no bubbletea.** `model`, `parser` are pure;
   `library`, `player`, `scraper` own their side effects and return errors.
4. **`kit` owns all rendering** — strings are never styled inline in screens.
5. **Style vars are exported from kit** (`kit.ErrorStyle`, `kit.ListSelected`)
   and reassigned wholesale by `kit.SetTheme`. Never mutate a shared style
   var in place — always `.Copy()`.
6. **Key constants live in `kit`** (`kit.KeyQuit`, `kit.KeyDown`, ...). Every
   screen's help text and handling use the same constants.
7. **Shutdown is idempotent.** `AppModel.Shutdown` stops playback, shuts down
   the synth, closes the watcher; it is safe to call more than once. External
   SIGTERM/SIGINT is handled by bubbletea itself — do NOT add a second
   signal handler (two senders on its unbuffered message channel deadlock
   shutdown). Cleanup runs against the live model returned by `p.Run()` in
   `internal/cli`.

## Bubble Tea gotchas (learned the hard way)

- The first `tea.WindowSizeMsg` can arrive AFTER `Init()`. Initialize screens
  with a default size (80x24) and resize on the first window message.
- Never store `*tea.Program` in a model. Return `tea.Quit` instead.
- `p.Send` is a non-blocking channel send; it is safe from other goroutines,
  but the channel is UNBUFFERED — two concurrent senders during shutdown
  deadlock. Route shutdown through the model, not a second signal handler.
- The playback goroutine must be stopped before the engine is released:
  `stopPlayback()` cancels the tick loop; `engine.Shutdown()` kills the synth
  process tree. Call them in that order (see `AppModel.Shutdown`).
- Terminal width math must use display width, not `len(s)` — multibyte runes
  (e.g. `│`, `—`) are 1 column each in kit rendering but 2+ bytes.
