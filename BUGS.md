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

## BUG-019 — multi-bar rhythm rows time every bar with bar 1's rhythm
- **Status:** FIXED
- **Symptom:** in a chunk like `| q  e  | h  q  |` over two bars, bar 2's notes play with bar 1's q/e durations (480/240) instead of its own h/q (960/480).
- **Root cause:** `barsFromColumn` attached the chunk-wide rhythm marks verbatim to every bar; marks for bar 2 sat at chunk-absolute positions that could never match bar 2's bar-relative note columns, and bar 1's marks matched by coincidence.
- **Fix:** `rhythmForBar` filters marks to each bar's pipe-delimited range and rebases them to bar-relative positions (`internal/parser/bars.go`). Also fixed the linear-viewer header glitch uncovered while testing (BarNumberStyle had a fixed `Width(5)` that wrapped every header onto a second line).
- **Tests:** `TestMultiBarRhythmRowRebasesMarksPerBar`, `TestMultiBarRhythmRowTimesBar2ByItsOwnMarks` (parser); updated `TestRhythmTicksForNoteUsesNearestMark`/`TestRhythmAwareNoteSustain` (player) and e2e `TestRhythmAwareEvents` to the now-correct mark alignment.

## BUG-020 — "Dropped D"/"Open D"/"E Flat" tuning labels produce broken tunings
- **Status:** FIXED
- **Symptom:** `Tuning: Dropped D` was not matched by the `"drop d"` case; `NoteLetters("Dropped D Tuning")` → `"DD"` → a 2-string tuning, so every note on strings 3–6 was silently dropped during playback. "Open D" fell back to Standard (wrong pitches); "E Flat" was unhandled.
- **Fix:** named cases for "dropped d", "open d" (new `model.OpenD`), "e flat"; letter-based fallback now requires `len(cleaned) <= stringCount` and a successful full-length `ParseTuning` so garbage labels fall back to Standard instead of a truncated tuning (`internal/parser/tuning.go`).
- **Tests:** `TestInferTuningNamedVariants`, `TestInferTuningGarbageLabelsFallBackToStandard` (`internal/parser/tuning_test.go`).

## BUG-021 — `Engine.Elapsed()` ignores `audioBase` and playback rate
- **Status:** FIXED
- **Symptom:** pressing `>`/`<` during audio playback sought to the wrong position (at 2× for 30s the audio is at 60s but the restart used 30s), and the playhead cursor lagged/led the music at any rate ≠ 1.
- **Root cause:** `Elapsed()` returned raw `time.Since(playbackStart)`; `audioBase` (set on every restart) was never read.
- **Fix:** `Elapsed() = audioBase + wallSinceStart × rate` (`internal/player/engine.go`).
- **Tests:** `TestElapsedAccountsForAudioBaseAndRate` (`internal/player/engine_test.go`).

## BUG-022 — Unix: exited players become zombies and playback never auto-ends
- **Status:** FIXED
- **Symptom:** ffplay/fluidsynth exiting naturally (track or SMF ends) left a zombie; `kill(pid, 0)` reports zombies alive, so `PlaybackEnded()` never fired — the cursor pinned to the last step and the playing indicator stayed until Space. Zombies also blocked pid reuse and accumulated per session. Windows was unaffected (GetExitCodeProcess).
- **Fix:** `startReaper` waits on each child and records it as exited; `processAlive` consults the registry (keyed by `*exec.Cmd`); `killProcessTree` waits on the reaper instead of double-`Wait`ing (`internal/player/process_unix.go`). `startReaper` is a no-op on Windows.
- **Tests:** `TestReaperMarksNaturallyExitedProcessAsDead`, `TestReaperReportsRunningProcessAlive`, `TestKillProcessTreeWithReaper` (`internal/player/process_unix_test.go`, `!windows`; compiled for linux/darwin via GOOS vet).

