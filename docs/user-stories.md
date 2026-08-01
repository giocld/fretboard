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
- **Status: backlog** — blocking issues/features: F4 (engine audio loop/seek), F5 (loop-region UI + keys).

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

# Roadmap backlog

Items that must land to satisfy the use cases above. Severity: how much the
missing item blocks the UC. Numbers F1…F8 are tracked as todos; each one
includes the blocker issue it resolves.

| ID | Feature / issue | Blocks | Severity |
|----|-----------------|--------|----------|
| F1 | Layout toggle (page grid ↔ linear strip) like TuxGuitar's Page/Linear modes | UC-1 (refinement) | low |
| F2 | Follow-mode auto-scroll: playhead scrolls the grid; manual scroll pauses following (tablatures.app pattern) | UC-1 (refinement) | low |
| F3 | Per-bar sync points (GP8 pattern): anchor bars to audio transients; mapping interpolates between anchors; bypasses constant-BPM schedule | UC-2/UC-3 | **high** |
| F4 | Engine support for looping/seek during audio playback (position resets to loop point) | UC-4 | **high** |
| F5 | A–B loop UI: set/clear loop points, region indicator on grid + panel | UC-4 | **high** |
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
