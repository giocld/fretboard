# User stories & use cases

5 real practice workflows for fretboard, each with a formal use case.
Status legend: **done** (implemented + tested) · **partial** (core works, refinement tracked) · **backlog** (needs work below).

---

## US-1 — Whole-song grid layout

> **As a** guitarist practicing a song, **I want** the entire tab laid out in a
> compact grid of bars that fills my terminal width, **so that** I can read
> ahead without scrolling through a long vertical strip that uses only half
> the screen.

**Use case UC-1: View a tab in page layout**
- **Actor:** musician
- **Preconditions:** library has a tab; the viewer is open
- **Main flow:**
  1. User opens a tab from the library.
  2. System renders bars left-to-right, wrapping into rows that span the terminal width (`kit.RenderTabGrid`).
  3. System pads each bar column so the tab fills the available width.
  4. System highlights the active bar and draws a vertical playhead (`┊`) on every string at the cursor column.
  5. If a bar is wider than one column, the system widens all columns to fit (rows wrap accordingly) — nothing is clipped.
- **Alternative flows:**
  - *A1. Narrow bars:* several bars share a row (up to 6).
  - *A2. Wide bars:* one bar per row, panning via `h`/`l` still works.
- **Postconditions:** the tab occupies the full panel width; the cursor bar is visible and highlighted.
- **Status: done** (page layout, adaptive bar width, grid-aware scroll math). Refinements: layout toggle, follow-mode auto-scroll → roadmap F1/F2.

---

## US-2 — Tab cursor follows a real MP3

> **As a** musician practicing with the actual studio recording, **I want** the
> tab cursor to stay on the bar/beat that is playing in the MP3, **so that** I
> can play along without losing my place.

**Use case UC-2: Play a tab with a backing MP3**
- **Actor:** musician
- **Preconditions:** an audio source (local file or downloaded) is selected; audio sync is active (engine mode "audio")
- **Main flow:**
  1. User presses `Space`.
  2. System starts audio playback and builds the note schedule from the tab's rhythm notation.
  3. Every 250 ms the system maps elapsed audio time to a schedule step via `StepIndexAtScheduleTime` — the tab's own note durations at the tab BPM, not a linear fraction of the audio file.
  4. System moves the playhead to that step's bar/column and scrolls to keep it visible.
- **Alternative flows:**
  - *A1. Music time before the tab start* (intro): cursor holds on bar 1.
  - *A2. Audio ends:* playback stops, cursor rests at the last bar.
- **Postconditions:** playhead tracks the recording; user can jump (digits+enter) or restart from cursor (Space).
- **Status: done** (schedule-time mapping replaced the linear audio-fraction mapping). Remaining drift for songs whose tempo changes mid-song → roadmap F3 (sync points).

---

## US-3 — Sync calibration for intros and count-ins

> **As a** musician syncing a tab to a recording that starts with silence,
> applause, or a count-in, **I want** to shift the tab start relative to the
> audio, **so that** bar 1 lands exactly on the first beat of the music.

**Use case UC-3: Calibrate the audio start offset**
- **Actor:** musician
- **Preconditions:** UC-2 is in progress (or a tab is loaded)
- **Main flow:**
  1. User plays the audio and watches the playhead.
  2. User presses `]` to nudge the tab start later in the audio, or `[` to nudge earlier (±0.5 s; `{`/`}` = ±5 s).
  3. System re-maps the playhead within 250 ms and shows the current offset in the panel title (`↔ +3.5s`).
  4. User presses `o` to reset the offset to 0.
  5. System persists the offset in the tab's metadata (`audio_offset`), so it is restored the next time the tab opens.
- **Alternative flows:**
  - *A1. Jumpy playback while adjusting:* user pauses (`Space`), adjusts, resumes — offset still applies.
- **Postconditions:** offset stored per tab; playhead aligns bar 1 with the music start.
- **Status: done** (keys, live re-mapping, persistence, load-time restore). Precise multi-point alignment for tempo changes → roadmap F3.

---

## US-4 — Loop a section (A–B) for drilling

> **As a** musician learning a hard passage, **I want** to loop a section of
> the tab and its audio, **so that** I can repeat the same bars until they
> stick.

**Use case UC-4: Practice a section on repeat**
- **Actor:** musician
- **Preconditions:** tab loaded; an audio source selected (or MIDI)
- **Main flow:**
  1. User moves the cursor to the section start bar and sets loop point A.
  2. User moves the cursor to the section end bar and sets loop point B.
  3. System shows the loop region in the panel and on the bar grid.
  4. When playback reaches bar B, the system jumps back to bar A (audio seeks to the corresponding time).