## BUG-023 — sync points and A-B loops are one bar late; loops ignore the intro offset
- **Status:** FIXED
- **Symptom:** with 2+ sync points (`s`) the playhead tracked one bar late (anchors persisted 1-based user bars but `stepIndexAtBar`/`ScheduleTimeAtBar` compare against 0-based `step.Bar`). A-B loops fired 20s early with an `audio_offset` intro, restarted into the intro, and dropped the cursor to bar 1.
- **Fix:** viewer converts persisted 1-based bars to 0-based at the mapping boundary (`syncPointsZeroBased`), the MIDI tick wrap uses `step.Bar >= loopEndBar`, and loop start/end are computed in audio file time (schedule time + offset) so the monitor's `Elapsed()` comparison and `RestartAt` seek are consistent (`internal/ui/viewer/viewer.go`). Player tests updated to real 0-based schedules.
- **Tests:** `TestSyncPointsZeroBased`, `TestLoopStartTimeUsesZeroBasedBarPlusOffset`, `TestSetLoopPointMapsToFileTime` (viewer); `TestStepIndexAtSyncPoints`/`TestScheduleTimeAtBar` (player, corrected).

## BUG-024 — quitting the TUI with a watcher panics: `close of closed channel`
- **Status:** FIXED
- **Symptom:** with `auto_import_path` set, pressing `q`/`ctrl+c` closed the watcher, then `cli.go`'s post-`Run` cleanup called `Shutdown()` again on the same `*Watcher` → `close(w.done)` on an already-closed channel → panic + nonzero exit after the TUI exited.
- **Fix:** `Watcher.Close` is now guarded by `sync.Once` (`internal/watcher/watcher.go`).
- **Tests:** `TestWatcherCloseIsIdempotent` (`internal/watcher/watcher_test.go`).

## BUG-025 — UG API backend has no HTTP timeout (TUI can hang forever)
- **Status:** FIXED
- **Symptom:** `ultimateguitar.New()` returns a client with a zero timeout; a hung UG request blocked the search/fetch spinner and leaked its tea.Cmd goroutine (the HTML/Songsterr backends both set 30s timeouts).
- **Fix:** `newUGAPIClient` sets `s.Client.Timeout = 30s` (`internal/scraper/ug.go`).
- **Tests:** `TestUGAPIClientHasTimeout` (`internal/scraper/scraper_test.go`).

## BUG-026 — follow-scroll (`f`) is broken in the linear layout (`v`)
- **Status:** FIXED
- **Symptom:** in linear layout each bar is a full-width block, but `ensureCursorVisible` used grid-row math, so the playhead was never scrolled into view during playback.
- **Fix:** `kit.LinearBarLineOffsets`/`kit.GridBarLineOffsets` compute exact start lines mirroring the renderers; the viewer picks the active layout's offsets (`internal/ui/kit/render.go`, `internal/ui/viewer/viewer.go`).
- **Tests:** `TestLinearBarLineOffsetsMatchRenderer`, `TestGridBarLineOffsetsMatchRenderer` (kit) — offsets asserted against the actual rendered output line-by-line.

## BUG-027 — grid follow-scroll overshoots rows with mixed string counts
- **Status:** FIXED
- **Symptom:** `barGridLineOffset` priced every grid row at the global max `RowHeight`; in tabs with mixed string counts (multi-section tabs) the computed target overestimated the real line, scrolling the playhead off the top of the screen.
- **Fix:** grid offsets accumulate each row's actual height (`1 + stringsPerRow(row) + 1`) instead of the global max.
- **Tests:** covered by `TestGridBarLineOffsetsMatchRenderer` (mixed 1/6/2-string rows at widths 40/76/120).

## BUG-028 — footer hint bars overflow narrow terminals
- **Status:** FIXED
- **Symptom:** `RenderFooter` emitted the full hint list; on 80-col terminals the viewer/browser footers wrapped onto ~3 lines and pushed the panel body out of view.
- **Fix:** `RenderFooter` now drops hints from the middle (keeping first + last, so `q quit` survives) until the bar fits the width (`internal/ui/kit/chrome.go`).
- **Tests:** `TestRenderFooterFitsWidth` (`internal/ui/kit/chrome_test.go`) at widths 40–120; e2e TUI tests re-verified.

