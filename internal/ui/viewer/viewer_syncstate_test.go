package viewer

import (
	"regexp"
	"testing"
	"time"

	"fretboard/internal/model"
	"fretboard/internal/player"
	"fretboard/internal/ui/msgs"
)

// stateTestModel returns a viewer mid-playback of a ready audio source: a
// tab loaded, an online source selected with a resolved path, and audio sync
// armed. The state tests mutate individual fields off this baseline.
func stateTestModel() ViewerModel {
	m := NewViewerModel()
	m.tab = &model.Tab{Title: "T", Artist: "A", Tuning: model.Standard,
		Metadata: map[string]string{},
		Bars:     []model.Bar{{Strings: []model.StringLine{{}}}}}
	m.playing = true
	m.audioSync = true
	m.audioCatalog = player.AudioCatalog{Sources: []player.AudioSource{
		{ID: "midi", Kind: player.SourceMIDI, Label: "MIDI"},
		{ID: "yt:official", Kind: player.SourceOnline, Label: "Official", Path: "/tmp/a.mp3"},
	}}
	m.selectedSourceIdx = 1
	m.resolvedAudio = "/tmp/a.mp3"
	return m
}

// TestSyncStateAxes guards the two-axis state machine: every Load state
// (5) and every Sync state (8) is reachable from the viewer fields, and the
// priority order of the Sync axis resolves the ambiguous combinations.
func TestSyncStateAxes(t *testing.T) {
	cases := []struct {
		name     string
		mutate   func(*ViewerModel)
		wantLoad string
		wantSync string
	}{
		// Load axis.
		{"load ready", func(m *ViewerModel) {}, loadReady, syncUnsynced},
		{"load no tab", func(m *ViewerModel) { m.tab = nil }, loadNoTab, syncOff},
		{"load midi", func(m *ViewerModel) { m.selectedSourceIdx = 0 }, loadMidi, syncUnsynced},
		{"load remote fetching catalog", func(m *ViewerModel) { m.fetchingCatalog = true }, loadRemote, syncUnsynced},
		{"load remote fetching audio", func(m *ViewerModel) { m.fetchingAudio = true }, loadRemote, syncUnsynced},
		{"load no source", func(m *ViewerModel) {
			m.resolvedAudio = ""
			m.audioCatalog = player.AudioCatalog{Sources: []player.AudioSource{
				{ID: "midi", Kind: player.SourceMIDI},
				{ID: "yt:online", Kind: player.SourceOnline, Label: "Online"},
			}}
		}, loadNoSource, syncUnsynced},
		{"load ready via catalog path", func(m *ViewerModel) {
			m.resolvedAudio = ""
			m.audioCatalog = player.AudioCatalog{Sources: []player.AudioSource{
				{ID: "midi", Kind: player.SourceMIDI},
				{ID: "local:x", Kind: player.SourceLocal, Label: "local.mp3", Path: "/tmp/x.mp3"},
			}}
		}, loadReady, syncUnsynced},

		// Sync axis.
		{"sync off paused", func(m *ViewerModel) { m.playing = false }, loadReady, syncOff},
		{"sync off midi", func(m *ViewerModel) { m.audioSync = false }, loadReady, syncOff},
		// F1 home: audio sync armed, no anchors, no auto tempo map.
		{"sync unsynced home", func(m *ViewerModel) {}, loadReady, syncUnsynced},
		{"sync anchor needed", func(m *ViewerModel) { m.audioOffset = 2.5 }, loadReady, syncAnchorNeeded},
		{"sync anchored", func(m *ViewerModel) {
			m.syncPoints = []player.SyncPoint{{Bar: 1, Seconds: 1}}
		}, loadReady, syncAnchored},
		{"sync auto", func(m *ViewerModel) { m.autoActive = true }, loadReady, syncAuto},
		{"sync drift", func(m *ViewerModel) { m.autoActive = true; m.syncDrift = 0.2 }, loadReady, syncDrift},
		{"sync loop", func(m *ViewerModel) { m.loopStartBar = 1; m.loopEndBar = 2 }, loadReady, syncLoop},
		{"sync ended", func(m *ViewerModel) { m.endBanner = true }, loadReady, syncEnded},

		// Sync priority order, first match wins.
		{"drift beats auto", func(m *ViewerModel) { m.autoActive = true; m.syncDrift = -0.1 }, loadReady, syncDrift},
		{"anchors beat auto map", func(m *ViewerModel) {
			m.autoActive = true
			m.syncPoints = []player.SyncPoint{{Bar: 1, Seconds: 1}}
		}, loadReady, syncAnchored},
		{"drift beats anchors", func(m *ViewerModel) {
			m.autoActive = true
			m.syncDrift = 0.2
			m.syncPoints = []player.SyncPoint{{Bar: 1, Seconds: 1}}
		}, loadReady, syncDrift},
		{"loop beats anchors", func(m *ViewerModel) {
			m.loopStartBar = 1
			m.loopEndBar = 2
			m.syncPoints = []player.SyncPoint{{Bar: 1, Seconds: 1}}
		}, loadReady, syncLoop},
		{"ended beats loop", func(m *ViewerModel) {
			m.endBanner = true
			m.loopStartBar = 1
			m.loopEndBar = 2
		}, loadReady, syncEnded},
		// The banner survives the stop that displayed it: the monitor sets
		// endBanner and then stops playback, so "ended" outranks "off".
		{"ended beats off", func(m *ViewerModel) { m.endBanner = true; m.playing = false }, loadReady, syncEnded},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := stateTestModel()
			tc.mutate(&m)
			got := syncStateOf(m)
			if got.load != tc.wantLoad || got.sync != tc.wantSync {
				t.Fatalf("syncState = [%s|%s], want [%s|%s]", got.load, got.sync, tc.wantLoad, tc.wantSync)
			}
		})
	}
}

