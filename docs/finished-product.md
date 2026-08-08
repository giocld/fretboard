# Fretboard 1.0 — finished-product draft

> What a *completely finished* fretboard looks like: every feature domain
> expanded into sub-features and use cases, compared against the current
> codebase, and — critically — the **invisible requirements**: behaviors that
> don't read as "features" but decide whether the product feels right, and
> which must be **Confirmed** through an implementation (see §4).
>
> Status legend: ✅ done · 🟡 partial (works, refinements tracked) · ⬜ missing · 🧪
> needs confirmation (behavior exists but is unverified for the stated use case)

---

## 1. Product vision

fretboard is the `less`/`bat` of guitar tabs: a keyboard-first terminal app
where a guitarist can find any tab (local or online), read it comfortably,
play it back against a real recording (or MIDI), loop and slow down hard
passages, and keep the tab cursor glued to the music even when the recording
has an intro or drifts tempo — all without leaving the terminal, on any OS,
with zero configuration required for the happy path.

"Finished" means: the happy paths are frictionless, the sad paths are
explanatory, and nothing silently does the wrong thing (e.g. playing a cover
when you wanted the studio version, or showing a 1-star user tab above the
official one).

---

## 2. Feature domains — the full draft

### 2.1 Library & acquisition

**Goal:** get tabs in, keep them organized, never lose work.

#### 2.1.1 Import (files & directories)
- ✅ `fretboard import <file|dir>` — recursive scan, `.txt` + `.gp3/.gp4/.gp5/.gpx`.
- ✅ Unparseable files are skipped with a summary; walk failures reported.
- 🧪 **Import feedback.** Batch imports print nothing per file today. Confirm:
  "imported 3 tabs (skipped 1: bad.txt — no tab region)" on stdout, exit code 0.
- ⬜ **Import by URL** — `fretboard import <ug-tab-url>` or from the search
  screen "import this exact URL". Users paste UG links from the browser all
  the time; today only search-driven fetch exists.
- 🟡 **Re-import on change.** The watcher upserts on Create/Write/Rename — but
  only in the *top* watched directory (`fsnotify.Add(dir)` is not recursive).
  Confirm and fix: nested folders under `auto_import_path` are silently
  ignored today.
- ⬜ **Content-hash dedup.** Same tab imported from two sources = two rows.
  Confirm: on import, if title+artist+tuning already exist, ask
  "Update existing or keep both?" (filepath dedup already exists).

#### 2.1.2 Library browser
- ✅ Fuzzy filter (`/`), sort (recent/alpha/artist/plays), favorite, delete
  with confirmation, right-side preview panel (≥106 cols), empty states.
- ⬜ **Metadata editing.** `e` on a row to fix title/artist (UG fetches often
  misparse titles; today the only fix is editing the file). Must be
  Confirmed: edits persist to the DB *and* survive re-import (they don't today
  — `Import` overwrites content from the file).
- ⬜ **Filters, not just search** — `F` favorites-only, `g` filter by tuning,
  `s` filter by source badge (local/online). A 400-tab library needs
  one-keypress narrowing, not a fuzzy query.
- 🟡 **Duplicate rows** from multiple sources. Confirm a "same song, N
  versions" grouping or a `!` key to compare versions side by side.
- ⬜ **Export.** Save a tab back to a file (`x`), copy tab text to clipboard —
  the whole point of a terminal tab tool is sharing tabs.
- ⬜ **Playlists / practice sets.** "Solo practice" set with 5 songs, in order,
  with per-song loop ranges. (Biggest library-level feature still missing.)

#### 2.1.3 Persistence & safety
- ✅ SQLite (WAL), atomic config writes, corrupt-config recovery, corrupt-tab
  JSON is surfaced as an error (not a crash).
- 🧪 **DB corruption.** A truncated `fretboard.db` today → every subcommand
  exits 1 with "malformed database". Confirm: same soft-recovery treatment as
  config (warn + recreate/backup) or at least a clear `fretboard db repair`
  path.
- ⬜ **Library backup/restore** — `fretboard export-library` (JSON dump) so a
  machine move doesn't lose favorites and sync calibration.
- ⬜ **Session resume.** Reopen the app → last opened tab, last cursor bar,
  last BPM restored. Terminal users expect `vim`-style statefulness.
- ⬜ **Per-tab notes.** A scratchpad ("this tab is wrong at bar 42, capo 3")
  attached to a tab.

---

