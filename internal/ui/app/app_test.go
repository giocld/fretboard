package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"fretboard/internal/config"
	"fretboard/internal/diag"
	"fretboard/internal/library"
	"fretboard/internal/model"
	"fretboard/internal/testutil"
	"fretboard/internal/ui/msgs"
	"fretboard/internal/watcher"
	tea "github.com/charmbracelet/bubbletea"
)

func TestAppSearchQueryQDoesNotQuit(t *testing.T) {
	a := NewApp()
	a.view = viewSearch
	model, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	a = model.(AppModel)
	if cmd != nil {
		if _, isQuit := cmd().(tea.QuitMsg); isQuit {
			t.Fatal("typing q in search query should not quit")
		}
	}
	if a.search.QueryValue() != "q" {
		t.Fatalf("input value = %q, want %q", a.search.QueryValue(), "q")
	}
}

func TestAppLibraryFilterQDoesNotQuit(t *testing.T) {
	a := NewApp()
	a.view = viewLibrary
	a.library.SetSearchActive(true)
	model, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	a = model.(AppModel)
	if cmd != nil {
		if _, isQuit := cmd().(tea.QuitMsg); isQuit {
			t.Fatal("typing q in library filter should not quit")
		}
	}
	if a.library.FilterValue() != "q" {
		t.Fatalf("searchInput = %q, want %q", a.library.FilterValue(), "q")
	}
}

func TestAppQQuitsWhenNoInputActive(t *testing.T) {
	a := NewApp()
	_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("q on home should quit")
	}
	if _, isQuit := cmd().(tea.QuitMsg); !isQuit {
		t.Fatalf("expected QuitMsg, got %#v", cmd())
	}
}

func TestAppShutdownMsgQuits(t *testing.T) {
	a := NewApp()
	model, cmd := a.Update(msgs.ShutdownMsg{})
	a = model.(AppModel)
	if cmd == nil {
		t.Fatal("ShutdownMsg should return a quit command")
	}
	if _, isQuit := cmd().(tea.QuitMsg); !isQuit {
		t.Fatalf("expected QuitMsg, got %#v", cmd())
	}
	// Shutdown must be idempotent: a second ShutdownMsg still quits cleanly.
	if _, cmd = a.Update(msgs.ShutdownMsg{}); cmd == nil {
		t.Fatal("second ShutdownMsg should still return a quit command")
	}
}

func TestAppQuestionMarkTypesInSearchQuery(t *testing.T) {
	a := NewApp()
	a.view = viewSearch
	model, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	a = model.(AppModel)
	if a.view != viewSearch {
		t.Fatalf("view = %v, want viewSearch", a.view)
	}
	if cmd != nil {
		if _, isQuit := cmd().(tea.QuitMsg); isQuit {
			t.Fatal("typing ? in search query should not quit")
		}
	}
	if a.search.QueryValue() != "?" {
		t.Fatalf("input value = %q, want %q", a.search.QueryValue(), "?")
	}
}

// TestRestoreSessionOpensLastTab guards G4.1: a persisted session reopens
// the tab at the saved cursor bar with the saved settings.
func TestRestoreSessionOpensLastTab(t *testing.T) {
	testutil.RedirectConfigDir(t)
	st, err := library.NewStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	id, err := st.Import("s.txt", &model.Tab{Title: "Sultans", Artist: "Dire Straits", Tuning: model.Standard,
		Bars: []model.Bar{
			{Number: 1, Strings: []model.StringLine{{Segments: []model.Segment{{Char: '0', Value: 0, Position: 0, Width: 1}}}}},
			{Number: 2, Strings: []model.StringLine{{Segments: []model.Segment{{Char: '3', Value: 3, Position: 0, Width: 1}}}}},
		}})
	if err != nil {
		t.Fatal(err)
	}
	if err := config.SaveSession(config.Session{TabID: id, Bar: 2, BPM: 96, Linear: true}); err != nil {
		t.Fatal(err)
	}

	m := NewAppWithOptions(st, nil, "", nil)
	cmd := m.RestoreSession()
	if cmd == nil {
		t.Fatal("RestoreSession should return a startup command")
	}
	if m.viewer.Tab() == nil {
		t.Fatal("session tab should be loaded")
	}
	if m.viewer.CursorBar() != 1 {
		t.Fatalf("cursor should restore to bar 2 (0-based 1), got %d", m.viewer.CursorBar())
	}
	if m.viewer.BPM() != 96 || !m.viewer.Linear() {
		t.Fatalf("settings not restored: bpm=%d linear=%v", m.viewer.BPM(), m.viewer.Linear())
	}
}

