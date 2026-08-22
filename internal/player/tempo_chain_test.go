package player

import (
	"testing"

	"fretboard/internal/model"
)

// TestResolveTempoChainOrder verifies the full resolution chain: every rung
// wins over every lower rung, and the reported source matches the winning
// rung.
func TestResolveTempoChainOrder(t *testing.T) {
	cases := []struct {
		name      string
		tab       *model.Tab
		audioBPM  int
		rememberd int
		wantBPM   int
		wantSrc   TempoSource
	}{
		{
			name:     "metadata beats audio sync",
			tab:      &model.Tab{Metadata: map[string]string{model.MetaKeyBPM: "110"}},
			audioBPM: 140,
			wantBPM:  110,
			wantSrc:  TempoMetadata,
		},
		{
			name:      "metadata beats remembered and filename",
			tab:       &model.Tab{Title: "Song 107 bpm", Metadata: map[string]string{model.MetaKeyTempo: "98"}},
			rememberd: 125,
			wantBPM:   98,
			wantSrc:   TempoMetadata,
		},
		{
			name:     "ug version text beats audio sync",
			tab:      &model.Tab{Metadata: map[string]string{model.MetaKeyTempo: "Based on the video. 107 bpm."}},
			audioBPM: 140,
			wantBPM:  107,
			wantSrc:  TempoUGVersion,
		},
		{
			name:      "audio sync beats remembered",
			tab:       &model.Tab{Title: "Song 107 bpm"},
			audioBPM:  140,
			rememberd: 130,
			wantBPM:   140,
			wantSrc:   TempoAudioSync,
		},
		{
			name:      "remembered beats filename guess",
			tab:       &model.Tab{Title: "Song 107 bpm"},
			rememberd: 125,
			wantBPM:   125,
			wantSrc:   TempoRemembered,
		},
		{
			name:    "filename guess beats default",
			tab:     &model.Tab{Title: "House of the Rising Sun 92 bpm"},
			wantBPM: 92,
			wantSrc: TempoFilenameGuess,
		},
		{
			name:    "default when nothing known",
			tab:     &model.Tab{Title: "No Tempo Here"},
			wantBPM: DefaultBPM,
			wantSrc: TempoDefault,
		},
		{
			name:     "nil tab falls through to default",
			audioBPM: 0,
			wantBPM:  DefaultBPM,
			wantSrc:  TempoDefault,
		},
		{
			name:     "audio sync is clamped",
			tab:      &model.Tab{},
			audioBPM: 500,
			wantBPM:  300,
			wantSrc:  TempoAudioSync,
		},
		{
			name:    "metadata bpm key wins over tempo key",
			tab:     &model.Tab{Metadata: map[string]string{model.MetaKeyBPM: "80", model.MetaKeyTempo: "90"}},
			wantBPM: 80,
			wantSrc: TempoMetadata,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bpm, src := ResolveTempo(c.tab, c.audioBPM, c.rememberd)
			if bpm != c.wantBPM || src != c.wantSrc {
				t.Fatalf("ResolveTempo = (%d, %s), want (%d, %s)", bpm, src, c.wantBPM, c.wantSrc)
			}
		})
	}
}

// TestTempoSourceString guards the stable identifiers used for machine-readable
// persistence (e.g. logs), independent of display-label wording.
func TestTempoSourceString(t *testing.T) {
	cases := []struct {
		src  TempoSource
		want string
	}{
		{TempoMetadata, "metadata"},
		{TempoUGVersion, "ug_version"},
		{TempoAudioSync, "audio_sync"},
		{TempoRemembered, "remembered"},
		{TempoFilenameGuess, "filename"},
		{TempoDefault, "default"},
	}
	for _, c := range cases {
		if got := c.src.String(); got != c.want {
			t.Fatalf("%v.String() = %q, want %q", c.src, got, c.want)
		}
	}
}

// TestTempoProvenanceLabels pins the exact short display labels the UI shows.
func TestTempoProvenanceLabels(t *testing.T) {
	cases := []struct {
		src  TempoSource
		want string
	}{
		{TempoMetadata, "metadata"},
		{TempoUGVersion, "from UG version"},
		{TempoAudioSync, "from audio sync"},
		{TempoRemembered, "remembered"},
		{TempoFilenameGuess, "from filename"},
		{TempoDefault, "default"},
	}
	for _, c := range cases {
		if got := TempoProvenanceLabel(c.src); got != c.want {
			t.Fatalf("TempoProvenanceLabel(%v) = %q, want %q", c.src, got, c.want)
		}
	}
}

// TestRecordTempoUsageRoundTrip verifies RecordTempoUsage persists the tempo
// into the explicit metadata keys and that ResolveTempo recalls it as the
// metadata rung on the next session.
func TestRecordTempoUsageRoundTrip(t *testing.T) {
	tab := &model.Tab{Title: "Song"}
	RecordTempoUsage(tab, 132)
	if got := tab.Metadata[model.MetaKeyTempo]; got != "132" {
		t.Fatalf("stored tempo key = %q, want 132", got)
	}
	if got := tab.Metadata[TempoSourceMetaKey]; got != "remembered" {
		t.Fatalf("stored source key = %q, want remembered", got)
	}
	bpm, src := ResolveTempo(tab, 0, 0)
	if bpm != 132 || src != TempoMetadata {
		t.Fatalf("round-trip resolve = (%d, %s), want (132, metadata)", bpm, src)
	}
}

// TestRecordTempoUsageKeepsProvenance verifies an existing metadata tempo keeps
// its own provenance label when a new usage is recorded.
func TestRecordTempoUsageKeepsProvenance(t *testing.T) {
	tab := &model.Tab{Metadata: map[string]string{model.MetaKeyTempo: "Based on the video. 107 bpm."}}
	RecordTempoUsage(tab, 110)
	if got := tab.Metadata[model.MetaKeyTempo]; got != "110" {
		t.Fatalf("stored tempo key = %q, want 110", got)
	}
	if got := tab.Metadata[TempoSourceMetaKey]; got != "from UG version" {
		t.Fatalf("stored source key = %q, want from UG version", got)
	}
}

// TestRecordTempoUsageGuards covers nil/zero inputs and clamping.
func TestRecordTempoUsageGuards(t *testing.T) {
	RecordTempoUsage(nil, 100) // must not panic

	tab := &model.Tab{}
	RecordTempoUsage(tab, 0) // zero is a no-op: no keys written
	if len(tab.Metadata) != 0 {
		t.Fatalf("zero bpm wrote metadata: %v", tab.Metadata)
	}

	RecordTempoUsage(tab, 500) // out-of-range value is clamped
	if got := tab.Metadata[model.MetaKeyTempo]; got != "300" {
		t.Fatalf("clamped tempo key = %q, want 300", got)
	}
}