### 2.2 Online discovery

**Goal:** find the *right* version of a song — official tabs first, covers
second, garbage never.

#### 2.2.1 Search coverage
- ✅ Four sources: UG (API + HTML fallback), Songsterr, GuitarTabs.cc,
  GuitareTab.com, merged and deduplicated, with `[UG][ST][GT][GR]` badges.
- ✅ Degraded-query retries for song-only engines; chord-only/drum/album
  pages rejected with clear errors instead of importing garbage.
- ⬜ **Pagination / "more results".** Today one page per source, capped. A
  `m` key to fetch the next page. For niche songs this is the difference
  between found and "no results".
- ⬜ **Search history & repeat.** `h` in search screen = last N queries; the
  query field is empty on every entry today.
- ⬜ **Offline search cache.** Last search results kept on disk (JSON) so a
  flaky connection doesn't wipe the previous result set.

#### 2.2.2 Result quality — *the* critical usability area (see §4.1)
- 🟡 **Ranking.** UG returns `Rating`/`Votes` which are parsed into
  `SearchResult` but **never displayed or sorted on**. The merged list is in
  source-append order: a 1-star user transcription from guitaretab.com can
  sit above the official 4.9★ UG tab. Must be Confirmed: a merge that sorts
  by (type=tab first, then rating/votes, then source trust) with the rating
  shown on the row.
- ⬜ **Official-version flag.** A `★ official` badge on rows whose author is
  the artist/VEVO/label or whose page is the artist's official tab, and a
  one-key filter (`f` = official only). The audio side already has this
  concept (`ClassifyAudioCandidate`); the tab side has nothing.
- 🟡 **Type visibility.** `Type` ("Tabs"/"Chords"/"Bass") is shown dimly at
  row end. Users repeatedly fetch chord sheets expecting tabs (rejected with
  an error — correct, but the row should have warned them). Confirm: type
  badge + "tabs first" default sort + a filter for chords-only songs.

#### 2.2.3 Fetch & import flow
- ✅ Fetch → parse → backfill title/artist/tuning/capo → import to library →
  open viewer → audio fetch, with generation guards against stale results.
- ✅ **Rate limiting.** One shared `rateLimiter` throttles every backend
  (UG API, UG HTML, Songsterr, text-tab sites) with the configured
  `ug_delay_ms` / `-ug-delay` (BUG-030).
- ⬜ **Fetch retry/backoff** for transient 429/5xx with a visible
  "retrying in 3s…" spinner state.

---

### 2.3 Reading & rendering

**Goal:** any tab readable in seconds, on any terminal, in any tuning.

#### 2.3.1 Layout & navigation
- ✅ Grid page layout + linear strip, `v` toggle, adaptive bar widths,
  `h`/`l` pan, `j`/`k` bar navigation, `gg`/`G`, digit-jump, follow-mode
  auto-scroll, grid/linear-aware scroll math, loop-bar highlighting.
- 🧪 **Half-page scroll** (`Ctrl+d`/`Ctrl+u` — promised in IDEA.md, and the
  viewport supports it) — Confirm it works in the viewer; it is wired for
  search results only.
- ⬜ **Search within a tab.** `/` to find a bar number or fret pattern
  (IDEA Phase 1 listed it; never implemented). For a 200-bar song this is
  the difference between finding the solo and scrolling forever.
- ⬜ **Mouse wheel.** Bubble Tea mouse support is never enabled; wheel
  scrolling in viewer/browser/search is the #1 terminal expectation after
  keys. Confirm with `tea.WithMouseCellMotion()`.
- ⬜ **Bar minimap / section index.** A right-edge ruler with section labels
  (`Intro | Verse | Chorus | Solo`) when the tab has them.
- 🟡 **Sections.** The parser flattens everything into numbered bars; section
  headers (`[Verse]`, `Chorus:`) in real tabs are ignored. Confirm: parse
  section markers into `Bar.Section`, show them in the header row, and allow
  jump-to-section (`[`/`]` on sections).

#### 2.3.2 Notation fidelity
- ✅ Rhythm rows (`| q e e q |`), multi-bar rhythm rebasing, column-spacing
  fallback, named tunings (Drop D / Open D / E Flat), capo metadata, BPM
  metadata (both `BPM: 112` and `112 BPM`), GP `ColumnTicks` import.