// TestShutdownSavesSession guards G4.1: quitting persists the open tab and
// its cursor position.
func TestShutdownSavesSession(t *testing.T) {
	testutil.RedirectConfigDir(t)
	st, err := library.NewStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	id, err := st.Import("s.txt", &model.Tab{Title: "Sultans", Artist: "Dire Straits", Tuning: model.Standard,
		Bars: []model.Bar{{Number: 1, Strings: []model.StringLine{{Segments: []model.Segment{{Char: '0', Value: 0, Position: 0, Width: 1}}}}}}})
	if err != nil {
		t.Fatal(err)
	}
	m := NewAppWithOptions(st, nil, "", nil)
	cmd := m.openTab(id)
	_ = cmd
	m.viewer.SetCursorBar(0)
	m.viewer.SetBPM(132)

	m.Shutdown()

	s := config.LoadSession()
	if s.TabID != id || s.BPM != 132 || s.Bar != 1 {
		t.Fatalf("session not saved on shutdown: %+v", s)
	}
}

// TestSettingsScreenRoundTrip guards G6: opening settings from home, changing
// values, and going back applies them live and persists the config.
func TestSettingsScreenRoundTrip(t *testing.T) {
	testutil.RedirectConfigDir(t)
	a := NewApp()
	a.view = viewHome

	model, _ := a.Update(msgs.HomeSettingsMsg{})
	a = model.(AppModel)
	if a.view != viewSettings {
		t.Fatalf("HomeSettingsMsg should open settings, view=%d", a.view)
	}

	// Strict toggle, volume down twice, strict on again, theme cycle.
	for _, k := range []string{"j", "enter", "k", "left", "left", "j", "enter", "j", "right"} {
		model, _ := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
		a = model.(AppModel)
	}
	cfg := a.settings.Config()
	if cfg.VolumePercent != 60 {
		t.Fatalf("volume should be 60 after two lefts, got %d", cfg.VolumePercent)
	}
	if !cfg.StrictAudioSelection {
		t.Fatal("strict audio should have toggled back on")
	}

	// Back applies and persists.
	model, cmd := a.Update(msgs.SettingsBackMsg{})
	a = model.(AppModel)
	_ = cmd
	if a.view != viewHome {
		t.Fatalf("back should return home, view=%d", a.view)
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.VolumePercent != 60 || !loaded.StrictAudioSelection {
		t.Fatalf("config not persisted: %+v", loaded)
	}
}

// TestSettingsFromLibraryKey guards the S key in the browser.
func TestSettingsFromLibraryKey(t *testing.T) {
	a := NewApp()
	a.view = viewLibrary
	model, _ := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("S")})
	a = model.(AppModel)
	if a.view != viewSettings {
		t.Fatalf("S in the library should open settings, view=%d", a.view)
	}
}

// ---------------------------------------------------------------------------
// Wave-2 features: consent (3.4), first-run tour (8.3), help filtering
// (8.3), degraded-mode probe (8.2), GP track picker (5.3b), edit + practice
// persistence (7).
// ---------------------------------------------------------------------------