// TestSyncStateFlags guards the orthogonal flags: calibrating and endBanner
// ride alongside the two axes without disturbing them.
func TestSyncStateFlags(t *testing.T) {
	// calibrating during SyncAnchored: the analysis is a background event,
	// orthogonal to the anchored state.
	m := stateTestModel()
	m.syncPoints = []player.SyncPoint{{Bar: 1, Seconds: 1}}
	m.calibrating = true
	s := syncStateOf(m)
	if s.sync != syncAnchored {
		t.Fatalf("sync = %q, want %q while calibrating", s.sync, syncAnchored)
	}
	if !s.calibrating {
		t.Fatal("calibrating flag must survive the state computation")
	}

	// endBanner during SyncEnded.
	m = stateTestModel()
	m.endBanner = true
	s = syncStateOf(m)
	if s.sync != syncEnded {
		t.Fatalf("sync = %q, want %q", s.sync, syncEnded)
	}
	if !s.endBanner {
		t.Fatal("endBanner flag must survive the state computation")
	}
}

// TestSyncStateLabel guards the rendered label: ASCII [load|sync] plus the
// "..." calibrating and "[end]" banner tags.
func TestSyncStateLabel(t *testing.T) {
	m := stateTestModel()
	if got := syncStateOf(m).label(); got != "[ready|unsynced]" {
		t.Fatalf("home label = %q, want [ready|unsynced]", got)
	}

	m = stateTestModel()
	m.syncPoints = []player.SyncPoint{{Bar: 1, Seconds: 1}}
	if got := syncStateOf(m).label(); got != "[ready|anchored]" {
		t.Fatalf("anchored label = %q, want [ready|anchored]", got)
	}

	m = stateTestModel()
	m.tab = nil
	if got := syncStateOf(m).label(); got != "[no-tab|off]" {
		t.Fatalf("no-tab label = %q, want [no-tab|off]", got)
	}

	m = stateTestModel()
	m.calibrating = true
	if got := syncStateOf(m).label(); got != "[ready|unsynced]..." {
		t.Fatalf("calibrating label = %q, want [ready|unsynced]...", got)
	}

	m = stateTestModel()
	m.endBanner = true
	if got := syncStateOf(m).label(); got != "[ready|ended][end]" {
		t.Fatalf("ended label = %q, want [ready|ended][end]", got)
	}
}