- ⬜ **Repeats & structure.** `|: :|`, `1.`/`2.` endings, D.C./D.S. are not
  parsed. For playback this is *the* biggest correctness gap: a song with a
  repeated chorus plays straight through and the cursor diverges from the
  recording after the first repeat. Must be Confirmed: repeat-aware bar
  ordering in `BuildSchedule` with a visual repeat marker on the bar header.
- ⬜ **Chord sheets.** Chord-only tabs (lyrics + `[ch]`) are rejected at fetch
  (correct) but never rendered. F9 backlog: a chord-sheet view mode
  (lyrics with chords above) is a separate reading mode, not a tab.
- ⬜ **Transpose.** The most-requested tab feature after search: `+`/`-` (or
  `T`) shifts the tab ±semitone for capo-less playing, display *and* MIDI
  playback. Must interact correctly with `Capo` metadata.
- ⬜ **Note-name display.** `n` toggles fret digits → note names (uses the
  existing `Tuning.Semitone`/`midiToNoteName` — the math is already there).
- ⬜ **GP multi-track.** `gp.go` picks the *first guitar track* silently.
  Confirm: a track chooser (`t` cycles tracks when the GP file has several —
  Lead/Rhythm/Bass), with track name shown in the title row.
- ⬜ **String isolation / hide strings.** Practice view: `1`-`6` toggles
  strings on/off for readability and for muting them in MIDI playback.

#### 2.3.3 Rendering robustness
- ✅ Wide-rune-safe widths everywhere (`lipgloss.Width`), rune-safe truncation,
  footer hint trimming on narrow terminals, stat-row stacking, empty states.
- ⬜ **Custom themes.** 3 built-ins; no user theme file. `~/.config/fretboard/theme.json` with the existing 20+ token set (`kit/styles.go`) is a one-afternoon feature that makes the tool feel personal.
- 🧪 **Color support tiers.** 256-color vs truecolor vs 8-color fallbacks are
  untested; Confirm on a 16-color terminal.

---

### 2.4 Playback & practice

**Goal:** practice like a real tool: hear the tab, slow it down, loop it,
and never lose sync with the recording.

#### 2.4.1 MIDI engine
- ✅ Realtime step playback via fluidsynth (shell mode) with per-note
  sustain, noteoff scheduling, same-pitch re-articulation; SMF generation;
  BPM clamp + UI; soundfont resolution (config → env → auto-discover).
- ⬜ **Metronome.** IDEA Phase 2 promised `m`. A click track (MIDI note 37)
  layered on the schedule with a beat-accent option. Essential for practice;
  trivial on top of `BuildSchedule`.
- ⬜ **Count-in.** `C` = 1–2 bars of metronome before playback starts —
  otherwise the user can't play along from a cold start.
- ⬜ **Instrument/program.** Hardcoded GM program 25 (steel guitar). Confirm:
  a picker (acoustic/nylon/electric/bass) that re-sends `prog` — 5 lines of
  MIDI, big feel difference.
- ⬜ **Tempo-change support in ASCII tabs.** `rit.`/`accel.` markers and
  mid-tab BPM changes are ignored (GP `ColumnTicks` only). With sync points
  the audio path already tolerates this; MIDI-only playback doesn't.
- 🧪 **MIDI export UI.** `WriteSMF` exists; no key writes a `.mid` file.
  Confirm: `m` conflict with metronome — use `E` = export MIDI to the tab's
  directory.

#### 2.4.2 Audio backing
- ✅ Local files (tab dir, `audio_search_paths`, config audio dir, filename
  matching), online search + download via yt-dlp (12 s search / 10 min
  download timeouts, cached in `~/.config/fretboard/audio/`), source picker
  with strict-mode badges (`[official]` `[live]` `⛔ not studio` `★`),
  pitch-preserving rate control (`>`/`<`, 0.25×–4×, ffmpeg atempo / mpv
  speed), ffplay→mpv→mpg123 fallback chain, seek/restart.
- 🧪 **Filename pairing.** `FindAudio` matches *normalized-exact* names only;
  "Dire Straits - Sultans of Swing (Official Audio).mp3" does **not** match.
  Confirm: relaxed matching (contains artist + contains title, first hit
  wins, shown in the picker as `[local]`).
- ⬜ **Audio cache management.** Downloads accumulate forever with no UI.
  Confirm: picker footer shows cache size; `c` in picker = "clear cache for
  this song" (keep currently selected).
