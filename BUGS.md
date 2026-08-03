# Fretboard Bug Tracker

Live bug registry maintained by the main agent. Each bug has an owner subagent,
a test that reproduces it, and a fix. Mark `STATUS: FIXED` only after the test
passes and the full package test suite is green.

Module root: `/home/gio/Projects/fretboard`
Test commands: `go test ./...` (repo) and `cd tests && go test ./...` (e2e module).

---

## BUG-017 — UG fetch broken: "ug html: data-content not found" — FIXED

**Symptom:** opening a search result fails with `Could not fetch tab: ug html: data-content not found`.

**Root cause:** Ultimate Guitar moved from the ID-based URL pattern
(`/tab/_/_tabs_<id>`, now 404) to slug-based pages
(`/tab/<artist>/<song>-<type>-<id>`). The HTML fallback fetched the dead
pattern, got a 404 page with no `data-content` attribute, and errored.
Additionally, UG search responses now include official/TabPro marketing rows
(no `type`, no `id`) that were being surfaced as fetchable results.

**Fix:** `SearchResult.TabURL` captured from search JSON; `ugTabURL` builds the
slug URL as a fallback (with `slugify`); official rows filtered out (only
Tabs/Chords with an id); `Fetch` now takes the result, checks HTTP status, and
rejects chord-only pages with a clear error instead of importing an empty tab.

**Tests:** `TestUGTabURLSlugPattern`, `TestSlugify`,
`TestUGHTMLSearchFiltersNonPublicResults`, live integration
`TestLiveUGSearchAndFetch` (`go test -tags live`).

## BUG-018 — Chord-only UG pages parse into 0-bar tabs — OPEN (rejected cleanly)

**Symptom:** fetching a Chords-type result yields a tab with no bars
(title/artist garbage from chord lines).

**Root cause:** the parser expects tab notation; chord sheets (lyrics + `[ch]`
marks) produce no bar structure.

**Current behavior:** rejected with a clear error (no garbage import).
**Fix path:** chord-sheet support → backlog F9.

---

## BUG-001 — `j`/`k` keys are dead in the library browser

- **Area:** `internal/ui/browser/browser.go`
- **Status:** FIXED (handleNormalKey now handles KeyDown/KeyUp; test TestBrowserJKMoveCursor; verified live)
- **Confirmed:** Yes (driven live TUI: `enter` then `j,j,j` leaves cursor on row 0;
  `down,down,down` moves it. README, footer `[j/k]move`, and help screen all claim j/k move.)
- **Root cause:** `BrowserModel.handleNormalKey` (browser.go) only handles `"down"`/`"up"`;
  `"j"`/`"k"` fall through and do nothing. `KeyDown`/`KeyUp` constants exist in
  `keymap.go` but are unused. The viewport is never updated for keys.
- **Repro:** `fretboard` → Enter (Library) → press `j` a few times → cursor does not move.
- **Expected:** `j`/`k` move the cursor exactly like `↓`/`↑` (and like the Home screen and Viewer).
- **Fix scope:** `handleNormalKey` only. Add `"j"`/`"k"` cases (use `KeyDown`/`KeyUp`).
- **Test:** add `j`/`k` movement assertion to `internal/ui/browser/browser_test.go`.

---

## BUG-002 — typing `q` while focused on a text input quits the whole app

- **Area:** `internal/ui/search/search.go` (online search query), `internal/ui/browser/browser.go`
  `handleSearchKey` (library filter)
- **Status:** FIXED
- **Confirmed:** Yes (pty test: library → `o` → type `q` → process exits rc=0;
  library → `/` → type `q` → process exits rc=0. Buttons `u` after `q` do nothing.)
- **Root cause:** Both `SearchModel.Update` and `BrowserModel.handleSearchKey` switch on
  `KeyQuit`/`KeyQuit2` (`"q"`, `ctrl+c`) *before* forwarding printable keys to the focused
  text input. So `q` inside a query quits instead of inserting the letter.
- **Repro:** `fretboard` → Enter → `o` → type `s` then `q` → app quits (query "sq" impossible).
- **Expected:** while a text input is active, `q` is just a character. Quit should only fire
  when the input is not being edited (and `ctrl+c` may remain a hard quit everywhere).