- **Alternative flows:**
  - *A1. Loop off:* user clears the region.
  - *A2. Loop region changed mid-play:* applied on the next pass.
- **Postconditions:** playback cycles the A–B region; the playhead wraps.
- **Status: done** — engine A-B loop with grid highlight and pre-play arming (US-6); audio seeks to the loop point each pass.

---

## US-5 — Slow-down practice with a real recording

> **As a** musician learning a fast solo, **I want** to play the recording
> slower without changing pitch, **so that** I can hear and learn the notes,
> **while** the tab cursor stays synced.

**Use case UC-5: Practice at reduced speed**
- **Actor:** musician
- **Preconditions:** UC-2 is active
- **Main flow:**
  1. User presses a speed-down key (e.g. `-` cycle 100% → 90% → 80% …).
  2. System re-plays the audio at the reduced rate (pitch preserved) and rescales the schedule-time mapping.
  3. Playhead follows the slowed audio.
- **Alternative flows:**
  - *A1. Speed up:* user cycles back toward 100%.
- **Postconditions:** audio tempo changed without pitch shift; cursor still synced.
- **Status: backlog** — blocking issues/features: F6 (audio rate control via ffmpeg atempo or resample), F7 (recompute schedule mapping for the playback rate), F8 (UI for speed cycling).

---

## US-6 — A–B loop set before playback actually loops

> **As a** musician who marks a section before hitting play, **I want** the A–B
> loop I set while the tab is paused to loop when playback starts, **so that** I
> don't have to start, stop, and re-mark the section every time.

**Use case UC-6: Set loop points before playback**
- **Actor:** musician
- **Preconditions:** tab loaded; cursor is parked on a bar; playback is not running
- **Main flow:**
  1. User moves to the section start bar and presses `i` (loop point A).
  2. User moves to the section end bar and presses `u` (loop point B).
  3. System shows the loop region (`↻ A-B`) in the panel and highlights the looped bars on the grid.
  4. User presses `Space`.
  5. System arms the audio/MIDI loop from the stored bars **before** the first note, so the playhead wraps at bar B on every pass.
- **Alternative flows:**
  - *A1. Audio sync:* the audio file seeks back to the loop start's file position each pass.
  - *A2. Loop restart fails (audio player died):* the failure is shown as an error banner instead of the cursor silently snapping to bar 1.
- **Postconditions:** playback cycles the A–B region without the user re-marking it.
- **Status: done** — engine region armed at playback start (and on every point
  change), looped bars highlighted on the grid, loop-restart failures surface
  as an error banner.

---

## US-7 — Sync-bar feedback instead of a silent key

> **As a** user who follows the footer hints, **I want** pressing `s` (sync bar)
> to either set an anchor or tell me why it can't, **so that** I never wonder
> whether the key did anything.

**Use case UC-7: Sync a bar to the recording**
- **Actor:** musician
- **Preconditions:** viewer is open with a tab loaded
- **Main flow:**
  1. User presses `s` while the tab is playing back through a real recording (audio-synced).
  2. System anchors the current bar to the current audio position and persists it (`sync_points` metadata).
  3. User presses `s` again at a later bar; a second anchor is stored.
  4. System maps the playhead between anchors so tempo changes stop drifting.
- **Alternative flows:**
  - *A1. Not playing / MIDI synth only:* system shows a hint — "Sync bar needs a real recording: play with an audio source (a), then press s" — instead of doing nothing.
  - *A2. Anchors exist:* `S` clears them, with the panel indicator updating immediately.
- **Postconditions:** every `s` press produces either an anchor or an explanatory message.
- **Status: done** — unavailable states show an explanatory hint; `S` clears
  anchors (and says when there are none); both keys documented in help + footer.

---

## US-8 — Linear layout playhead ruler lines up with the notes

> **As a** guitarist reading in linear mode (`v`), **I want** the playhead ruler
> above the strings to point at the same column as the `┊` markers on the note
> rows, **so that** I can follow the moving cursor across the bar.

**Use case UC-8: Follow the playhead in linear layout**
- **Actor:** musician
- **Preconditions:** viewer open, linear layout active (`v`), playback running
- **Main flow:**
  1. User toggles to linear layout.
  2. Each bar block shows a ruler line above the string rows with a moving `┊`.
  3. The ruler's playhead column matches the string rows' playhead columns exactly (both use the same label prefix width).
- **Postconditions:** the ruler and the note rows agree on where the current beat is.
- **Status: done** — ruler and string rows share the 5-column prefix; the
  playhead now also appears on every string row at the cursor column.

---

## US-9 — Honest audio-source errors