- ⬜ **Playlist mode.** After a tab ends (audio file finished), auto-advance
  to the next library row? At minimum Confirm: restart-at-cursor vs
  stop-at-end is currently stop (fine), but there's no "replay from bar 1".
- 🧪 **Rate + sync interplay.** `Elapsed()` is rate-aware (BUG-021 fixed), but
  Confirm: changing rate mid-loop keeps the A–B region anchored in *file
  time* (it should — `RestartAt` preserves `audioBase`).

#### 2.4.3 Practice tools
- ✅ A–B loop (MIDI wrap + audio seek, pre-play arming, grid highlight,
  failure banner), slow-down, follow-scroll.
- ⬜ **Step-through.** `,`/`.` are taken by fine sync nudges — use `[`/`]` is
  taken too… propose `n`/`N`-free key: `tab` = advance one schedule step,
  `shift+tab` = back one. For transcribing solos note-by-note.
- ⬜ **Anchor editing** (see 2.5).
- ⬜ **Practice stats.** Per-tab: sessions, minutes practiced, loop count —
  the home screen already has play counts; extend to "last practice
  duration" and show a streak on home.
- ⬜ **Performance mode.** Hide the tab, show only section names + progress
  bar, so you actually play from memory. (Nice-to-have, cheap to build on
  the section index from 2.3.)

---

### 2.5 Sync & calibration

**Goal:** the cursor is *never* visibly wrong, and fixing it takes one
keystroke.

- ✅ Everything in the sync toolkit: offset calibration (`[ ] { } , . o`),
  per-source calibration keys (`audio_offset:<id>`, `sync_points:<id>` with
  legacy fallback), leading-silence intro detection (ffmpeg `silencedetect`,
  auto marker, never re-runs), sync anchors (`s`/`S`), tick-density
  interpolation (`segmentStep`), `SegmentBPM`/`TicksBetweenBars`,
  tempo-map + drift indicators in the status row, offset-aware BPM
  derivation, loop-armed-at-start, honest error surfacing.
- ⬜ **Per-anchor editing.** Today: `S` clears *all* anchors. A wrong anchor
  (mistimed `s` press) poisons every subsequent segment. Confirm: `S` =
  undo last anchor, `Shift-S` twice = clear all; show anchor list with
  per-anchor bars+times on a dedicated panel.
- ⬜ **Anchor undo/redo** for nudges too — `o` resets the offset with no
  way back; make `o` undo-able (keep previous value, `o` again restores).
- ⬜ **Beat-level fine sync.** `,`/`.` at 0.1 s is good; at fast tempos
  (160 BPM, 0.375 s/beat) a 50 ms nudge key helps. Confirm: `[`/`]` hold-
  repeat (key repeat) instead of a new key.
- 🧪 **Sync validation.** Drift RMS is shown (`±2.0s`) but there's no
  "good/bad" reading. Confirm: color the drift (green < 0.3 s, yellow < 1 s,
  red above) and add a `?`-style hint when anchors disagree wildly
  ("anchor 4 contradicts anchors 2–3 — likely mistimed").

---

### 2.6 Personalization & configuration

- ✅ Config file (theme, ug delay, import path, volume, soundfont, audio
  paths, auto-fetch, strict mode), atomic writes, corrupt-file recovery,
  env vars, theme cycling persisted.
- ⬜ **In-TUI settings screen.** Today: change theme (`t`) or edit JSON by
  hand. A `S`-free key (`ctrl+s`?) settings panel for volume, strict mode,
  BPM default, rate limits, and paths — with the config file staying the
  source of truth.
- ⬜ **Volume control key.** Volume is config-only; playback volume can't be
  adjusted live. Confirm: `v`-free key (`0`–`9`? no…) — propose `g`-free
  `=`/`-` are BPM… propose `[volume]` keys `u`/`d`? All taken. This is
  exactly the kind of key-collision decision that needs Confirmation.

---

### 2.7 Platform & robustness

- ✅ Cross-platform process handling (reaper, process trees, Windows exit
  codes), zombie-free, idempotent shutdown, SIGTERM-clean, WAL, atomic
  config, timeouts everywhere, hermetic fake-bin tests, unix-gated tests
  run in WSL2.
- ⬜ **Crash logging.** A panic in the TUI prints a raw stack and exits.
  Confirm: recover + write `~/.config/fretboard/crash.log` + "please report"
  message (Bubble Tea panics are catchable in `Update`).