- **Fix scope:** in `search.go`, only quit when `!m.inputActive` (query not focused) OR keep
  `ctrl+c` as hard quit; in `browser.go handleSearchKey`, only quit when not actively typing
  (e.g., treat `q` as a character like other printable keys). Do NOT touch `handleNormalKey`
  (that's BUG-001's file region).
- **Test:** add cases typing `q` into the search query and library filter to the relevant
  `*_test.go` files.

---

## BUG-003 — `fretboard import <dir>` aborts on the first unparseable `.txt`

- **Area:** `internal/library/crud.go` `ImportDirectory`
- **Status:** FIXED
- **Confirmed:** Yes (code: `filepath.Walk` returns on the first `ImportFile` error, killing
  the whole walk; a bad `.txt` stops all later imports).
- **Root cause:** `return fmt.Errorf("import %s: %w", path, err)` on line ~249.
- **Repro:** dir with `good.txt`, `bad.txt` (garbage that fails `parser.ParsePath`), `good2.txt`
  → `fretboard import dir/` imports `good.txt` then fails; `good2.txt` never imported.
- **Expected:** skip unparseable files with a warning and keep importing the rest; only return
  an error if nothing could be imported (or aggregate a summary).
- **Fix scope:** `ImportDirectory` walk callback only.
- **Test:** add a case in `tests/e2e/import_test.go` with a mixed dir asserting the good files
  still import.

---

## BUG-004 — `lipglossWidth` uses byte length, not display width

- **Area:** `internal/ui/kit/render.go` (`lipglossWidth`), used by `RenderStatusBar`
- **Status:** FIXED
- **Confirmed:** Yes (code: `func lipglossWidth(s string) int { return len(s) }`).
- **Root cause:** byte length ≠ terminal cell width for wide/multibyte runes (CJK, emoji,
  accented chars); the status bar spacer math misaligns.
- **Expected:** use `lipgloss.Width(s)` (or `ansi.StringWidth`) so alignment is correct.
- **Fix scope:** `render.go` `lipglossWidth` only. Do not rename the function (other call sites).
- **Test:** a unit test in `internal/ui` asserting wide-rune width is not byte length.

---

## BUG-005 — `truncate`/`truncateErr` slice by byte index, splitting UTF-8 runes

- **Area:** `internal/ui/home/home.go` `truncate`, `internal/ui/viewer/viewer.go` `truncateErr`
- **Status:** FIXED
- **Confirmed:** Yes (code: `s[:max-1]` byte slicing; can cut a multibyte rune mid-sequence,
  producing a broken/box glyph in titles and error messages).
- **Root cause:** byte-index slicing on strings that may contain non-ASCII.
- **Expected:** truncate on rune boundaries (e.g., `[]rune(s)`, or `utf8`-safe width cut) and
  keep the trailing `…`.
- **Fix scope:** both functions (they are unexported; viewer has its own copy).
- **Test:** unit test in `internal/ui` with a multibyte title asserting no replacement char.

---

## BUG-006 — realtime MIDI cuts every note at each step boundary (sustain ignored)
- **Status:** FIXED (PlaybackStep.Sustain from BuildSchedule; scheduled noteoff goroutines with generation+epoch guards; tests TestBuildSchedulePopulatesSustain, TestPlayStepSustainsNotesUntilStop)

## BUG-007 — backspace in library filter splits UTF-8 runes
- **Status:** FIXED (rune-safe backspace in browser.go handleSearchKey; test TestBrowserBackspaceRemovesFullRune)

## BUG-008 — Home screen stat boxes overflow narrow terminals
- **Status:** FIXED (stat row now shrinks/stacks to fit width; test TestHomeStatRowFitsAvailableWidth)

## BUG-009 — stale AppModel copy + deadlock on external SIGTERM
- **Status:** FIXED (removed main's duplicate signal handler; bubbletea's own handler now delivers the quit, and main calls Shutdown() on the live model returned by p.Run(). Also fixed pre-existing deadlock: main's p.Send + bubbletea's unguarded send on its unbuffered msgs channel deadlocked Program.shutdown. Test TestAppShutdownMsgQuits. Verified 10/10 SIGTERM exits in 0.1s.)

