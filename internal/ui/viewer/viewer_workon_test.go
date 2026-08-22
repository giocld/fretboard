package viewer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fretboard/internal/model"
	"fretboard/internal/parser"
	"fretboard/internal/player"
	"fretboard/internal/ui/msgs"
	tea "github.com/charmbracelet/bubbletea"
)

// chordTab returns a chord-sheet tab (kind=chords, raw text, no bars).
func chordTab() *model.Tab {
	return &model.Tab{
		Title:  "Chords",
		Artist: "A",
		Metadata: map[string]string{
			"kind": "chords",
			"raw":  "Intro:  Am   C    G\nVerse:  F#m7  B7   Em\n        [Am] [C]",
		},
	}
}

// mustParseTab parses tab text through the real parser for fixtures.
func mustParseTab(t *testing.T, text string) *model.Tab {
	t.Helper()
	tab, err := parser.Parse(strings.NewReader(text))
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	return tab
}

// twoBarTabText is a two-bar ASCII tab used by the edit tests: bar 1 is
// frets 0, bar 2 is frets 3 (D string edited to 5 in fixtures).
const twoBarTabText = "Song\nArtist\n\nE|--0--|--3--|\nB|--0--|--0--|\nG|--0--|--0--|\nD|--2--|--0--|\nA|--2--|--2--|\nE|--0--|--3--|\n"

func ctrlPKey() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyCtrlP}
}

// TestChordSheetTransposesOnlyChordWords guards S1.2: transposing a chord
// sheet shifts chord-shaped tokens (via parser.TransposeChord) and leaves
// non-chord text — lyrics, brackets, spacing — byte-identical.
func TestChordSheetTransposesOnlyChordWords(t *testing.T) {
	raw := "Verse:  Am   C    G\n        F#m7  B7   Em\n  (slowly)  D  \n"
	got := transposeChordSheetText(raw, 2)
	want := "Verse:  Bm   D    A\n        G#m7  C#7   F#m\n  (slowly)  E  \n"
	if got != want {
		t.Fatalf("transpose(+2) = %q, want %q", got, want)
	}
	// Zero transpose is the identity.
	if got := transposeChordSheetText(raw, 0); got != raw {
		t.Fatalf("transpose(0) must not change the text, got %q", got)
	}
}

// TestChordSheetModeDisablesPlayback guards S1.2: toggling playback on a
// chord sheet refuses honestly and never starts a session.
func TestChordSheetModeDisablesPlayback(t *testing.T) {
	m := NewViewerModel()
	m.LoadTab(chordTab(), "chords.txt", 0)
	if !m.chordSheet {
		t.Fatal("a kind=chords tab must enter chord-sheet mode")
	}
	if cmd := m.togglePlayback(); cmd != nil {
		t.Fatal("chord sheets must not produce a playback command")
	}
	if !strings.Contains(m.errMsg, "playback unavailable") {
		t.Fatalf("errMsg = %q, want the honest playback-unavailable hint", m.errMsg)
	}
	// The body renders the raw text verbatim.
	view := m.View()
	if !strings.Contains(view, "Intro:  Am   C    G") {
		t.Fatalf("chord-sheet body missing the raw text:\n%s", view)
	}
}

// TestQualityBadgeGuardsKind guards S1.1: the timing word renders only for
// kind=tab sheets with quality_timing metadata; missing metadata shows no
// badge.
func TestQualityBadgeGuardsKind(t *testing.T) {
	m := NewViewerModel()
	m.LoadTab(sampleTab(), "x.txt", 0)
	m.tab.Metadata["kind"] = "tab"
	m.tab.Metadata["quality_timing"] = "solid"
	if got := m.qualityTiming(); got != "solid" {
		t.Fatalf("qualityTiming = %q, want solid", got)
	}
	if view := m.View(); !strings.Contains(view, "timing: solid") {
		t.Fatalf("status missing the timing badge:\n%s", view)
	}
	// Chord sheets and missing metadata carry no badge.
	m.tab.Metadata["kind"] = "chords"
	if got := m.qualityTiming(); got != "" {
		t.Fatalf("chord sheets must not badge timing, got %q", got)
	}
	m.tab.Metadata["kind"] = "tab"
	delete(m.tab.Metadata, "quality_timing")
	if got := m.qualityTiming(); got != "" {
		t.Fatalf("missing metadata must yield no badge, got %q", got)
	}
}