- 🧪 **GP parser missing.** Fetching a `.gp5` without the Rust binary —
  Confirm the error message names `FRETBOARD_GP_PARSER` and the
  `tools/gp-parser` build path (the README documents it; the error path is
  untested).
- 🧪 **Offline mode.** All network ops time out and error cleanly; Confirm
  the app is fully usable offline with a local library (search screen should
  say "offline" instead of spinning).

---

### 2.8 Cross-cutting UX

- ✅ Help screen with full keymap + sync workflow, footer hints with
  status segment, breadcrumbs, empty states with next actions, feedback for
  every key press (the BUG-045 family), honest errors everywhere.
- ⬜ **First-run onboarding.** Confirm the happy path for a brand-new user:
  empty library → home shows import hint (done) → but nothing tells them
  "fluidsynth not found — MIDI playback will be silent" until they open a
  tab. Home's `audioWarning()` covers part of this; Confirm it appears
  before first playback.
- ⬜ **Keybinding conflicts registry.** With ~40 keys, collisions are now
  the main UX risk (see 2.6 volume). Confirm: a generated table in `help.go`
  and a test that fails on duplicate key assignment across screens.
- ⬜ **i18n** — out of scope for 1.0, but all user-facing strings are
  currently hardcoded English; keep them in one place (`kit`/`msgs`) so a
  strings table can be added later.

---

## 3. Status matrix — draft vs current codebase

| Domain | Feature | Status | Where today |
|---|---|---|---|
| **Import** | file/dir import, skip-bad-files | ✅ | `internal/cli/cli.go`, `library/crud.go` |
| | recursive watch of `auto_import_path` | 🟡 top-level only | `watcher/watcher.go` (`fsnotify.Add(dir)`) |
| | URL import | ⬜ | — |
| | content dedup / "update vs keep both" | ⬜ | — |
| **Browser** | filter/sort/favorite/delete/preview | ✅ | `ui/browser/browser.go` |
| | metadata editing | ⬜ | — |
| | favorites/tuning/source filters | ⬜ | — |
| | playlists / practice sets | ⬜ | — |
| | export to file / clipboard | ⬜ | — |
| **Search** | 4 sources, badges, dedup, degraded retries | ✅ | `scraper/*`, `ui/search` |
| | rating/votes displayed + sorted | 🟡 parsed, unused | `scraper/ug.go` → `SearchResult{Rating, Votes}` |
| | official-version flag + filter | ⬜ | — |
| | pagination, history, offline cache | ⬜ | — |
| **Viewer** | grid/linear, follow, pan, jump, loop highlight | ✅ | `ui/kit/render.go`, `ui/viewer` |
| | search within tab | ⬜ | — (IDEA Phase 1, never built) |
| | sections (verse/chorus) | ⬜ | parser flattens; no `Section` field |
| | repeats/endings in playback | ⬜ | `BuildSchedule` walks bars linearly |
| | transpose, note-name view | ⬜ | `Tuning.Semitone` exists but unused |
| | GP track chooser | 🟡 first guitar track only | `tools/gp-parser/src/main.rs` `pick_guitar_track` |
| | chord-sheet mode | ⬜ (rejected cleanly) | F9 backlog in BUGS.md |
| | mouse wheel | ⬜ | no `tea.WithMouse…` |
| | custom themes | ⬜ | 3 built-ins only |
| **Playback** | realtime MIDI + sustain, SMF | ✅ | `player/realtime.go`, `smf.go` |
| | metronome / count-in | ⬜ | — (IDEA Phase 2 promised `m`) |
| | instrument program choice | ⬜ hardcoded prog 25 | `realtime.go` |
| | audio backing (local/online/strict/rate) | ✅ | `player/audio*.go`, `engine.go` |
| | relaxed audio filename pairing | 🟡 exact-normalized only | `audio.go` `normalizeAudioName` |
| | audio cache management | ⬜ | downloads accumulate |
| | step-through playback | ⬜ | — |
| | practice stats / performance mode | ⬜ | — |
| **Sync** | offset/nudges/per-source/intro/anchors/tempo-map/drift | ✅ | `player/audio_sync.go`, `intro.go`, `viewer.go` |
| | per-anchor undo/edit, nudge undo | ⬜ | `S` clears all; `o` unrecoverable |
| | drift color/hints | ⬜ | raw numbers only |
| **Config** | file/env/atomic/corrupt-safe | ✅ | `config/config.go` |
| | in-TUI settings, live volume | ⬜ | — |
| **Robustness** | cross-platform, reaper, timeouts, idempotent shutdown | ✅ | `player/process_*.go`, `watcher` |
| | crash log, offline UX, GP-missing message | 🧪 | untested paths |
| **Docs** | README/GUIDE/BUGS/user-stories current | ✅ | — |

