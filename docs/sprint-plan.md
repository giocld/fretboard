# Fretboard sprint plan — feature groups (from docs/finished-product.md)

Execution model: one group per iteration. For each group: implement subtasks
in order → unit tests → build + full test suite → drive the CLI/TUI to verify
behavior → commit with a simple message → move to the next group.

Priority is value-per-effort × user-visible trust. Within a group, subtasks
are ordered so earlier ones unblock or de-risk later ones.

---

## S1 — Search result trust (R1) — *highest priority*

*Users must see the right version first: official/top-rated tabs above
covers and chord sheets.*

- [x] S1.1 Show UG rating/votes on search rows (`★ 4.9 · 2.1k`)
- [x] S1.2 Rank merged results: tabs first, then rating/votes, then source
      trust (UG > Songsterr > GuitarTabs > GuitareTab); dedup keeps the
      highest-rated row per source
- [x] S1.3 Type badge on rows (`TAB`/`CHD`/`BASS`) so chord sheets are
      recognizable before fetching
- [x] S1.4 `★ top` badge for high-rated tabs matching the query's artist
- [x] S1.5 Carry source + rating into the library (`source_badge` column,
      migration) and show it in the browser row and viewer status
- [x] S1.6 Tests: ranking order fixture, rating formatting, merge dedup,
      import badge persistence
- [x] S1.7 Drive the TUI search screen (pty) with a stubbed client; verify
      badges/order render — **done via live test** (no pty on this Windows
      host): `TestLiveSearchRankedOfficialFirst` searches the real 4 sources
      and asserts the official UG tab ranks #1

**Done =** a search result list where the official tab precedes the
low-rated cover, chord sheets are labeled, and the rating survives into the
library.

---

## S2 — Playback correctness: repeats + end-of-track (R2)

*The cursor must follow the music even when the tab repeats, and the app
must say when the recording ends.*

- [x] S2.1 Parse repeat markers (`|:`, `:|`) and 1./2. endings into
      `model.Bar` fields — marker digits are stripped from note content so
      they never play as frets
- [x] S2.2 `BuildSchedule` expands repeats so MIDI playback + cursor visit
      bars in human reading order (incl. `1.` → skip to `2.` on second pass)
- [x] S2.3 Repeat markers drawn on bar headers in grid + linear layouts
- [x] S2.4 End-of-track banner: audio file finished → "Track ended (4:12)
      before the tab finished — Space restarts from this bar" instead of
      silent stop
- [x] S2.5 Tests: repeat parsing fixture, schedule expansion order,
      header rendering, end-of-track message
- [x] S2.6 Drive MIDI playback on a repeated tab — no pty on this Windows
      host, so verified through the real CLI code path: parsed a repeated
      tab with the shipped parser, printed `RepeatOrder`/`BuildSchedule`
      bar order (1 2 3 → 1 2 4 → 5) and the rendered grid headers with
      `│:`, `1.`, `2.`, `:│`

---

## S3 — Practice kit: metronome, count-in, instrument (R2)

*Practice tools that every guitar app has and this one still lacks.*

- [x] S3.1 Metronome click (GM 37) on beat boundaries during MIDI playback,
      `m` toggles (footer + help) — accented on the first beat of each bar,
      derived from the bar's own tick durations (`BeatColumns`)
- [x] S3.2 Count-in: `C` cycles 0→1→2 bars of lead-in clicks before the
      tab starts (blocks the playback cmd until done, then the schedule
      begins)
- [x] S3.3 Instrument program picker (`y` cycles steel/nylon/clean/
      overdrive/bass) re-sends `prog` to fluidsynth at session start;
      restarting playback applies it live; status row shows the name
- [x] S3.4 Tests: beat-column derivation, click/off-beat/no-click through a
      fake fluidsynth, count-in click count + accent, program command, and
      a full viewer key-handler → real engine end-to-end test
- [x] S3.5 Drive playback — done headlessly: fake fluidsynth captures the
      real command stream (`prog 0 24` + accent click + count-in clicks)
      through the actual `m`/`C`/`y`/`Space` key path; no pty on this host

---

## S4 — Library depth (R3 subset)

*Fix what you can't rename, narrow what you can't search, share what you
read, and pair the MP3s you already own.*

- [x] S4.1 Edit title/artist from the browser (`e` two-step editor, empty
      input with current value as placeholder; empty Enter keeps the old
      value; warns that re-import overwrites) — rewrites row + content
- [x] S4.2 Favorites-only filter (`F`), combined with existing search
- [x] S4.3 Export tab to file + copy to clipboard (`x` browser / `X`
      viewer, plain ASCII via `kit.RenderTabPlain`, round-trip re-parses)
- [x] S4.4 Relaxed backing-track filename pairing (contains artist+title
      tokens, exact wins, shortest stem wins among contains matches)
- [x] S4.5 Tests: UpdateMeta round-trip, edit flow + empty-keep, favorites
      filter, exports (browser + viewer), plain render round-trip, relaxed
      pairing order
- [x] S4.6 Drive via CLI code path: paired a `(Live 1984).mp3`, exported
      `Sultans of Swing.txt` (clipboard copy confirmed on Windows) and
      re-parsed it — no pty on this host

---

## S5 — Reading power (R4 subset)

*Find the solo, play it in another key, see the notes.*

- [x] S5.1 Search within a tab (`/` — bar number or fret pattern, live
      match count, `n`/`N` cycle, Enter jumps and closes; match bar
      highlighted with SearchBarStyle; paste-friendly input)
- [x] S5.2 Transpose ±semitone (`T` up, `Z` down, `R` reset, ±12 clamp) —
      display and MIDI playback both use the transposed copy
      (`model.TransposedTab`), metadata untouched
- [x] S5.3 Note-name view (`e` toggles fret digits → note names via
      `Tuning.NoteNameAt`, same column width)
- [x] S5.4 Tests: transposed copy + clamp + metadata, note naming math,
      note-name render, search-highlight render, viewer key flows
      (transpose/playback, search/cycle/jump, notes toggle)
- [x] S5.5 Drive the viewer: `/` + `33` found the bar-2 riff, Enter jumped
      the cursor to bar 2, `T`+`e` showed `transpose +1 ♪ notes [33] 1/1`
      in the status row — no pty on this host

---

## S6 — Hardening (R5 subset)

*Small confirm-and-fix items that protect existing work.*

- [ ] S6.1 `S` removes the last sync anchor (undo); repeated presses remove
      more; help/footer updated; nudge `o` becomes undo-able (press again
      restores previous offset)
- [ ] S6.2 Recursive watcher: subdirectories of `auto_import_path` are
      watched too
- [ ] S6.3 Sticky manual audio pick: a manual picker selection survives
      catalog refreshes
- [ ] S6.4 Crash log: panic in the TUI writes `~/.config/fretboard/crash.log`
      and shows a friendly message
- [ ] S6.5 Tests: anchor undo order, nested watcher event, sticky pick,
      crash-log write
- [ ] S6.6 Full regression: `go test ./...` + `cd tests && go test ./...`

---

## Backlog (after S6, not yet sliced)

S7: search pagination/history/offline cache · S8: sections + minimap ·
S9: metronome-adjacent practice stats/performance mode · S10: GP track
chooser + chord-sheet mode · S11: mouse wheel + custom themes +
settings screen + volume key · S12: playlists, session resume, DB repair,
i18n prep.