// TestMidiPreviewLabel guards S2.3: MIDI playback is labelled "MIDI preview".
func TestMidiPreviewLabel(t *testing.T) {
	writeFakeFluidsynthTest(t)
	m := NewViewerModel()
	m.LoadTab(sampleTab(), "x.txt", 0)
	m.engine.Soundfont = "fake.sf2"
	m.engine.Volume = 80
	m, cmd := m.Update(key(" "))
	if cmd == nil {
		t.Fatal("space should start MIDI playback")
	}
	m, _ = m.Update(cmd())
	if !m.playing || m.engine.Mode() != "midi" {
		t.Fatalf("should be playing midi: playing=%v mode=%q", m.playing, m.engine.Mode())
	}
	if view := m.View(); !strings.Contains(view, "MIDI preview") {
		t.Fatalf("playing status missing the MIDI preview label:\n%s", view)
	}
	m.StopPlayback()
}

// TestTempoProvenanceResolvedAtPlayStart guards S2.1: starting playback
// resolves the tempo through the chain and remembers the provenance; ending
// playback records the used tempo.
func TestTempoProvenanceResolvedAtPlayStart(t *testing.T) {
	m := NewViewerModel()
	tab := &model.Tab{Title: "X", Tuning: model.Standard,
		Bars:     []model.Bar{{Strings: []model.StringLine{{Segments: []model.Segment{{Char: '0', Value: 0, Position: 0, Width: 1}}}}}},
		Metadata: map[string]string{},
	}
	m.LoadTab(tab, "x.txt", 0)
	m.derivedBPM = 96 // a measured audio tempo with no metadata tempo
	cmd := m.togglePlayback()
	if cmd == nil {
		t.Fatal("playback should start")
	}
	if !m.tempoSrcSet || player.TempoProvenanceLabel(m.tempoSrc) != "from audio sync" {
		t.Fatalf("tempo provenance = %v, want audio sync", m.tempoSrc)
	}
	if m.bpm != 96 {
		t.Fatalf("resolved bpm = %d, want the audio-synced 96", m.bpm)
	}
	// Ending the session records the tempo for the next one.
	m.playing = true
	m.tempoSrcSet = true
	m.stopPlayback()
	if got := tab.Metadata[model.MetaKeyTempo]; got != "96" {
		t.Fatalf("tempo metadata = %q, want the used 96 recorded", got)
	}
}

// TestManualBPMNudgeBeatsTheChain guards S2.1: a manual +/- nudge keeps the
// user's tempo across plays instead of the chain's value.
func TestManualBPMNudgeBeatsTheChain(t *testing.T) {
	m := NewViewerModel()
	tab := &model.Tab{Title: "X", Tuning: model.Standard,
		Bars:     []model.Bar{{Strings: []model.StringLine{{Segments: []model.Segment{{Char: '0', Value: 0, Position: 0, Width: 1}}}}}},
		Metadata: map[string]string{model.MetaKeyBPM: "140"},
	}
	m.LoadTab(tab, "x.txt", 0)
	m, _ = m.Update(key("+")) // user nudges to 145
	if m.bpm != 145 {
		t.Fatalf("nudged bpm = %d, want 145", m.bpm)
	}
	_ = m.togglePlayback() // chain says 140 (metadata), user says 145
	if m.bpm != 145 {
		t.Fatalf("manual nudge must win over the chain, got %d", m.bpm)
	}
}