**Backlog status note:** every item in `docs/user-stories.md`'s roadmap
(F1–F8) and every blocker bug (BUG-015…053) is now done. The tables above
are the next backlog.

---

## 4. Invisible requirements — "not features, but must be Confirmed"

These are behaviors users notice within the first minute, that no feature
list captures, and that only an implementation can confirm. Each needs an
acceptance test. The precedent for this discipline is the whole BUGS.md
tracker: `q` in a text input quitting the app, `_` wildcarding search,
byte-sliced UTF-8 — none were "features", all had to be found by driving the
real TUI. The same rigor now applies to the following.

### 4.1 The covers-and-garbage problem (search ranking) — *highest priority*

**Scenario:** user opens search, types "sultans of swing". What they want is
the official Dire Straits tab. What they may get: row 1 = a random
guitaretab.com user tab, row 5 = the official UG tab with 4.9★/2000 votes —
because results are merged in source order and `Rating`/`Votes` are parsed
but never used.

**Why it's invisible:** ranking is not a screen or a key; it's the default
merge order. Nobody files a bug ticket for "the app works, but shows me the
wrong tab first". They just stop using it (or worse, import the wrong tab
and think the app is bad).

**Must be Confirmed (implementation + tests):**
1. `Rating`/`Votes` shown on every UG row (`★★★★☆ 4.9 · 2.1k`).
2. Merge sorts: `Type == Tabs` first, then rating (votes ≥ threshold),
   then source trust order (UG > Songsterr > GT.cc > GuitareTab), *then*
   title. Covers by unknown authors sink unless the query matches only them.
3. An `official` flag: UG tab whose artist name == searched artist AND
   rating high → `★ official` badge; one key (`f`) filters to it.
4. Test: a fixture where a low-rated cover precedes the official tab in raw
   source order must render the official tab first.

The audio side already solved the identical problem (`ClassifyAudioCandidate`
+ `strict_audio_selection`). The tab side is the same disease, untreated.

### 4.2 "Is this a tab or a chord sheet?"

**Scenario:** user searches a song that only exists as chords on UG. Row
says "Chords" in dim gray at the end. They press Enter → "chord-only pages
are rejected" error. Correct behavior, wrong timing: the error should have
been visible *before* the fetch.

**Must be Confirmed:** type badge (`TAB`/`CHD`) at row start, tabs sorted
before chords, and a footer hint "chords only — will render as lyrics" when
the selected row is a chord sheet (or, with F9, actually render it).

### 4.3 "Why is there no sound?" (first-run audio check)

**Scenario:** fresh install, no fluidsynth, no yt-dlp. User opens a tab,
presses Space, hears nothing, sees "MIDI engine stopped early" or a
fluidsynth error only after playback fails.

**Must be Confirmed:** the warning currently exists on the *home* screen
(`audioWarning()` in `home.go`) but (a) shows only the first missing item
(soundfont → synth → yt-dlp, early return) and (b) never appears in the
viewer. Extend it to list all missing pieces and surface it at first `Space`
in the viewer, then test the exact first-run sequence end-to-end.

### 4.4 "The tab doesn't match my MP3" (filename pairing)

**Scenario:** user drops `Sultans of Swing (Live 1984).mp3` next to the tab.
`FindAudio` normalizes to `sultansofswinglive1984` ≠ `sultansofswing` →
not found → auto-downloads something instead of using the file they
explicitly placed there.

**Must be Confirmed:** relaxed matching (filename contains title AND
artist tokens), prefer exact over relaxed, and always *show* the matched
file in the picker as `[local]` so the user can see the pairing. A
"no local match — here's why" hint when a same-artist file exists but
didn't match.

### 4.5 "It played the chorus twice but the cursor didn't loop"

**Scenario:** any tab with `|: :|` repeats. MIDI playback walks bars
linearly; after the first repeat the cursor is one section ahead of the
music. No error, no crash — just quietly wrong. This is the single worst
playback-correctness gap.

**Must be Confirmed:** repeat-aware schedule expansion (repeat bars
*in the schedule* so the cursor visits them twice, mirroring how a human
reads the tab), repeat markers drawn on bar headers, and a `BuildSchedule`
unit test with a `|: A :| B` fixture asserting bar visit order.