## BUG-010 — library.Search treats `_` as a wildcard (not escaped)
- **Status:** FIXED (escape `_` and `\` in LIKE pattern; test TestSearchEscapesUnderscore, red→green proven)

## BUG-011 — `d` deletes a tab permanently with no confirmation/undo
- **Status:** FIXED (delete now arms a `Delete "title"? [y]es [n]o` prompt; test TestBrowserDeleteRequiresConfirmation; `d` hint added to footer)

## BUG-012 — theme cycle (`t`) order is random (map iteration)
- **Status:** FIXED (ThemeNames() now returns a stable order; tests TestThemeNamesDeterministic, TestCycleThemeIsStable)

## BUG-013 — audio catalog search runs yt-dlp with no timeout; auto-fetch can hang forever
- **Status:** FIXED (ytSearch 12s + DownloadYouTubeAudio 10m via exec.CommandContext; tests TestYTSearchTimeoutKillsHangingYtDlp, TestDownloadYouTubeAudioTimeoutKillsHangingYtDlp — red→green proven, hermetic fake-yt-dlp)

## BUG-014 — UG album-page results import garbage tabs ("Track 02 - Lady Writer - Not Included - Anyone?")
- **Status:** FIXED (ug.go Fetch rejects `Part=="album"` + track-listing content detection via `isAlbumTab`; `ugScraper` interface for fake-injected tests TestFetchRejectsAlbumPart / TestFetchAcceptsRegularTab / TestIsAlbumTabDetectsTrackListing; verified live against UG id 50558 — now errors instead of importing 289 garbage bars)

## CANDIDATES — under investigation by main agent (not yet assigned)

<!-- REPRO NOTES (main agent):
- TUI driver scripts at /tmp/drive_fretboard.py and /tmp/check_exit.py (python3 pty).
- DB backed up at /tmp/fretboard.db.bak (restore with:
  cp /tmp/fretboard.db.bak ~/.config/fretboard/fretboard.db)
- Build: go build -o /tmp/fretboard-test ./cmd/fretboard
-->

## Architecture refactor (2026-08)

Not a bug — a structural cleanup performed after the bug hunt. Commits:
- `P1+P4`: BPM helpers moved `player` → `model` (kills scraper→player layering violation); scraper rate-limit sleep deduped into `ratelimit.go`; metadata keys are `model.MetaKey*` constants; dead code removed (`FetchID`, `NewAppWithStore`, `BeatDuration`, unused `difficulty`/`tags` schema columns); config `AutoFetchAudio` is a plain bool with default-`true` unmarshal; `library.ErrNotFound` replaces sentinel errors.
- `P2`: `internal/tui` split into `internal/ui/{kit, msgs, home, browser, search, viewer, help, app}`; viewer split into `viewer.go` / `viewer_playback.go` / `viewer_audio.go`; router pokes screens only via a small exported API.
- `P3`: `internal/cli` with testable `Run(args, stdout, stderr) int`; `cmd/fretboard/main.go` is a 10-line wrapper.
- Final polish: module path `github.com/YOUR_USERNAME/fretboard` → `fretboard` (also `tests/go.mod`); dead exports removed (`SetAllowOnline`, `kit.KeySearch`); gofmt across the repo; docs updated.

Non-blocking candidates that remain open (from the bug hunt):
- Online-search album pages (multi-track UG results) are rejected with a clear error instead of importing mangled tabs.
- Audio-sync cursor uses linear time mapping; it drifts slightly for tabs with heavy rhythm variation.

## Audio-sync / rendering bugs (round 2 — user-reported issues)

- **BUG-015 — `DeriveBPMFromAudio` ignores the calibrated `audio_offset`.** It scales the whole audio file to the whole tab, so with an intro (offset > 0) the derived BPM is underestimated by `offset`. Fix: derive using `audioDur - offset`. **Status:** FIXED (offset-aware derivation + test).
- **BUG-016 — `maxPanOffset` assumes a linear strip** (uses `maxBarColumns`), so `h`/`l` panning in the grid layout can overshoot for padded bar columns. Fix: pan against grid columns. **Status:** FIXED (grid width from `BarGridMetrics`, pan only when the grid overflows).

## View/audio features delivered (round 2)

- Page-layout bar grid (`kit.RenderTabGrid`): bars flow left-to-right, wrap, and pad to fill the terminal; adaptive column width; grid-aware scroll math. (US-1)
- Schedule-time audio mapping (`player.StepIndexAtScheduleTime`): playhead follows the tab's own rhythm durations instead of a linear fraction of the audio. (US-2)
- Audio offset calibration: `[` `]` ±0.5 s, `{` `}` ±5 s, `o` reset; persisted per tab via `audio_offset` metadata; live re-mapping. (US-3)