// TestEditReapplyReplacesTab guards S1.3: after the editor writes new
// content, the re-parse replaces the tab, reports the changed bars, and
// emits msgs.EditPersistMsg for the app to persist.
func TestEditReapplyReplacesTab(t *testing.T) {
	m := NewViewerModel()
	dir := t.TempDir()
	path := filepath.Join(dir, "song.txt")
	if err := os.WriteFile(path, []byte(twoBarTabText), 0o644); err != nil {
		t.Fatal(err)
	}
	m.LoadTab(mustParseTab(t, twoBarTabText), path, 42)

	// The editor "writes" a modified version: bar 2's D string becomes a 5.
	edited := strings.Replace(twoBarTabText, "D|--2--|--0--|", "D|--2--|--5--|", 1)
	editPath := filepath.Join(dir, "edit.txt")
	if err := os.WriteFile(editPath, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	m.editing = true
	m.editPath = editPath
	m, cmd := m.Update(editDoneMsg{path: editPath})
	if cmd == nil {
		t.Fatal("the edit flow must emit a persistence message")
	}
	msg, ok := cmd().(msgs.EditPersistMsg)
	if !ok {
		t.Fatalf("cmd() = %T, want msgs.EditPersistMsg", msg)
	}
	if msg.TabID != 42 || msg.Path != path || msg.Content != edited {
		t.Fatalf("EditPersistMsg = %+v, want TabID 42, path %s, edited content", msg, path)
	}
	if !strings.Contains(m.infoMsg, "bar 2 modified") {
		t.Fatalf("infoMsg = %q, want a bar-2 modification summary", m.infoMsg)
	}
	// The loaded tab now shows the edited fret in bar 2. The parser may
	// reorder string rows by label, so scan for the edited value.
	found5 := false
	for _, sl := range m.tab.Bars[1].Strings {
		for _, seg := range sl.Segments {
			if seg.Value == 5 {
				found5 = true
			}
		}
	}
	if !found5 {
		t.Fatalf("edited bar 2 must contain a fret 5, got %+v", m.tab.Bars[1].Strings)
	}
}

// TestEditParseErrorKeepsOldTab guards S1.3: a re-parse failure keeps the
// previous tab and surfaces the parse error.
func TestEditParseErrorKeepsOldTab(t *testing.T) {
	m := NewViewerModel()
	dir := t.TempDir()
	path := filepath.Join(dir, "song.txt")
	if err := os.WriteFile(path, []byte(twoBarTabText), 0o644); err != nil {
		t.Fatal(err)
	}
	m.LoadTab(mustParseTab(t, twoBarTabText), path, 0)
	old := m.tab
	badPath := filepath.Join(dir, "bad.txt")
	// An emptied edit parses to a tab with no bars and no chord kind — the
	// guard treats it as a failed edit.
	if err := os.WriteFile(badPath, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	m, _ = m.Update(editDoneMsg{path: badPath})
	if m.tab != old {
		t.Fatal("a failed re-parse must keep the previous tab")
	}
	if !strings.Contains(m.errMsg, "re-parse failed") {
		t.Fatalf("errMsg = %q, want the re-parse failure", m.errMsg)
	}
}

// TestPrintWritesHTMLNextToTab guards S1.4: ctrl+p writes the HTML export
// into the tab's directory and names the written path in the status line.
func TestPrintWritesHTMLNextToTab(t *testing.T) {
	m := NewViewerModel()
	dir := t.TempDir()
	path := filepath.Join(dir, "song.txt")
	if err := os.WriteFile(path, []byte(twoBarTabText), 0o644); err != nil {
		t.Fatal(err)
	}
	m.LoadTab(mustParseTab(t, twoBarTabText), path, 0)
	m, _ = m.Update(ctrlPKey())
	if m.errMsg != "" {
		t.Fatalf("ctrl+p surfaced an error: %q", m.errMsg)
	}
	if !strings.Contains(m.infoMsg, ".html") {
		t.Fatalf("infoMsg = %q, want the written html path", m.infoMsg)
	}
	data, err := os.ReadFile(filepath.Join(dir, "song.html"))
	if err != nil {
		t.Fatalf("html export missing: %v", err)
	}
	if !strings.Contains(string(data), "<html") {
		t.Fatalf("export is not html:\n%s", data)
	}
}

// TestPickerShowsPickReasonUnderSelection guards S3.2: the selected source's
// one-liner reason renders under its row.
func TestPickerShowsPickReasonUnderSelection(t *testing.T) {
	cat := player.AudioCatalog{Sources: []player.AudioSource{
		{ID: "midi", Kind: player.SourceMIDI, Label: "MIDI"},
		{ID: "yt:o", Kind: player.SourceOnline, Label: "Official", PickReason: "UG Pro copy fell through — used the community tab"},
	}}
	body := renderAudioPickerBody(cat, 1, false, false, 1, nil)
	if !strings.Contains(body, "UG Pro copy fell through") {
		t.Fatalf("picker body missing the selected source's reason:\n%s", body)
	}
}

// TestConfirmSourcePinsVideo guards S3.2: confirming a YouTube source pins
// its video, and the pinned video wins the next auto-pick.
func TestConfirmSourcePinsVideo(t *testing.T) {
	m := NewViewerModel()
	tab := &model.Tab{Title: "X", Artist: "Y", Tuning: model.Standard,
		Bars:     []model.Bar{{Strings: []model.StringLine{{Segments: []model.Segment{{Char: '0', Value: 0, Position: 0, Width: 1}}}}}},
		Metadata: map[string]string{},
	}
	m.LoadTab(tab, "x.txt", 0)
	m.audioCatalog = player.AudioCatalog{Sources: []player.AudioSource{
		{ID: "midi", Kind: player.SourceMIDI, Label: "MIDI"},
		{ID: "yt:live", Kind: player.SourceOnline, Label: "Live", VideoID: "livevid", Category: player.CatLive},
		{ID: "yt:studio", Kind: player.SourceOnline, Label: "Studio", VideoID: "studiovid", Category: player.CatOfficial, StrictOK: true},
	}}
	m.audioCursor = 2
	m.showAudioPicker = true
	m, _ = m.Update(key("enter"))
	if id, ok := player.PinnedVideoFor(m.tab); !ok || id != "studiovid" {
		t.Fatalf("pinned video = %q (ok=%v), want studiovid", id, ok)
	}
	if got := pickAudioSourceIndex(m.tab, m.audioCatalog); got != 2 {
		t.Fatalf("pinned pick = %d, want the pinned studio index 2", got)
	}
}

// TestVerifyChipAutoKeeps guards S4.1: a pending verification session
// auto-keeps after its deadline via the UI tick, and the chip disappears.
func TestVerifyChipAutoKeeps(t *testing.T) {
	m := NewViewerModel()
	m.LoadTab(sampleTab(), "x.txt", 0)
	m.startVerifySession(4, 3200*time.Millisecond)
	if m.verify == nil || m.verify.State != player.VerifyPending {
		t.Fatal("verification session should start pending")
	}
	if view := m.View(); !strings.Contains(view, "verify?") {
		t.Fatalf("status missing the verify chip:\n%s", view)
	}
	// Roll the deadline into the past and tick.
	m.verify.AutoKeepAt = time.Now().Add(-time.Second)
	m, _ = m.Update(verifyTickMsg{})
	if m.verify != nil {
		t.Fatal("the auto-kept session must be removed from the model")
	}
	if view := m.View(); strings.Contains(view, "verify?") {
		t.Fatalf("verify chip must auto-remove after keeping:\n%s", view)
	}
}

// TestSyncDuringPendingRefines guards S4.1: pressing s while the
// verification is pending cancels the passive keep and switches to manual.
func TestSyncDuringPendingRefines(t *testing.T) {
	m := NewViewerModel()
	m.LoadTab(sampleTab(), "x.txt", 0)
	m.startVerifySession(3, time.Second)
	m, _ = m.Update(key("s"))
	if m.verify != nil {
		t.Fatal("s during pending must clear the verification session")
	}
}

// TestPausedCalibrationRejectsInsaneAnchor guards S4.2: a paused anchor
// implying a <40/>300 BPM tempo is rejected with the reason shown.
func TestPausedCalibrationRejectsInsaneAnchor(t *testing.T) {
	m := NewViewerModel()
	m.LoadTab(sampleTab(), "x.txt", 0)
	// The existing pair (bar 1 at 0s, bar 2 at 1000s) already implies a
	// sub-1 BPM tempo; the sanity gate must reject the new anchor outright.
	m.syncPoints = []player.SyncPoint{{Bar: 1, Seconds: 0}, {Bar: 2, Seconds: 1000}}
	m.schedule = tempoMapSchedule()[:8] // 2 bars of 480-tick quarters at 120 BPM
	m.bpm = 120
	m.cursorBar = 2 // user bar 3
	m, _ = m.setPausedSyncPoint()
	if len(m.syncPoints) != 2 {
		t.Fatalf("an insane anchor must be rejected, got %+v", m.syncPoints)
	}
	if !strings.Contains(m.errMsg, "rejected") {
		t.Fatalf("errMsg = %q, want the rejection reason", m.errMsg)
	}
}

// TestPausedCalibrationKeepsDeviatingAnchorWithWarning guards S4.2: a
// suspicious-but-possible anchor is kept with an inline amber warning.
func TestPausedCalibrationKeepsDeviatingAnchorWithWarning(t *testing.T) {
	m := NewViewerModel()
	m.LoadTab(sampleTab(), "x.txt", 0)
	m.bpm = 120
	m.syncPoints = []player.SyncPoint{{Bar: 1, Seconds: 0}}
	// Bar 2 maps to 1s (two 480-tick quarters at 120 BPM): implied 240 BPM,
	// within the 40..300 band but 100% off the tab tempo -> kept + warned.
	m.schedule = []player.PlaybackStep{
		{Bar: 0, Col: 0, Ticks: 480}, {Bar: 0, Col: 4, Ticks: 480},
		{Bar: 1, Col: 0, Ticks: 480}, {Bar: 1, Col: 4, Ticks: 480},
	}
	m.cursorBar = 1 // user bar 2
	m, _ = m.setPausedSyncPoint()
	if len(m.syncPoints) != 2 {
		t.Fatalf("a deviating but plausible anchor must be kept, got %+v", m.syncPoints)
	}
	if !strings.Contains(m.warnMsg, "verify") {
		t.Fatalf("warnMsg = %q, want the amber deviation warning", m.warnMsg)
	}
}

// TestUndoRemovesLastAnchor guards S4.2: U removes the most recent anchor.
func TestUndoRemovesLastAnchor(t *testing.T) {
	m := NewViewerModel()
	m.LoadTab(sampleTab(), "x.txt", 0)
	m.syncPoints = []player.SyncPoint{{Bar: 1, Seconds: 0.5}, {Bar: 3, Seconds: 12}}
	m, _ = m.Update(key("U"))
	if len(m.syncPoints) != 1 || m.syncPoints[0].Bar != 1 {
		t.Fatalf("U should drop the last anchor, got %+v", m.syncPoints)
	}
	if !strings.Contains(m.infoMsg, "Undid sync anchor at bar 3") {
		t.Fatalf("infoMsg = %q, want the undo message", m.infoMsg)
	}
	m, _ = m.Update(key("U"))
	if len(m.syncPoints) != 0 {
		t.Fatalf("second U should drop the remaining anchor, got %+v", m.syncPoints)
	}
	m, _ = m.Update(key("U"))
	if !strings.Contains(m.errMsg, "No sync anchors") {
		t.Fatalf("U on empty anchors should say so, got %q", m.errMsg)
	}
}

// TestCalibrationChipRendering guards S4.3: with audio loaded the chip shows
// offset, anchors, drift, and the color-coded state word.
func TestCalibrationChipRendering(t *testing.T) {
	m := NewViewerModel()
	dir := t.TempDir()
	audio := filepath.Join(dir, "backing.mp3")
	if err := os.WriteFile(audio, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	m.LoadTab(sampleTab(), "x.txt", 0)
	m.audioCatalog = player.AudioCatalog{Sources: []player.AudioSource{
		{ID: "midi", Kind: player.SourceMIDI, Label: "MIDI"},
		{ID: "local:" + audio, Kind: player.SourceLocal, Label: "backing.mp3", Path: audio},
	}}
	m.selectedSourceIdx = 1
	m.audioOffset = 0.4
	m.syncPoints = []player.SyncPoint{{Bar: 1, Seconds: 0.4}, {Bar: 3, Seconds: 5}}
	chip := m.calibrationChip()
	if chip == "" {
		t.Fatal("chip must render for a loaded audio source")
	}
	for _, want := range []string{"+0.4s", "2 anchors", "calibrated"} {
		if !strings.Contains(chip, want) {
			t.Fatalf("chip %q missing %q", chip, want)
		}
	}
	// MIDI-only: no chip.
	m.selectedSourceIdx = 0
	if m.calibrationChip() != "" {
		t.Fatal("no chip without audio")
	}
}

// TestProAndReconstructedBadges guards S5.1: pro and songsterr-reconstructed
// tabs carry their badges in the header.
func TestProAndReconstructedBadges(t *testing.T) {
	m := NewViewerModel()
	tab := sampleTab()
	tab.Metadata = map[string]string{"pro": "1", "reconstructed": "1", "source": "songsterr-via-ug"}
	m.LoadTab(tab, "x.txt", 0)
	view := m.View()
	for _, want := range []string{"pro", "songsterr (reconstructed)"} {
		if !strings.Contains(view, want) {
			t.Fatalf("header missing %q:\n%s", want, view)
		}
	}
}

// TestPickReasonShownInStatus guards S5.2: a fetch fallback reason surfaces
// in the status line.
func TestPickReasonShownInStatus(t *testing.T) {
	m := NewViewerModel()
	tab := sampleTab()
	tab.Metadata = map[string]string{"pick_reason": "UG Pro version requires a subscription — used the community tab"}
	m.LoadTab(tab, "x.txt", 0)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	view := m.View()
	if !strings.Contains(view, "UG Pro version requires a subscription") {
		t.Fatalf("status missing the pick reason:\n%s", view)
	}
}

// TestGPtracksMetadataParsed guards S5.3b: the "tracks" metadata key is
// parsed into the switcher and the active track name shows in the status.
func TestGPtracksMetadataParsed(t *testing.T) {
	m := NewViewerModel()
	tab := sampleTab()
	tab.Metadata = map[string]string{
		"tracks": `[{"name":"Guitar","instrument":"Steel Guitar","strings":6,"tuning":"EADGBE"},{"name":"Bass","instrument":"Bass","strings":4,"tuning":"EADG"}]`,
	}
	m.LoadTab(tab, "song.gp", 0)
	if len(m.gpTracks) != 2 {
		t.Fatalf("gpTracks = %+v, want 2 parsed tracks", m.gpTracks)
	}
	if name := m.currentTrackName(); name != "Guitar" {
		t.Fatalf("current track = %q, want Guitar", name)
	}
	if view := m.View(); !strings.Contains(view, "track: Guitar") {
		t.Fatalf("status missing the track name:\n%s", view)
	}
	// t without a multi-track file says so.
	m2 := NewViewerModel()
	m2.LoadTab(sampleTab(), "x.txt", 0)
	m2, _ = m2.Update(key("t"))
	if !strings.Contains(m2.errMsg, "No multi-track") {
		t.Fatalf("t on a single-track tab should say so, got %q", m2.errMsg)
	}
}

// TestDrumsDisableTranspose guards S5.3a: a drum tab labels itself and
// blocks T/Z transpose.
func TestDrumsDisableTranspose(t *testing.T) {
	m := NewViewerModel()
	tab := &model.Tab{Title: "Drum Track", Tuning: model.Standard,
		Bars:     []model.Bar{{Strings: []model.StringLine{{Segments: []model.Segment{{Char: 'x', Value: -1, Position: 0, Width: 1}}}}}},
		Metadata: map[string]string{},
	}
	m.LoadTab(tab, "drums.txt", 0)
	// The player-side detector flags x/o hits in >=2 string rows; seed the
	// cached flag directly so the test does not depend on the detector.
	m.drums = true
	if !m.isDrums() {
		t.Fatal("drums flag must be set")
	}
	m, _ = m.Update(key("T"))
	if m.transpose != 0 {
		t.Fatalf("T must not transpose a drum tab, got %d", m.transpose)
	}
	if !strings.Contains(m.errMsg, "disabled for drum tabs") {
		t.Fatalf("errMsg = %q, want the drums hint", m.errMsg)
	}
	// The header labels the drum part.
	if view := m.View(); !strings.Contains(view, "drums") {
		t.Fatalf("header missing the drums label:\n%s", view)
	}
}

// TestPracticeSessionRampsAndReports guards S8.1: loop passes count, the
// tempo ramps +5% every 3 clean loops toward the target, and exiting emits
// msgs.PracticeSessionMsg with the summary card.
func TestPracticeSessionRampsAndReports(t *testing.T) {
	m := NewViewerModel()
	m.LoadTab(sampleTab(), "x.txt", 7)
	m.loopStartBar, m.loopEndBar = 1, 2
	m.bpm = 120
	m.startSession()
	m.sessionTargetBPM = 132
	if !m.sessionMode {
		t.Fatal("M should enter session mode")
	}
	// 3 clean loops ramp 120 -> 126 (120 + max(5, 6%)) = 126.
	for i := 0; i < 3; i++ {
		m.noteLoopPass()
	}
	if m.bpm != 126 {
		t.Fatalf("bpm after 3 loops = %d, want 126", m.bpm)
	}
	if m.sessionLoops != 3 {
		t.Fatalf("sessionLoops = %d, want 3", m.sessionLoops)
	}
	// 3 more loops ramp 126 -> 132 (capped at the target).
	for i := 0; i < 3; i++ {
		m.noteLoopPass()
	}
	if m.bpm != 132 {
		t.Fatalf("bpm after 6 loops = %d, want the capped target 132", m.bpm)
	}
	// Exit: summary card + PracticeSessionMsg.
	m2, cmd := m.endSessionCmd()
	if cmd == nil {
		t.Fatal("a session with loops must emit PracticeSessionMsg")
	}
	msg, ok := cmd().(msgs.PracticeSessionMsg)
	if !ok {
		t.Fatalf("cmd() = %T, want msgs.PracticeSessionMsg", msg)
	}
	if msg.TabID != 7 || msg.Loops != 6 || msg.TempoBPM != 132 {
		t.Fatalf("PracticeSessionMsg = %+v, want TabID 7, 6 loops, 132 bpm", msg)
	}
	if m2.sessionMode {
		t.Fatal("session must end")
	}
	if !strings.Contains(m2.sessionCard, "6 loops") || !strings.Contains(m2.sessionCard, "120→132 bpm") {
		t.Fatalf("session card = %q, want the loop/bpm summary", m2.sessionCard)
	}
}

// TestSessionModeNeedsLoop guards S8.1: entering session mode requires an
// A-B loop and a paused state.
func TestSessionModeNeedsLoop(t *testing.T) {
	m := NewViewerModel()
	m.LoadTab(sampleTab(), "x.txt", 0)
	m, _ = m.Update(key("M"))
	if m.sessionMode {
		t.Fatal("M without a loop must not enter session mode")
	}
	if !strings.Contains(m.errMsg, "Set a loop first") {
		t.Fatalf("errMsg = %q, want the set-a-loop hint", m.errMsg)
	}
	m, _ = m.setLoopPoint(true)
	m, _ = m.setLoopPoint(false)
	m, _ = m.Update(key("M"))
	if !m.sessionMode {
		t.Fatal("M with a loop set must enter session mode")
	}
}

// TestDownloadProgressState guards S3.3: the shared slot reports active
// downloads, the package hook routes progress lines into it, and end clears
// both.
func TestDownloadProgressState(t *testing.T) {
	m := NewViewerModel()
	m.LoadTab(sampleTab(), "x.txt", 0)
	st := m.downloadState()
	if active, _ := st.get(); active {
		t.Fatal("no download active initially")
	}
	st.begin()
	defer st.end()
	player.NotifyDownloadProgress("[download]  45.0% of 4.21MiB at 1.23MiB/s ETA 00:02")
	active, pct := st.get()
	if !active || pct != 45 {
		t.Fatalf("download state = (%v, %.0f), want (true, 45)", active, pct)
	}
	// The status row renders the percentage while active.
	m.download = st
	if view := m.View(); !strings.Contains(view, "downloading 45%") {
		t.Fatalf("status missing the download progress:\n%s", view)
	}
	st.end()
	if active, _ := st.get(); active {
		t.Fatal("end must clear the active flag")
	}
	if player.OnDownloadProgress != nil {
		t.Fatal("end must reset the package hook to nil")
	}
}

// TestCacheScreenDelete guards S3.3: d deletes the selected cache entry and
// D clears the cache, with the screen re-listing after each action. The
// config dir is redirected to a temp dir so the real user cache is untouched.
func TestCacheScreenDelete(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := NewViewerModel()
	m.LoadTab(sampleTab(), "x.txt", 0)
	m, _ = m.Update(key("K"))
	if !m.showCache {
		t.Fatal("K should open the cache screen")
	}
	entries, _, dir := player.CacheStats()
	if dir == "" {
		t.Fatal("cache dir should resolve under the temp config")
	}
	_ = entries
	// Seed two fake cache entries.
	for _, name := range []string{"a.mp3", "b.mp3"} {
		if err := os.WriteFile(filepath.Join(dir, name), make([]byte, 100), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m.loadCacheList()
	if len(m.cacheRows) != 2 {
		t.Fatalf("cache rows = %d, want 2", len(m.cacheRows))
	}
	body := renderCacheScreen(m)
	if !strings.Contains(body, "a.mp3") || !strings.Contains(body, "b.mp3") {
		t.Fatalf("cache screen missing entries:\n%s", body)
	}
	m.cacheCur = 1
	m, _ = m.Update(key("d"))
	if len(m.cacheRows) != 1 || m.cacheRows[0].name != "a.mp3" {
		t.Fatalf("d should delete the selected entry, got %+v", m.cacheRows)
	}
	if _, err := os.Stat(filepath.Join(dir, "b.mp3")); !os.IsNotExist(err) {
		t.Fatal("b.mp3 must be gone from disk")
	}
	m, _ = m.Update(key("D"))
	if len(m.cacheRows) != 0 {
		t.Fatalf("D should clear the cache, got %+v", m.cacheRows)
	}
	m, _ = m.Update(key("esc"))
	if m.showCache {
		t.Fatal("esc should close the cache screen")
	}
}

// TestFooterHintsContextual guards S8.3: the footer hint set matches the
// viewer state (playing vs session vs idle).
func TestFooterHintsContextual(t *testing.T) {
	m := NewViewerModel()
	m.LoadTab(sampleTab(), "x.txt", 0)
	if got := m.footerHints(); len(got) < 4 {
		t.Fatalf("idle hints too few: %+v", got)
	}
	m.playing = true
	playing := m.footerHints()
	found := false
	for _, h := range playing {
		if h.Key == "Space/p" {
			found = true
		}
	}
	if !found {
		t.Fatalf("playing hints missing the pause key: %+v", playing)
	}
	m.playing = false
	m.sessionMode = true
	sess := m.footerHints()
	if sess[0].Key != "M" || sess[0].Label != "exit" {
		t.Fatalf("session hints should lead with exit: %+v", sess)
	}
}