### 4.6 "I pressed Enter and it fetched, but I can't tell if it's good"

**Scenario:** fetch succeeds, tab opens, viewer shows bars. The user has no
idea whether this tab is the 4.9★ official one or a 1★ mess until they play
it.

**Must be Confirmed:** the fetched source's rating/type/source badge is
carried into the library row (`online://` rows show `[UG] ★4.9` in the
browser) and into the viewer title line. One field in `Tab.Metadata`
(`source_rating`), shown in two places.

### 4.7 "My settings changed but the app forgot"

**Scenario:** user cycles theme with `t` — persisted (good). User sets
volume in config — but there is no volume key at all, and BPM changes are
never persisted. After closing the app, BPM resets to 120.

**Must be Confirmed:** which session state persists (BPM per tab? volume?)
and which doesn't, documented on the help screen. Silence = users assume
the app is buggy when a value "disappears".

### 4.8 "The watcher ignores my subfolders"

**Scenario:** `auto_import_path: ~/tabs` with `~/tabs/rock/*.txt`. Files
drop in, nothing happens. The watcher only registers the top directory.

**Must be Confirmed:** recursive registration (`fsnotify` per-subdir or a
walk + re-scan), with a test that drops a file two levels deep.

### 4.9 "I fat-fingered a sync anchor and now it's all drifting"

**Scenario:** user presses `s` at the wrong moment once. Every segment
after it is shifted. `S` clears everything (losing 20 good minutes of
calibration); there is no undo-last.

**Must be Confirmed:** `S` = undo last anchor (with a hint "S again =
clear all"), nudge undo for `o`, and drift coloring (2.5).

### 4.10 "The audio stopped and I don't know why"

**Scenario:** the backing track ends mid-song (short radio edit). Cursor
pins, playback stops, no message. The user thinks the app crashed.

**Must be Confirmed:** "track ended (4:12) — press Space to restart from
bar 1" banner, and a decision: stop vs auto-advance to next library song
(2.4.2 playlist mode).

### 4.11 "The cover I picked on purpose keeps getting skipped"

**Scenario:** strict mode is on; user opens the picker and *deliberately*
selects the live version. It plays (manual pick overrides strict — good),
but the moment the catalog refreshes (`r`), auto-pick snaps back to
strict-compatible. Confirm: manual selection is sticky for the session
even across catalog refreshes.

### 4.12 "Nothing happens when I press keys" — the discoverability audit

**Scenario:** new user, viewer open. The footer shows 13 hints; the help
screen lists ~40 keys. Keys that exist but are absent from both: none
anymore (BUG-045 fixed the last), but the audit must be *ongoing*: every
new key must land in footer + help + a collision test (2.8).

---

## 5. Roadmap waves

Priorities by (user pain × frequency) / (implementation cost).

| Wave | Contains | Rationale |
|---|---|---|
| **R1 — Trust the results** | 4.1 ranking + ratings, 4.2 type badges, 4.6 source badge on import, 4.3 first-run audio check | Search and playback are the front door; wrong-first-result is the #1 trust killer. Small, well-scoped, testable. |
| **R2 — Playback correctness** | 4.5 repeats/endings in `BuildSchedule`, 4.10 end-of-track banner, metronome + count-in, step-through, instrument program picker | The "it plays wrong" family. Repeats is the biggest single parsing/playback feature. |
| **R3 — Library depth** | metadata editing, filters, playlists, export/clipboard, session resume, relaxed audio pairing (4.4), cache management | Turns a viewer into a practice tool. |
| **R4 — Reading power** | search-in-tab, sections + minimap, transpose, note-name view, GP track chooser, chord-sheet mode, mouse wheel, custom themes | The power-user wave; each item is independent. |
| **R5 — Hardening** | 4.7 persistence audit, 4.8 recursive watcher, 4.9 anchor undo, 4.11 sticky manual pick, crash log, DB repair, offline UX, settings screen | Small confirm-and-fix items; most are acceptance tests + a few lines. |

Suggested first slice (one focused PR-sized chunk):
1. `SearchResult` ranking merge + rating display + official flag (R1)
2. Repeats in `BuildSchedule` + bar-header markers (R2)
3. Anchor undo + drift coloring (R5)
4. The tests that Confirm each — in the spirit of BUGS.md's
   "test first, red → green, then FIXED".