## BUG-029 — single-mark rhythm rows (`| h |`) are dropped entirely
- **Status:** FIXED
- **Symptom:** slow-ballad rows with one mark failed the `letters >= 2` check and were absorbed as a string line; the tab's written rhythm was ignored (timing fell back to column spacing).
- **Fix:** `looksLikeRhythmLine` accepts `>= 1` mark, guarded by a first-non-space `|` check so labeled string lines (`e|--|`) are never misclassified (`internal/parser/rhythm.go`).
- **Tests:** `TestSingleMarkRhythmRowParses`, updated `TestLooksLikeRhythmLine`.

## BUG-030 — rate limiter is not shared across scraper backends
- **Status:** FIXED
- **Symptom:** each backend (UG API, UG HTML, Songsterr) owned a private limiter and `Client.delay` was dead code; a single search fired back-to-back requests across sources, defeating the `ug_delay_ms` setting and inviting 429s.
- **Fix:** `NewClient` creates one `*rateLimiter` shared by all backends (`internal/scraper/{client,ug,ughtml,songsterr}.go`).
- **Tests:** `TestRateLimiterSharedAcrossBackends`.

## BUG-031 — corrupt config.json locks the app out; Save is non-atomic
- **Status:** FIXED
- **Symptom:** a truncated/partial config.json made `Load` hard-fail and every subcommand exit 1 with no recovery; a crash mid-write could create exactly that file.
- **Fix:** `Load` returns defaults + `ErrCorruptConfig` (CLI warns and continues); `Save` writes temp + rename (`internal/config/config.go`, `internal/cli/cli.go`).
- **Tests:** `TestLoadCorruptConfigReturnsDefaultsAndSoftError`, `TestSaveIsAtomicAndRoundTrips`.

## BUG-032 — tabs without a tuning render the literal text "null"/"[]"
- **Status:** FIXED
- **Symptom:** `json.Unmarshal` of `"null"` into a `Tuning` succeeds with an empty slice, so the browser row showed `null` and fuzzy search matched the text "null".
- **Fix:** `formatRowTuning` returns "" for empty tunings (`internal/ui/browser/browser.go`).
- **Tests:** `TestFormatRowTuningEmpty`.

## BUG-033 — search screen keeps the results-mode hint while editing the query
- **Status:** FIXED
- **Symptom:** `esc`/`up` back to the query box (and `tab` into results) never re-rendered the viewport, so the results-mode hint line and highlight stayed stale until the next keypress.
- **Fix:** `focusQuery`/`focusResults` now refresh (`internal/ui/search/search.go`).
- **Tests:** `TestSearchFocusSwitchRerenders`.

## BUG-034 — UG HTML search parses error pages as results
- **Status:** FIXED
- **Symptom:** `ugHTMLClient.Search` never checked the status code (Fetch did); on 403/429/5xx the error page surfaced as a misleading "ug html: data-content not found".
- **Fix:** non-200 → `status N` error before reading the body, in both UG HTML and Songsterr (`internal/scraper/ughtml.go`, `songsterr.go`).
- **Tests:** `TestUGHTMLSearchRejectsBadStatus` (httptest + base-URL seam).

## BUG-035 — fetched tab entity decoding is nondeterministic
- **Status:** FIXED
- **Symptom:** `normalizeContent` iterated a Go map, so double-encoded sequences like `&amp;quot;` decoded differently run to run.
- **Fix:** ordered replacement list (`&quot;`/`&#039;`/… before `&amp;`) (`internal/scraper/ug.go`).
- **Tests:** `TestNormalizeContentIsDeterministic`.

## BUG-036 — sub-millisecond sustains schedule 0 ms and never release
- **Status:** FIXED
- **Symptom:** `StepDuration(1, 126)` floored to 0 ms; `scheduleNoteOff` skipped the noteoff goroutine, so notes at high BPM rang forever; per-step flooring also drifted the audio-sync cursor.
- **Fix:** `StepDuration` rounds up (`internal/player/rhythm.go`).
- **Tests:** `TestStepDurationRoundsUp`.

## BUG-037 — MIDI loop ending on the last bar never wraps
- **Status:** FIXED
- **Symptom:** the tick wrap fired only when a step landed beyond the loop end bar; with B on the final bar no later step exists, so playback stopped at the end instead of looping.
- **Fix:** wrap also on schedule end (`atEnd || beyondLoop`) (`internal/ui/viewer/viewer.go`).
- **Tests:** `TestMidiLoopWrapsAtLastBar`.

