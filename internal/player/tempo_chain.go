package player

import (
	"strconv"
	"strings"

	"fretboard/internal/model"
)

// TempoSource identifies where a tab's playback tempo came from. The iota
// order IS the resolution chain priority: an earlier source wins over every
// later one, so callers and the UI can reason about the chain by the enum
// alone.
type TempoSource int

const (
	// TempoMetadata is an explicit numeric tempo in tab metadata (the "bpm"
	// or "tempo" keys). The scraper folds UG version tempos into "bpm" at
	// import, so a plain number here is the strongest available signal.
	TempoMetadata TempoSource = iota
	// TempoUGVersion is a tempo embedded in free-text metadata, e.g. a UG
	// version description ("Based on the video. 107 bpm.") that was not
	// normalized to a bare number.
	TempoUGVersion
	// TempoAudioSync is a BPM measured from aligned audio.
	TempoAudioSync
	// TempoRemembered is the per-tab value recalled from a previous session.
	TempoRemembered
	// TempoFilenameGuess is a tempo parsed from the tab title, which is
	// derived from the filename at import.
	TempoFilenameGuess
	// TempoDefault is the 120 BPM fallback when nothing else yields a tempo.
	TempoDefault
)

// String returns a stable snake_case identifier for the source, suitable for
// logging or machine-readable persistence.
func (s TempoSource) String() string {
	switch s {
	case TempoMetadata:
		return "metadata"
	case TempoUGVersion:
		return "ug_version"
	case TempoAudioSync:
		return "audio_sync"
	case TempoRemembered:
		return "remembered"
	case TempoFilenameGuess:
		return "filename"
	case TempoDefault:
		return "default"
	}
	return "unknown"
}

// TempoProvenanceLabel renders the short display label shown next to a
// resolved tempo in the UI. The strings are deliberately terse: they appear
// inline in a status row, not in a settings panel.
func TempoProvenanceLabel(src TempoSource) string {
	switch src {
	case TempoMetadata:
		return "metadata"
	case TempoUGVersion:
		return "from UG version"
	case TempoAudioSync:
		return "from audio sync"
	case TempoRemembered:
		return "remembered"
	case TempoFilenameGuess:
		return "from filename"
	case TempoDefault:
		return "default"
	}
	return "unknown"
}

// TempoSourceMetaKey is the metadata key holding the provenance label of the
// last recorded tempo (see RecordTempoUsage), so a later session can show why
// a tempo was chosen without re-running the resolution chain.
const TempoSourceMetaKey = "tempo_source"

// ResolveTempo picks the playback tempo for a tab by walking the resolution
// chain in priority order:
//
//  1. TempoMetadata — an explicit numeric tempo in tab metadata. read via the
//     "bpm"/"tempo" keys, matching how TabBPM reads them.
//  2. TempoUGVersion — free-text metadata (e.g. a UG version description)
//     containing "N bpm".
//  3. TempoAudioSync — the BPM measured from aligned audio (audioBPM).
//  4. TempoRemembered — the per-tab value remembered from a previous session
//     (remembered).
//  5. TempoFilenameGuess — a tempo parsed from the tab title (the title is
//     derived from the filename at import; Tab carries no path field).
//  6. TempoDefault — 120 BPM.
//
// The returned src tells the caller (and the UI) which rung won.
func ResolveTempo(tab *model.Tab, audioBPM int, remembered int) (bpm int, src TempoSource) {
	// Rungs 1-2: explicit tab metadata outranks every measured or guessed
	// tempo, because it is the one value the user (or the source site)
	// actually stated.
	if n, s := resolveMetadataTempo(tab); s != TempoDefault {
		return n, s
	}
	// Rung 3: an aligned recording's measured BPM beats remembered and
	// guessed values — it is measured from the very audio being played.
	if audioBPM > 0 {
		return ClampBPM(audioBPM), TempoAudioSync
	}
	// Rung 4: what the user settled on last time beats a filename guess.
	if remembered > 0 {
		return ClampBPM(remembered), TempoRemembered
	}
	// Rung 5: a tempo in the title ("Song 107 bpm") is a weak but useful
	// signal; Tab has no filename, so the title is the closest proxy.
	if tab != nil && tab.Title != "" {
		if n := model.ParseBPMFromText(tab.Title); n > 0 {
			return n, TempoFilenameGuess
		}
	}
	// Rung 6: nothing known — fall back to the canonical 120 BPM.
	return DefaultBPM, TempoDefault
}

// resolveMetadataTempo reads the metadata-only rungs of the chain: a bare
// integer in the "bpm"/"tempo" keys is TempoMetadata; free text containing
// "N bpm" (a UG version description) is TempoUGVersion. It returns
// TempoDefault (with bpm 0) when metadata has no usable tempo, so ResolveTempo
// can continue down the chain — unlike TabBPM, which falls back to 120.
func resolveMetadataTempo(tab *model.Tab) (int, TempoSource) {
	if tab == nil || tab.Metadata == nil {
		return 0, TempoDefault
	}
	for _, key := range []string{model.MetaKeyBPM, model.MetaKeyTempo} {
		s := strings.TrimSpace(tab.Metadata[key])
		if s == "" {
			continue
		}
		// Bare number: explicitly set tempo.
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			return ClampBPM(n), TempoMetadata
		}
		// Free text: UG version description style ("... 107 bpm.").
		if n := model.ParseBPMFromText(s); n > 0 {
			return n, TempoUGVersion
		}
	}
	return 0, TempoDefault
}

// RecordTempoUsage persists the tempo actually used for a tab so the next
// session recalls it: the BPM goes into the explicit "tempo" metadata key
// (the first rung of the resolution chain) and TempoSourceMetaKey records the
// provenance label. The label is derived from the pre-write metadata when the
// tab already carried a metadata tempo; otherwise the stored value is marked
// "remembered" — the role it plays for later sessions.
func RecordTempoUsage(tab *model.Tab, bpm int) {
	if tab == nil || bpm <= 0 {
		return
	}
	if tab.Metadata == nil {
		tab.Metadata = map[string]string{}
	}
	bpm = ClampBPM(bpm)
	if _, s := resolveMetadataTempo(tab); s != TempoDefault {
		tab.Metadata[TempoSourceMetaKey] = TempoProvenanceLabel(s)
	} else {
		tab.Metadata[TempoSourceMetaKey] = TempoProvenanceLabel(TempoRemembered)
	}
	tab.Metadata[model.MetaKeyTempo] = strconv.Itoa(bpm)
}