// TestSyncStateLabelAscii guards the no-emoji rule across every axis pair and
// flag combination: every rendered label is plain ASCII in the [load|sync]
// shape with only the [end] and ... tags appended.
func TestSyncStateLabelAscii(t *testing.T) {
	loads := []string{loadNoTab, loadMidi, loadRemote, loadNoSource, loadReady}
	syncs := []string{syncOff, syncUnsynced, syncAnchorNeeded, syncAnchored, syncAuto, syncDrift, syncLoop, syncEnded}
	shape := regexp.MustCompile(`^\[[a-z-]+\|[a-z-]+\](\[end\])?(\.\.\.)?$`)
	for _, l := range loads {
		for _, s := range syncs {
			for _, flags := range []syncState{
				{load: l, sync: s},
				{load: l, sync: s, calibrating: true},
				{load: l, sync: s, endBanner: true},
				{load: l, sync: s, calibrating: true, endBanner: true},
			} {
				if lab := flags.label(); !shape.MatchString(lab) {
					t.Fatalf("label %q must match the ASCII [load|sync][end]... shape", lab)
				}
			}
		}
	}
}

// TestCalibratingLifecycle guards the flag wiring: analysis commands set
// calibrating when emitted, and the landing messages clear it for the still-
// current source only.
func TestCalibratingLifecycle(t *testing.T) {
	// maybeAlignCmd sets calibrating when it returns a command for an
	// unaligned ready source.
	m, _ := bpmTestViewer(t)
	cmd := m.maybeAlignCmd()
	if cmd == nil {
		t.Fatal("maybeAlignCmd should return a command for an unaligned ready source")
	}
	if !m.calibrating {
		t.Fatal("maybeAlignCmd must set calibrating when a command is returned")
	}

	// A MIDI source gets no analysis: calibrating stays off.
	m = NewViewerModel()
	m.tab = &model.Tab{Title: "T", Artist: "A", Metadata: map[string]string{}}
	m.alignedSources = map[string]bool{}
	if cmd := m.maybeAlignCmd(); cmd != nil {
		t.Fatal("MIDI source must not produce an align command")
	}
	if m.calibrating {
		t.Fatal("a nil align command must not set calibrating")
	}

	// handleAlignment clears calibrating when the result lands for the
	// current source.
	m, audio := bpmTestViewer(t)
	m.calibrating = true
	updated, _ := m.Update(msgs.AlignmentMsg{
		SourceID: "local:" + audio, BPM: 118, Offset: 3200 * time.Millisecond,
		Confidence: 0.85, Artist: "Y", Title: "X", TabID: 0, TabPath: "x.txt",
	})
	m = updated
	if m.calibrating {
		t.Fatal("handleAlignment must clear calibrating when the analysis lands")
	}

	// ... but a stale source's result must not clear it.
	m, _ = bpmTestViewer(t)
	m.calibrating = true
	updated, _ = m.Update(msgs.AlignmentMsg{
		SourceID: "yt:other", BPM: 140, Offset: 0, Confidence: 0.9,
		Artist: "Y", Title: "X", TabID: 0, TabPath: "x.txt",
	})
	m = updated
	if !m.calibrating {
		t.Fatal("a stale-source analysis must not clear calibrating")
	}

	// handleIntroDetected clears calibrating for the current source.
	m, audio = bpmTestViewer(t)
	m.calibrating = true
	updated, _ = m.Update(msgs.IntroDetectedMsg{
		SourceID: "local:" + audio, Offset: 3 * time.Second,
		Artist: "Y", Title: "X", TabID: 0, TabPath: "x.txt",
	})
	m = updated
	if m.calibrating {
		t.Fatal("handleIntroDetected must clear calibrating")
	}

	// handleBPMDerived clears calibrating for the current source.
	m, audio = bpmTestViewer(t)
	m.calibrating = true
	updated, _ = m.Update(msgs.BPMDerivedMsg{SourceID: "local:" + audio, BPM: 132})
	m = updated
	if m.calibrating {
		t.Fatal("handleBPMDerived must clear calibrating")
	}
}

// TestEndBannerLifecycle guards the banner wiring: a new playback start
// clears a previously shown track-ended banner.
func TestEndBannerLifecycle(t *testing.T) {
	m := NewViewerModel()
	m.LoadTab(sampleTab(), "", 0)
	m.endBanner = true
	updated, _ := m.Update(msgs.PlaybackStartedMsg{
		Schedule: []player.PlaybackStep{{Bar: 0, Ticks: 480}}, StepIdx: 0,
		Duration: time.Millisecond, AudioSync: true,
	})
	m = updated
	if m.endBanner {
		t.Fatal("handlePlaybackStarted must clear endBanner")
	}
}