## BUG-038 — realtime repeated same-pitch notes blend into one tone
- **Status:** FIXED
- **Symptom:** two consecutive steps on the same pitch re-triggered the noteon without a noteoff (the generation guard discarded the stale noteoff), slurring the notes; the SMF path re-articulated correctly.
- **Fix:** `PlayStep` sends noteoff before noteon for pitches already active (`internal/player/realtime.go`).
- **Tests:** `TestPlayStepRearticulatesRepeatedPitch`.

## BUG-039 — grid string rows drift 1 cell per bar under their headers
- **Status:** FIXED
- **Symptom:** content rows padded content to `barWidth-4` while the label prefix is actually 5 cells (label style `Width(3)`), so each bar's string row was 1 cell wider than its header — fret digits drifted right under the headers.
- **Fix:** pad to `barWidth - lipgloss.Width(prefix)`, measured at render time (`internal/ui/kit/render.go`).
- **Tests:** `TestGridContentRowsAlignWithHeaders` (all rows exactly bars×barWidth at widths 60/76/120).

## BUG-040 — NewStore leaks the DB handle on setup failure
- **Status:** FIXED
- **Symptom:** `NewStore` returned on PRAGMA/migration errors without closing the `*sql.DB`.
- **Fix:** close the handle on every error path (`internal/library/store.go`).

## BUG-041 — MIDI early-stop error appears only on the next tick
- **Status:** FIXED
- **Symptom:** "MIDI engine stopped early" was set without a refresh, so the banner lagged ~250 ms behind the failure (same stale-render family as BUG-033).
- **Fix:** `m.refresh()` after setting the error (`internal/ui/viewer/viewer.go`).

## BUG-042 — long multibyte titles get byte-truncated into invalid filenames
- **Status:** FIXED
- **Symptom:** `sanitizeAudioFilename` cut at `name[:120]` bytes, splitting a UTF-8 rune in the middle for CJK titles → invalid cached-audio filenames (same byte-slicing family as BUG-005).
- **Fix:** truncate on rune boundaries to 120 runes (`internal/player/audio_online.go`).
- **Tests:** `TestSanitizeAudioFilenameRuneSafe`.

## BUG-043 — Unix: killProcessTree hangs forever after SIGKILL (reaper never closes its done channel)
- **Status:** FIXED (caught by running the unix-gated tests in WSL2)
- **Symptom:** on Linux/macOS, stopping playback of a player that ignored SIGTERM (or that took > 200 ms to die) deadlocked: `killProcessTree` escalated to SIGKILL and then blocked on `<-done` forever. The 200 ms SIGTERM path also always fell through to SIGKILL because the channel was never closed. Windows was unaffected (no reaper path).
- **Root cause:** `startReaper`'s goroutine did `cmd.Wait()` + bookkeeping but never `close(done)` — the close was missing from the implementation.
- **Fix:** `close(done)` after reaping (`internal/player/process_unix.go`).
- **Tests:** `TestReaperReportsRunningProcessAlive`, `TestKillProcessTreeWithReaper`, `TestReaperMarksNaturallyExitedProcessAsDead` — all green on Linux (WSL2, Go 1.26.5); full repo suite + e2e re-run green there too.

## BUG-044 — A–B loop set before playback never fires in audio mode (US-6)
- **Status:** FIXED
- **Symptom:** setting loop points (`i`/`u`) while paused showed `↻ A-B` but audio-synced playback never looped. MIDI mode looped (its tick wrap reads the stored bars), so the failure was mode-dependent and confusing.
- **Root cause:** `setLoopPoint` armed `engine.SetLoop(...)` only when a schedule already existed; the schedule is populated at `PlaybackStartedMsg`, so a paused-session loop never reached the engine, and the audio monitor checks `engine.LoopRegion()`.
- **Fix:** extracted `applyLoopRegion()` (maps 1-based inclusive bars → engine times + intro offset); it runs from `setLoopPoint` **and** from `PlaybackStartedMsg`. Loop-restart failures now surface as an error banner instead of `_ =` swallowing (`internal/ui/viewer/viewer.go`). Looped bar headers are highlighted on the grid via `kit.TabCursor.LoopStartBar/LoopEndBar` + `kit.LoopBarStyle`.
- **Tests:** `TestLoopSetBeforePlayArmsEngineAtPlaybackStart`, `TestLoopClearWithoutScheduleClearsEngine` (viewer), `TestGridLoopHeadersHighlighted` (kit).