> **As a** user whose online-audio search fails, **I want** the picker to tell me
> what actually went wrong (yt-dlp missing, timed out, network error), **so
> that** I can fix the cause instead of assuming no recording exists.

**Use case UC-9: Diagnose a failed online-audio lookup**
- **Actor:** musician
- **Preconditions:** a tab is open; the audio picker (`a`) is shown
- **Main flow:**
  1. User opens the audio source picker.
  2. System searches for a matching recording.
  3. If every search query fails, the picker keeps the local/MIDI sources and shows the underlying failure (e.g. "yt-dlp search timed out").
  4. If at least one query succeeds, online results are ranked and shown as before.
- **Postconditions:** the user can distinguish "no matching recording" from "the search tool is broken".
- **Status: done** — total failures surface the underlying error (yt-dlp
  missing/timed out/network); local/MIDI sources are kept alongside the error.

---

## US-10 — Preview a tab in the library browser

> **As a** browser user browsing a big library, **I want** to see the selected
> tab's first bars without opening it, **so that** I can tell tabs apart by
> their riffs.

**Use case UC-10: Preview before opening**
- **Actor:** musician
- **Preconditions:** library has at least one tab; browser is open
- **Main flow:**
  1. User moves the cursor over a tab.
  2. System loads the tab's content in the background and renders its first bars in a preview panel beside the list.
  3. Moving to another row reloads the preview for that row.
- **Alternative flows:**
  - *A1. Narrow terminal:* the preview collapses and the list takes the full width (as before).
- **Postconditions:** the selected tab's opening bars are visible next to the list.
- **Status: done** — the browser renders a `Preview · <title>` panel beside the
  list on wide terminals (≥ 106 cols); narrow terminals keep the full-width list.

---

## US-11 — Strict studio audio selection

> **As a** musician who practices against recordings, **I want** the app to pick
> the studio/official version of a song and keep live shows, covers, and
> lessons out of my way, **so that** the tab actually matches the audio I'm
> hearing.

**Use case UC-11: Pick the right recording**
- **Actor:** musician
- **Preconditions:** a tab is open; online audio is available
- **Main flow:**
  1. System searches for recordings and classifies each result (official / live / cover / backing / lesson) from its title, channel, and description.
  2. The picker shows the classification as a badge (`[official]`, `[live]`, …) with the recommended pick marked `★`.
  3. Auto-pick selects the best strict-compatible candidate (official or backing); live/cover/lesson candidates are marked `⛔ not studio` and skipped.
  4. If nothing passes strict selection, the app plays MIDI and says why instead of auto-downloading a mismatched recording.
- **Alternative flows:**
  - *A1. Strict off:* the old behavior (prefer local file, else MIDI) applies.
- **Postconditions:** the default pick is the same performance as the tab; mismatched recordings are clearly labeled.
- **Status: done** — category classifier, strict scorer, `🔒strict` mode
  (config `strict_audio_selection`, default on), picker badges and
  recommendations.

---

## US-12 — Per-source sync calibration

> **As a** musician who alternates between a studio version and a live version
> of a song, **I want** each recording to keep its own intro offset and sync
> anchors, **so that** switching sources never reuses the wrong calibration.

**Use case UC-12: Calibrate each recording separately**
- **Actor:** musician
- **Preconditions:** a tab is open with an audio source selected
- **Main flow:**
  1. User selects a source in the picker and calibrates it (`[`/`]` nudges, `s` anchors).
  2. System stores the offset and anchors under the source's key (`audio_offset:<id>`, `sync_points:<id>`), mirroring the legacy keys.
  3. User switches to another source; the new source's own calibration is loaded, and the old source's values are untouched.
- **Postconditions:** every recording keeps its own sync state; legacy tabs still work.
- **Status: done** — per-source keys with legacy fallback, restore on source
  switch and tab load.

---

## US-13 — Tempo-map sync from anchors

> **As a** musician whose song's tempo drifts, **I want** the playhead to follow
> the anchors instead of a constant-BPM schedule, **so that** it stays on the
> right bar from intro to outro.

**Use case UC-13: Follow drifting tempo**
- **Actor:** musician
- **Preconditions:** audio-synced playback is active
- **Main flow:**
  1. User sets 2+ sync anchors (`s` at recognizable bars).
  2. System derives the tempo of each anchor-to-anchor segment from the tab's MIDI ticks and the real audio time.
  3. Between anchors the playhead maps audio time to the schedule proportionally to note density (tick-accumulated), not raw step count.
  4. The panel shows the tempo map (`60→120 bpm`) and the anchor drift estimate (`±2.0s`).
- **Postconditions:** the cursor tracks the audio even when the tempo changes.
- **Status: done** — tick-aware anchor interpolation, `SegmentBPM`,
  `TicksBetweenBars`, panel indicators.