// forceOnline makes the consent gate behave as if yt-dlp were installed,
// so the consent-state tests are hermetic on machines without it. It also
// shims a fake yt-dlp onto PATH: BeginAudioFetch re-checks the player
// package's own yt-dlp lookup before taking the online branch, so the host
// install would otherwise decide whether Accept proceeds with a fetch.
func forceOnline(t *testing.T) {
	t.Helper()
	old := onlineAudioAvailable
	onlineAudioAvailable = func() bool { return true }
	t.Cleanup(func() { onlineAudioAvailable = old })

	dir := t.TempDir()
	name := "yt-dlp"
	script := "#!/bin/sh\necho 2026.01.01\n"
	if runtime.GOOS == "windows" {
		name += ".cmd"
		script = "@echo off\r\necho 2026.01.01\r\n"
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// newTestStore opens an isolated library store for a test.
func newTestStore(t *testing.T) *library.Store {
	t.Helper()
	st, err := library.NewStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func testTab() *model.Tab {
	return &model.Tab{Title: "Sultans", Artist: "Dire Straits", Tuning: model.Standard,
		Bars: []model.Bar{{Number: 1, Strings: []model.StringLine{{Segments: []model.Segment{{Char: '0', Value: 0, Position: 0, Width: 1}}}}}}}
}

// TestConsentAcceptPersistsAndFetches guards 3.4: the first online-audio
// use raises the consent overlay; Accept persists ConsentOnlineAudio and
// proceeds with the online fetch.
func TestConsentAcceptPersistsAndFetches(t *testing.T) {
	testutil.RedirectConfigDir(t)
	forceOnline(t)
	st := newTestStore(t)
	id, err := st.Import("s.txt", testTab())
	if err != nil {
		t.Fatal(err)
	}

	a := NewAppWithOptions(st, nil, "", nil)
	model, _ := a.Update(msgs.TabSelectedMsg{ID: id})
	a = model.(AppModel)
	if !a.consentPending {
		t.Fatal("opening a tab with auto-fetch and no consent should raise the consent overlay")
	}
	if !strings.Contains(a.View(), "Online audio consent") {
		t.Error("the consent overlay should render")
	}

	model, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	a = model.(AppModel)
	if a.consentPending {
		t.Fatal("accept should close the consent overlay")
	}
	if cmd == nil {
		t.Fatal("accept should proceed with the audio fetch")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.ConsentOnlineAudio {
		t.Fatal("accept should persist ConsentOnlineAudio=true")
	}

	// A second tab open never re-asks.
	model, _ = a.Update(msgs.TabSelectedMsg{ID: id})
	a = model.(AppModel)
	if a.consentPending {
		t.Fatal("consent should be one-time after accept")
	}
}

// TestConsentDeclineDisablesOnlineForSession guards 3.4: Decline closes the
// overlay, pins the session to local-only, and never re-prompts.
func TestConsentDeclineDisablesOnlineForSession(t *testing.T) {
	testutil.RedirectConfigDir(t)
	forceOnline(t)
	st := newTestStore(t)
	id, err := st.Import("s.txt", testTab())
	if err != nil {
		t.Fatal(err)
	}

	a := NewAppWithOptions(st, nil, "", nil)
	model, _ := a.Update(msgs.TabSelectedMsg{ID: id})
	a = model.(AppModel)
	if !a.consentPending {
		t.Fatal("opening a tab should raise the consent overlay")
	}

	model, _ = a.Update(tea.KeyMsg{Type: tea.KeyEsc})
	a = model.(AppModel)
	if a.consentPending {
		t.Fatal("decline should close the consent overlay")
	}
	if !a.consentDeclined {
		t.Fatal("decline should pin the session to local-only")
	}
	// BeginAudioFetch(false) may return nil when there is nothing local to
	// fetch; the observable contract is the session-local pin above.
	cfg, _ := config.Load()
	if cfg.ConsentOnlineAudio {
		t.Fatal("decline must not persist consent")
	}

	// The next tab open does not re-prompt (session-scoped decline).
	model, _ = a.Update(msgs.TabSelectedMsg{ID: id})
	a = model.(AppModel)
	if a.consentPending {
		t.Fatal("decline should not re-prompt within the session")
	}
}

// TestConsentNotRaisedWhenAutoFetchOff guards the gate: no auto-fetch, no
// consent screen.
func TestConsentNotRaisedWhenAutoFetchOff(t *testing.T) {
	testutil.RedirectConfigDir(t)
	forceOnline(t)
	st := newTestStore(t)
	id, err := st.Import("s.txt", testTab())
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := config.Load()
	cfg.AutoFetchAudio = false
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	a := NewAppWithOptions(st, nil, "", nil)
	model, _ := a.Update(msgs.TabSelectedMsg{ID: id})
	a = model.(AppModel)
	if a.consentPending {
		t.Fatal("no auto-fetch should not raise the consent overlay")
	}
}

// TestTourTransitions guards 8.3: the tour walks three cards, completes on
// the last Enter, skips on Esc, and both paths persist TourSeen.
func TestTourTransitions(t *testing.T) {
	testutil.RedirectConfigDir(t)
	a := NewApp()

	model, _ := a.Update(tourStartMsg{})
	a = model.(AppModel)
	if !a.tourPending || a.tourCard != 0 {
		t.Fatalf("tour should start pending at card 0, pending=%v card=%d", a.tourPending, a.tourCard)
	}

	// The tour is passive: other keys pass through to the screen behind it
	// (navigation messages land on the next Update cycle).
	var navCmd tea.Cmd
	model, navCmd = a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	a = model.(AppModel)
	if !a.tourPending {
		t.Fatal("the tour should still be pending after a passthrough key")
	}
	if msg := navCmd(); msg != nil {
		model, _ = a.Update(msg)
		a = model.(AppModel)
	}
	if a.view != viewLibrary {
		t.Fatalf("'l' should open the library under the tour, view=%d", a.view)
	}
	model, navCmd = a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	a = model.(AppModel)
	if msg := navCmd(); msg != nil {
		model, _ = a.Update(msg)
		a = model.(AppModel)
	}
	if a.view != viewHome {
		t.Fatalf("browser 'h' should return home under the tour, view=%d", a.view)
	}

	// Walk the three cards.
	for want := 1; want <= 2; want++ {
		model, _ = a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
		a = model.(AppModel)
		if !a.tourPending || a.tourCard != want {
			t.Fatalf("card %d: pending=%v card=%d", want, a.tourPending, a.tourCard)
		}
	}
	model, _ = a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	a = model.(AppModel)
	if a.tourPending {
		t.Fatal("the last card should complete the tour")
	}
	cfg, _ := config.Load()
	if !cfg.TourSeen {
		t.Fatal("completion should persist TourSeen=true")
	}

	// A later tour message is ignored once seen.
	model, _ = a.Update(tourStartMsg{})
	a = model.(AppModel)
	if a.tourPending {
		t.Fatal("tour must not reappear after completion")
	}
}

func TestTourSkipPersistsSeen(t *testing.T) {
	testutil.RedirectConfigDir(t)
	a := NewApp()
	model, _ := a.Update(tourStartMsg{})
	a = model.(AppModel)
	if !a.tourPending {
		t.Fatal("tour should start pending")
	}
	model, _ = a.Update(tea.KeyMsg{Type: tea.KeyEsc})
	a = model.(AppModel)
	if a.tourPending {
		t.Fatal("Esc should skip the tour")
	}
	cfg, _ := config.Load()
	if !cfg.TourSeen {
		t.Fatal("skip should persist TourSeen=true")
	}
}

// TestHelpFilteredToActiveScreen guards 8.3: ? opens the reference filtered
// to the screen that was active.
func TestHelpFilteredToActiveScreen(t *testing.T) {
	a := NewApp()
	a.view = viewLibrary
	model, _ := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	a = model.(AppModel)
	if a.view != viewHelp {
		t.Fatalf("? should open help, view=%d", a.view)
	}
	view := a.help.View()
	if !strings.Contains(view, "Library browser") {
		t.Errorf("library help should show the library block:\n%s", view)
	}
	if strings.Contains(view, "Tab viewer") {
		t.Errorf("library help must not show the viewer block:\n%s", view)
	}

	// From the viewer, the viewer block shows instead.
	a.view = viewViewer
	model, _ = a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	a = model.(AppModel)
	if !strings.Contains(a.help.View(), "Tab viewer") {
		t.Error("viewer help should show the viewer block")
	}
}

// TestDiagProbeSurfacesMissingDeps guards 8.2: the async probe result
// reaches the home screen as a banner + footer marker.
func TestDiagProbeSurfacesMissingDeps(t *testing.T) {
	testutil.RedirectConfigDir(t)
	a := NewApp()
	model, _ := a.Update(msgs.TabsLoadedMsg{Tabs: nil})
	a = model.(AppModel)

	model, _ = a.Update(diagProbeMsg{missing: []string{"fluidsynth/timidity"}})
	a = model.(AppModel)
	if len(a.missingDeps) != 1 || a.missingDeps[0] != "fluidsynth/timidity" {
		t.Fatalf("missingDeps = %v", a.missingDeps)
	}
	view := a.View()
	if !strings.Contains(view, "missing: fluidsynth/timidity") {
		t.Errorf("home should show the missing-dep banner:\n%s", view)
	}
	if !strings.Contains(view, "fretboard doctor") {
		t.Errorf("banner should point at `fretboard doctor`:\n%s", view)
	}
}

func TestCriticalMissingGroups(t *testing.T) {
	ok := func(names ...string) []diag.CheckResult {
		var out []diag.CheckResult
		for _, n := range names {
			out = append(out, diag.CheckResult{Name: n, OK: true})
		}
		return out
	}
	if got := criticalMissing(ok("mpv", "ffplay", "fluidsynth", "timidity")); len(got) != 0 {
		t.Fatalf("all present should report nothing, got %v", got)
	}
	if got := criticalMissing(ok("mpv", "ffplay")); len(got) != 1 || got[0] != "fluidsynth/timidity" {
		t.Fatalf("missing both MIDI synths should report the group, got %v", got)
	}
	if got := criticalMissing(ok("mpv", "ffplay", "fluidsynth")); len(got) != 1 || got[0] != "timidity" {
		t.Fatalf("missing one synth should report it, got %v", got)
	}
	if got := criticalMissing(ok("fluidsynth", "timidity")); len(got) != 1 || got[0] != "mpv/ffplay" {
		t.Fatalf("missing both players should report the group, got %v", got)
	}
}

// TestEditPersistMsg guards 7: the edited content is written to its file,
// re-imported into the library row, and the row is stamped as edited.
func TestEditPersistMsg(t *testing.T) {
	st := newTestStore(t)
	path := filepath.Join(t.TempDir(), "song.txt")
	if err := os.WriteFile(path, []byte("Tuning: E Standard\n\ne|0-3-5|\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	id, err := st.ImportFile(path)
	if err != nil {
		t.Fatal(err)
	}

	a := NewAppWithOptions(st, nil, "", nil)
	edited := "Title: Edited Song\nArtist: New Artist\n\ne|0-3-5|3-|\n"
	model, _ := a.Update(msgs.EditPersistMsg{TabID: id, Path: path, Content: edited, Title: "Edited Song", Artist: "New Artist"})
	a = model.(AppModel)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != edited {
		t.Fatalf("file content = %q, want edited content", string(data))
	}
	row, err := st.GetRow(id)
	if err != nil {
		t.Fatal(err)
	}
	if row.Title != "Edited Song" || row.Artist != "New Artist" {
		t.Fatalf("row title/artist = %q/%q, want Edited Song/New Artist", row.Title, row.Artist)
	}
	if row.EditedAt == 0 {
		t.Fatal("edit should stamp edited_at")
	}
	if a.view != viewHome {
		t.Fatalf("view should stay home, view=%d", a.view)
	}
}

// TestPracticeSessionMsg guards 7: PracticeSessionMsg records the session
// via the library practice log.
func TestPracticeSessionMsg(t *testing.T) {
	st := newTestStore(t)
	id, err := st.Import("s.txt", testTab())
	if err != nil {
		t.Fatal(err)
	}
	a := NewAppWithOptions(st, nil, "", nil)
	model, _ := a.Update(msgs.PracticeSessionMsg{TabID: id, DurationSec: 125, TempoBPM: 96, Loops: 4})
	a = model.(AppModel)

	total, byTab, err := st.PracticeStats(7)
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 { // 125s → 2 whole minutes
		t.Fatalf("practice total = %d min, want 2", total)
	}
	if len(byTab) != 1 || byTab[0].TabID != id {
		t.Fatalf("practice rows = %+v, want one row for tab %d", byTab, id)
	}
}

// writeFakeGpParser installs a fake gp-parser that echoes the given --all
// envelope, so app-level GP import flows run without a real Guitar Pro
// file. The shim is a shell script on Unix and a .cmd batch file on Windows
// (CreateProcess cannot run extensionless shebang scripts; .cmd/.bat are
// executed via cmd.exe).
func writeFakeGpParser(t *testing.T, envelope string) {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "gp-parser")
	if runtime.GOOS == "windows" {
		bin += ".cmd"
		// JSON is whitespace-insensitive, so one echo line carries the
		// envelope and keeps cmd quoting out of the picture.
		script := "@echo off\r\n" + "echo " + strings.ReplaceAll(envelope, "\n", "") + "\r\n"
		if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("FRETBOARD_GP_PARSER", bin)
		return
	}
	script := "#!/bin/sh\ncat <<'EOF'\n" + envelope + "\nEOF\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FRETBOARD_GP_PARSER", bin)
}

// twoTrackEnvelope is a gp-parser --all payload with two tracks; the second
// track's bar carries fret 7 so imports are distinguishable.
const twoTrackEnvelope = `{"title":"Test Song","artist":"Test Artist","tracks":[
{"name":"Guitar 1","instrument":"Steel String Guitar","strings":6,"tuning":[64,59,55,50,45,40],"key":"C major","bars":[{"number":1,"column_ticks":[480],"strings":[{"segments":[{"char":"0","value":0,"position":0,"width":1}]},{"segments":[]},{"segments":[]},{"segments":[]},{"segments":[]},{"segments":[]}]}]},
{"name":"Bass","instrument":"Fingered Bass","strings":4,"tuning":[43,38,33,28],"key":"C major","bars":[{"number":1,"column_ticks":[480],"strings":[{"segments":[{"char":"7","value":7,"position":0,"width":1}]},{"segments":[]},{"segments":[]},{"segments":[]}]}]}
]}`

// TestGPFileImportPicker guards 5.3b: a multi-track GP file raises the
// picker, and picking a non-first track imports that track's content and
// stashes the full track list for the viewer's switcher.
func TestGPFileImportPicker(t *testing.T) {
	writeFakeGpParser(t, twoTrackEnvelope)
	st := newTestStore(t)
	a := NewAppWithOptions(st, nil, "", nil)
	gpPath := filepath.Join(t.TempDir(), "song.gp5")
	if err := os.WriteFile(gpPath, []byte("fake gp bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := a.handleFileAdded(watcher.FileAddedMsg{Path: gpPath})
	if cmd != nil {
		t.Fatal("a pending picker should not import yet")
	}
	if a.gpPicker == nil || len(a.gpPicker.tracks) != 2 {
		t.Fatalf("multi-track file should raise the picker, got %+v", a.gpPicker)
	}
	if !strings.Contains(a.View(), "Guitar 1") || !strings.Contains(a.View(), "Bass") {
		t.Error("picker should list every track")
	}

	// Move to the second track and import it.
	model, _ := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	a = model.(AppModel)
	if a.gpPicker.cursor != 1 {
		t.Fatalf("cursor = %d, want 1", a.gpPicker.cursor)
	}
	model, cmd = a.Update(tea.KeyMsg{Type: tea.KeyEnter})
	a = model.(AppModel)
	if a.gpPicker != nil {
		t.Fatal("picking should close the picker")
	}
	if cmd == nil {
		t.Fatal("picking should import")
	}

	rows, err := st.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 imported row, got %d", len(rows))
	}
	tab, err := st.Get(rows[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if tab.Title != "Test Song" {
		t.Fatalf("title = %q, want Test Song", tab.Title)
	}
	if len(tab.Bars) != 1 || len(tab.Bars[0].Strings) != 4 {
		t.Fatalf("picked track should be the 4-string bass, bars=%+v", tab.Bars)
	}
	fret := tab.Bars[0].Strings[0].Segments[0].Value
	if fret != 7 {
		t.Fatalf("picked track content should be bass fret 7, got %d", fret)
	}
	raw := tab.Metadata["tracks"]
	if raw == "" {
		t.Fatal("imported tab must carry metadata[\"tracks\"]")
	}
	var metas []map[string]any
	if err := json.Unmarshal([]byte(raw), &metas); err != nil {
		t.Fatalf("tracks metadata is not JSON: %v", err)
	}
	if len(metas) != 2 || metas[0]["name"] != "Guitar 1" || metas[1]["name"] != "Bass" {
		t.Fatalf("tracks metadata = %v", metas)
	}
	if metas[1]["strings"] != float64(4) {
		t.Fatalf("second track strings = %v, want 4", metas[1]["strings"])
	}
}

// TestGPFileSingleTrackImportsDirectly guards 5.3b: a single-track GP file
// imports as today — no picker — but still carries the tracks metadata.
func TestGPFileSingleTrackImportsDirectly(t *testing.T) {
	writeFakeGpParser(t, `{"title":"Solo","artist":"A","tracks":[
{"name":"Gtr","instrument":"Clean Guitar","strings":6,"tuning":[64,59,55,50,45,40],"key":"G major","bars":[]}]}`)
	st := newTestStore(t)
	a := NewAppWithOptions(st, nil, "", nil)
	gpPath := filepath.Join(t.TempDir(), "solo.gp5")
	if err := os.WriteFile(gpPath, []byte("fake gp bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := a.handleFileAdded(watcher.FileAddedMsg{Path: gpPath})
	if a.gpPicker != nil {
		t.Fatal("single-track file must not raise the picker")
	}
	if cmd == nil {
		t.Fatal("single-track import should proceed immediately")
	}
	rows, err := st.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	tab, err := st.Get(rows[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if tab.Metadata["tracks"] == "" {
		t.Fatal("single-track import should still carry tracks metadata")
	}
}

// TestGPPickerCancelImportsNothing guards the picker's Esc path.
func TestGPPickerCancelImportsNothing(t *testing.T) {
	writeFakeGpParser(t, twoTrackEnvelope)
	st := newTestStore(t)
	a := NewAppWithOptions(st, nil, "", nil)
	gpPath := filepath.Join(t.TempDir(), "song.gp5")
	if err := os.WriteFile(gpPath, []byte("fake gp bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.handleFileAdded(watcher.FileAddedMsg{Path: gpPath})
	if a.gpPicker == nil {
		t.Fatal("picker should be up")
	}
	model, _ := a.Update(tea.KeyMsg{Type: tea.KeyEsc})
	a = model.(AppModel)
	if a.gpPicker != nil {
		t.Fatal("Esc should cancel the picker")
	}
	rows, _ := st.List()
	if len(rows) != 0 {
		t.Fatalf("cancel must not import anything, got %d rows", len(rows))
	}
}

// TestDecodeGPTrackJSONPicksTrack guards the app-side re-decode used for
// non-first track picks.
func TestDecodeGPTrackJSONPicksTrack(t *testing.T) {
	tab, err := decodeGPTrackJSON([]byte(twoTrackEnvelope), 1)
	if err != nil {
		t.Fatal(err)
	}
	if tab.Title != "Test Song" || tab.Metadata["track"] != "Bass" {
		t.Fatalf("decoded tab = %+v", tab)
	}
	if len(tab.Tuning) != 4 {
		t.Fatalf("bass tuning = %v, want 4 strings", tab.Tuning)
	}
	if tab.Bars[0].Strings[0].Segments[0].Value != 7 {
		t.Fatalf("bass fret = %d, want 7", tab.Bars[0].Strings[0].Segments[0].Value)
	}
}