## BUG-045 — `s` sync-bar key is a silent no-op outside audio-synced playback (US-7)
- **Status:** FIXED
- **Symptom:** pressing `s` while paused or during MIDI playback did nothing with zero feedback, while the footer advertises `[s] sync bar` unconditionally; `S` (clear sync) was undocumented.
- **Fix:** `s` outside audio-synced playback shows an explanatory hint ("Sync bar needs a real recording…"); `S` reports when there is nothing to clear; `S` added to the footer hints and the help screen (which was also missing `[ ]`, `i/u`, `v`, `f`, `> <`) (`internal/ui/viewer/viewer.go`, `internal/ui/help/help.go`).
- **Tests:** `TestSyncBarKeyGivesFeedbackWhenUnavailable`, `TestSyncBarKeyFeedbackClearsOnEsc`, `TestClearSyncPointsKeyReportsEmpty`.

## BUG-046 — linear-layout playhead ruler floats left of the notes (US-8)
- **Status:** FIXED
- **Symptom:** in linear mode (`v`) the ruler line prefixed the `┊` with 3 spaces while string rows use a 5-column label prefix (3-wide right-aligned label + 2 spaces), so the ruler playhead always sat 2 columns left of the notes' playhead. Related: the vertical playhead vanished entirely on rows whose content ended before the cursor column.
- **Fix:** ruler uses the same 5-column prefix; `renderStringContent` extends the highlighted bar's rows to `maxCol = max(maxCol, cursorCol+1)` so the playhead appears on every string at the cursor column (`internal/ui/kit/render.go`).
- **Tests:** `TestLinearRulerAlignsWithStringPlayhead` (ruler and every string row align).

## BUG-047 — online-audio search failures are reported as "no matches" (US-9)
- **Status:** FIXED
- **Symptom:** `SearchOnlineCandidates` discarded per-query errors and returned `(nil, nil)` when every query failed; the audio picker then showed no online sources with no explanation, and the viewer dropped the whole catalog (including local/MIDI sources) whenever the online search errored.
- **Fix:** `SearchOnlineCandidates` returns the last underlying error when no results were found (yt-dlp missing/timed out/network); `BuildAudioCatalog` keeps local/MIDI sources and carries the error; the viewer applies the catalog even on error and surfaces the message (`internal/player/audio_online.go`, `internal/ui/viewer/viewer.go`).
- **Tests:** `TestSearchOnlineCandidatesReportsFailureWhenAllQueriesFail`, `TestSearchOnlineCandidatesEmptyWithoutErrorStaysEmpty`, `TestBuildAudioCatalogSurfacesOnlineSearchError` (player, hermetic fake yt-dlp); `TestAudioCatalogMsgKeepsSourcesAndShowsError`, `TestAudioCatalogMsgErrorWithoutSourcesDoesNotCrash` (viewer).

## BUG-048 — browser has no preview panel despite GUIDE.md documenting one (US-10)
- **Status:** FIXED
- **Symptom:** `GUIDE.md` described a "right-side tab preview" in the browser; the browser rendered a bare list (only the home screen had a preview).
- **Fix:** the browser now loads the selected row's tab in the background (`BrowserPreviewMsg` with a generation guard) and renders a `Preview · <title>` panel beside the list on terminals ≥ 106 columns; narrower terminals keep the full-width list. The list viewport narrows when the preview is active (`internal/ui/browser/browser.go`, `internal/ui/msgs/msgs.go`).
- **Tests:** `TestBrowserPreviewLoadsForSelectedRow`, `TestBrowserPreviewFollowsCursor`, `TestBrowserPreviewStaleGenerationIgnored`, `TestBrowserPreviewCollapsesOnNarrowTerminal`.