---

## US-14 — Intro auto-detection and fine sync controls

> **As a** musician syncing a tab to a recording with a long intro, **I want**
> the app to detect the leading silence and let me fine-tune in 0.1 s steps,
> **so that** bar 1 lands on the first beat without guesswork.

**Use case UC-14: Sync against an intro**
- **Actor:** musician
- **Preconditions:** a tab is open with an audio source selected (ffmpeg optional)
- **Main flow:**
  1. System probes the selected recording's leading silence (ffmpeg `silencedetect`) and, if uncalibrated, applies the detected intro as the offset (`↔ +3.2s`), marked as auto-detected.
  2. User fine-tunes with `[`/`]` (±0.5 s) or `,`/`.` (±0.1 s) and resets with `o`.
  3. Manual nudges and anchors replace the auto value; auto-detection never re-runs for that source.
- **Postconditions:** intros are accounted for automatically, with fast fine control.
- **Status: done** — `LeadingSilence` probe (0.4–30 s window, graceful when
  ffmpeg is absent), auto marker, fine nudge keys, help + footer docs.

---

## US-15 — More tab sources, fewer dead ends

> **As a** guitarist who searches for songs that Ultimate Guitar and Songsterr
> don't have, **I want** the search to also query other tab archives, **so
> that** the tab I need is found instead of a "no results" dead end.

**Use case UC-15: Search across multiple tab archives**
- **Actor:** musician
- **Preconditions:** online search is open
- **Main flow:**
  1. User types a song or artist and searches.
  2. System queries Ultimate Guitar (API, HTML fallback), Songsterr, GuitarTabs.cc, and GuitareTab.com, merging and deduplicating results.
  3. Results carry a source badge (`[UG]`, `[ST]`, `[GT]`, `[GR]`).
  4. Fetching a GuitarTabs.cc / GuitareTab.com result extracts the page's `<pre>` tab text (including the guitartabs usenet-header cleanup and `:`-repeat-row stripping), parses it with the standard ASCII parser, and backfills title/artist from the search result.
- **Alternative flows:**
  - *A1. Song-only engine misses "Artist Song":* the query is retried with the leading/trailing word pairs dropped.
  - *A2. Chord-only, drum, or unparseable pages:* rejected with a clear error, never imported as garbage.
- **Postconditions:** four archives feed the search; more songs resolve to real, playable tabs.
- **Status: done** — `textTabClient` with a site table (`guitartabs.cc`,
  `guitaretab.com`), degraded-query retries for song-only engines, pre-block
  extraction, cleanup, tuning normalization, source badges, and hermetic
  tests.

---

# Roadmap backlog

Items that must land to satisfy the use cases above. Severity: how much the
missing item blocks the UC. Numbers F1…F8 are tracked as todos; each one
includes the blocker issue it resolves.

| ID | Feature / issue | Blocks | Severity |
|----|-----------------|--------|----------|
| F1 | Layout toggle (page grid ↔ linear strip) like TuxGuitar's Page/Linear modes | UC-1 (refinement) | low |
| F2 | Follow-mode auto-scroll: playhead scrolls the grid; manual scroll pauses following (tablatures.app pattern) | UC-1 (refinement) | low |
| F3 | Per-bar sync points (GP8 pattern): anchor bars to audio transients; mapping interpolates between anchors; bypasses constant-BPM schedule | UC-2/UC-3 | **high** |
| F4 | Engine support for looping/seek during audio playback (position resets to loop point) | UC-4 | ~~**high**~~ **done (US-6)** |
| F5 | A–B loop UI: set/clear loop points, region indicator on grid + panel | UC-4 | ~~**high**~~ **done (US-6)** |
| F6 | Audio rate control (pitch-preserving, via ffmpeg `atempo` or resample pipeline) | UC-5 | **high** |
| F7 | Schedule-time mapping scaled by playback rate + offset-aware BPM derivation (DeriveBPMFromAudio should subtract the calibrated offset) | UC-5, UC-3 (refinement) | medium |
| F8 | Speed-cycle UI keys + panel indicator | UC-5 | medium |

## Blocker issues currently tracked in BUGS.md

- **BUG-015** — `DeriveBPMFromAudio` scales the whole audio file to the whole
  tab; with a nonzero `audio_offset` the derived BPM is shifted by the intro.
  Fix: derive using `audioDur - offset`. (Related to F7.)
- **BUG-016** — `maxPanOffset` assumes a linear strip (uses `maxBarColumns`),
  so `h`/`l` panning in grid layout can overshoot for multi-bar rows.
  Fix: pan against grid columns. (Related to F1.)
