package viewer

import "fretboard/internal/player"

// The two-axis playback state machine, rendered in the status line as
// [load|sync]. The Load axis describes how far the tab's audio setup has
// come; the Sync axis describes how well the playhead is tied to a real
// recording. calibrating and endBanner are orthogonal flags that tag the
// label without changing either axis.

// Load-axis states. Evaluation order (first match wins): LoadNoTab is the
// nil-tab guard checked first, then LoadMidi, LoadRemote, LoadNoSource,
// LoadReady. LoadNoTab leads because every downstream field degenerates for a
// nil tab (selectedSource defaults to MIDI, fetching flags are always clear,
// resolvedAudio is empty); the status line only renders inside the tab!=nil
// branch, so LoadNoTab exists for the pure function and its tests.
const (
	loadNoTab    = "no-tab"
	loadMidi     = "midi"
	loadRemote   = "remote"
	loadNoSource = "no-source"
	loadReady    = "ready"
)

// Sync-axis states. Evaluation order (first match wins): SyncEnded,
// SyncOff, SyncLoop, SyncDrift, SyncAuto, SyncAnchored, SyncAnchorNeeded,
// SyncUnsynced (the F1 home). SyncEnded outranks SyncOff because the monitor
// sets the banner and then stops playback, so a strict "not playing first"
// order would never surface the ended state.
const (
	syncEnded        = "ended"
	syncOff          = "off"
	syncLoop         = "loop"
	syncDrift        = "drift"
	syncAuto         = "auto"
	syncAnchored     = "anchored"
	syncAnchorNeeded = "anchor"
	syncUnsynced     = "unsynced"
)

// syncState is the pure two-axis playback state of a viewer. The function is
// named syncStateOf because Go forbids a package-level function and type
// sharing one name.
type syncState struct {
	load        string
	sync        string
	calibrating bool // an analysis command (align/intro/BPM) is in flight
	endBanner   bool // the track-ended banner is showing
}

// label renders the state as an ASCII status tag, e.g. [ready|anchored],
// with "[end]" for the track-ended banner and "..." for an in-flight
// analysis appended after the bracket.
func (s syncState) label() string {
	out := "[" + s.load + "|" + s.sync + "]"
	if s.endBanner {
		out += "[end]"
	}
	if s.calibrating {
		out += "..."
	}
	return out
}

// syncStateOf computes the two-axis state from the viewer fields.
func syncStateOf(m ViewerModel) syncState {
	return syncState{
		load:        loadState(m),
		sync:        syncAxis(m),
		calibrating: m.calibrating,
		endBanner:   m.endBanner,
	}
}

// loadState resolves the Load axis. A MIDI source is the most specific load
// state (the tab is set up for the synthesizer, not a recording); an in-
// flight online fetch is next; then a selected source with no usable audio
// path; and finally the ready state (resolved path or a catalog source with
// one).
func loadState(m ViewerModel) string {
	if m.tab == nil {
		return loadNoTab
	}
	if m.selectedSource().Kind == player.SourceMIDI {
		return loadMidi
	}
	if m.fetchingCatalog || m.fetchingAudio {
		return loadRemote
	}
	if m.resolvedAudio == "" && !catalogReady(m.audioCatalog) {
		return loadNoSource
	}
	return loadReady
}

// catalogReady reports whether any non-MIDI source in the catalog carries a
// usable audio path.
func catalogReady(cat player.AudioCatalog) bool {
	for _, s := range cat.Sources {
		if s.Kind != player.SourceMIDI && s.Path != "" {
			return true
		}
	}
	return false
}

// syncAxis resolves the Sync axis. The endBanner state leads so the track-
// ended condition stays visible after the monitor stops playback. SyncOff
// uses !audioSync as the mirror of the engine mode: audioSync is set from
// syncedFor(engine.Mode()) at playback start, so it is true exactly when the
// playhead follows the audio (mode "audio") rather than the tab deadline
// clock (MIDI). SyncUnsynced is the fallback home: audio-synced playback
// with no anchors, no auto tempo map, and no offset.
func syncAxis(m ViewerModel) string {
	if m.tab == nil {
		return syncOff
	}
	if m.endBanner {
		return syncEnded
	}
	if !m.playing || !m.audioSync {
		return syncOff
	}
	if m.loopStartBar > 0 && m.loopEndBar > 0 {
		return syncLoop
	}
	if m.autoActive && (m.syncDrift > 0.04 || m.syncDrift < -0.04) {
		return syncDrift
	}
	if m.autoActive && len(m.syncPoints) == 0 {
		return syncAuto
	}
	if len(m.syncPoints) > 0 {
		return syncAnchored
	}
	if m.audioOffset != 0 {
		return syncAnchorNeeded
	}
	return syncUnsynced
}