## BUG-049 — audio selection is not strict: live/cover/lesson tracks win auto-pick (US-11)
- **Status:** FIXED
- **Symptom:** YouTube candidates were ranked by loose keyword scoring, so a live show or a cover (with "guitar" in the title) could outrank the official studio recording — the tab then never matched the audio.
- **Fix:** new `ClassifyAudioCandidate` taxonomy (official/backing/live/cover/lesson/other) drives a strict scorer with hard penalties for live/cover/lesson and strong rewards for official markers, VEVO, and artist channels. `🔒strict` mode (config `strict_audio_selection`, default on) makes auto-pick skip non-studio candidates, the picker badges every source (`[official]`…) and marks rejects (`⛔ not studio`) and the recommended pick (`★`). When nothing passes, auto-pick falls back to MIDI with an explanatory note instead of downloading a mismatched track (`internal/player/audio_catalog.go`, `audio_online.go`, `internal/ui/viewer/{viewer,viewer_audio}.go`, `internal/config`).
- **Tests:** `TestClassifyAudioCandidate`, `TestStrictCompatible`, `TestScoreYouTubeResultPrefersStudio`, `TestAudioCatalogHasStrictRejected` (player); `TestPickStrictAudioSourceIndexSkipsLiveCover`, `TestPickStrictPrefersLocal`, `TestPickStrictFallsBackToMidi`, `TestRenderAudioPickerShowsBadges` (viewer); `TestDefaultsStrictAudioSelection` (config).

## BUG-050 — sync calibration is shared across audio sources (US-12)
- **Status:** FIXED
- **Symptom:** `audio_offset` and `sync_points` were stored per tab, so switching from a studio version (short intro) to a live version (long intro) reused the wrong offset and anchors — the playhead was consistently off.
- **Fix:** calibration is stored per source under `audio_offset:<source-id>` / `sync_points:<source-id>` (legacy keys mirrored, read as fallback). Switching sources in the picker, completing a download, and opening a tab all restore the right calibration for the selected source; BPM derivation now runs after the restore so the intro offset is excluded (`internal/ui/viewer/viewer.go`).
- **Tests:** `TestPerSourceCalibrationRestoredOnSourceSwitch`, `TestAdjustAudioOffsetWritesPerSourceKey`, `TestSyncPointsWrittenPerSource` (viewer).

## BUG-051 — anchor mapping is step-count linear, not tick-aware; no tempo map (US-13)
- **Status:** FIXED
- **Symptom:** between sync anchors the playhead mapped audio time linearly over the step *count*, so a bar full of chords (many steps) and a sparse bar at the same real-time length advanced at the same rate, and tempo changes between anchors were ignored entirely.
- **Fix:** segment mapping now accumulates MIDI ticks (`segmentStep`, `TicksBetweenBars`) so density is respected; `SegmentBPM` derives the effective tempo of each anchor segment; the panel shows the spanned tempo range (`60→120 bpm`) and the anchor drift RMS (`±2.0s`). `stepIndexAtBar` now returns `len(schedule)` for out-of-range bars so half-open end anchors clamp correctly (`internal/player/audio_sync.go`, `internal/ui/viewer/viewer.go`).
- **Tests:** `TestSegmentStepUsesTickDensity`, `TestTicksBetweenBars`, `TestSegmentBPMDerivesTempo`, `TestTicksToSeconds`, `TestStepIndexAtSyncPointsFollowsAnchors` (player); `TestTempoMapAndQuality`, `TestTempoMapNeedsTwoAnchors` (viewer); existing `TestStepIndexAtSyncPoints` boundary updated.

## BUG-052 — intros must be found, not guessed; fine nudges missing (US-14)
- **Status:** FIXED
- **Symptom:** recordings with long intros stayed misaligned until the user manually guessed an offset; the coarsest nudge was 0.5 s, and there was no assist for finding the intro at all.
- **Fix:** `LeadingSilence` probes the selected file with ffmpeg `silencedetect` (0.4–30 s window, silent no-op without ffmpeg) and, when the source is uncalibrated, applies the detected offset marked `audio_offset_auto` (announced in the panel, never re-run). New `,`/`.` keys nudge ±0.1 s; help and footer document the full sync workflow (`internal/player/intro.go`, `internal/ui/viewer/viewer.go`, `internal/ui/help/help.go`).
- **Tests:** `TestLeadingSilenceDetectsIntro`, `TestLeadingSilenceNoIntro`, `TestLeadingSilenceAbsentFFmpeg` (player, hermetic fake ffmpeg); `TestIntroDetectedMsgAppliesOffset`, `TestIntroDetectedMsgIgnoresCalibrated`, `TestIntroDetectedMsgIgnoresStaleSource`, `TestAdjustAudioOffsetFineNudge` (viewer).

## BUG-053 — song search only covers UG + Songsterr; many songs can't be found (US-15)
- **Status:** FIXED
- **Symptom:** the search was a two-source dead end: UG (API + HTML) and Songsterr, whose "fetch" is itself a UG fallback — so when UG failed, nothing could be imported.
- **Fix:** new `textTabClient` (`internal/scraper/texttab.go`) queries two plain-text tab archives — guitartabs.cc and guitaretab.com — parsing artist→tab anchor pairs from search pages and extracting the ASCII tab from `<pre>` blocks (span-row unwrapping for guitaretab). Degraded-query retries (drop first/last 2 words) handle song-only engines for "Artist Song" queries. Fetched content is cleaned (legacy usenet headers, `:`-prefixed repeat rows) before the standard ASCII parser; title/artist come from the search result; tuning is normalized when `:`-rows inflated the string count; chord-only and drum pages are rejected with clear errors. Results are merged + deduplicated with the existing sources and badged `[GT]`/`[GR]` in the search UI (`internal/scraper/client.go`, `source.go`, `internal/ui/search/search.go`). Also fixed the parser's `bpmRegex` to accept "112 BPM" (number-first) in addition to "BPM: 112".
- **Tests:** `TestTextTabSearchParsesGuitarTabsPage`, `TestTextTabSearchParsesGuitareTabPage`, `TestTextTabSearchFiltersJunk`, `TestTextTabFetchExtractsPreAndParses`, `TestTextTabFetchGuitareTabSpans`, `TestTextTabFetchRejectsChordOnly`, `TestTextTabFetchRejectsBadStatus`, `TestTextTabFetchRejectsDrumTabs`, `TestTextTabSongOnlyRetries`, `TestDropWords`, `TestTextTabSearchMergesAcrossSites`, `TestCleanupFetchedTabStripsUsenetHeaders`, `TestNormalizeFetchedTuning`; live-verified against both sites (32 results for "sultans of swing dire straits", real tabs of 50–131 bars).

## UI redesign (2026-08, tui-design skill)

Not a bug — a chrome overhaul driven by the pageton/tui-design skill
(installed at `~/.config/opencode/skills/tui-design/`). What changed and why:

- **Header**: dropped the full-width background bar and the theme name — the
  breadcrumb is now plain text next to the logo (weight + position carry it).
- **Borders**: one border per screen instead of borders everywhere. Home stat
  boxes → aligned label-above-value columns (`TABS │ FAVORITES │ RECENT`);
  search's two stacked panels → a single panel with an inline query/results
  divider; the library list → a table with a dim `TITLE/ARTIST/TUNING` header
  and sort indicator, inside an untitled panel. `RenderPanel("")` renders a
  border without a title/divider (`internal/ui/kit/chrome.go`).
- **Viewer**: title (`Layla — Eric Clapton`) and a dim status row (tuning ·
  bar · BPM · ▶/♪/↔/⚓/↻/⏩ indicators) sit above the panel; the tab body
  renders without its internal header (`RenderTabGridBody` /
  `RenderTabLinearBody`). The old single line cramming every indicator into
  the panel title is gone.
- **Footer**: gains a left status segment (`3 tabs · recent`, `♪ Layla`) via
  `RenderFooterWithStatus`; hint lists trimmed to essentials.
- **States**: proper empty states (no tabs / no filter matches) with a next
  action; narrow-terminal stat fallback to a flat label:value row.
- **Tests**: home stat tests updated to the new layout; new
  `TestHomeStatRowSeparatorsAligned`; e2e green.

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
