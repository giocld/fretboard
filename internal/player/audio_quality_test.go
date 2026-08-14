package player

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"fretboard/internal/model"
)

// TestClassifyLocalFilename guards the local-file strict classification: a
// "(Live)" file is CatLive (not strict-compatible), an official one is
// CatOfficial, and a plain name stays CatLocal.
func TestClassifyLocalFilename(t *testing.T) {
	cases := []struct {
		name string
		want AudioCategory
	}{
		{"Sultans of Swing.mp3", CatLocal},
		{"Sultans of Swing (Live 1984).mp3", CatLive},
		{"Sultans of Swing (Official Audio).mp3", CatOfficial},
		{"Sultans of Swing cover by John.mp3", CatCover},
		{"Sultans of Swing Karaoke.mp3", CatBacking},
		{"Sultans of Swing - guitar lesson.mp3", CatLesson},
		{"Sultans of Swing (Live Cover).mp3", CatCover}, // cover wins over live
	}
	for _, c := range cases {
		got := ClassifyLocalFilename(c.name)
		if got != c.want {
			t.Fatalf("ClassifyLocalFilename(%q) = %s, want %s", c.name, got, c.want)
		}
	}
	if StrictCompatible(ClassifyLocalFilename("x (Live).mp3")) {
		t.Fatal("a live local file must not be strict-compatible")
	}
	if !StrictCompatible(ClassifyLocalFilename("x (Official Audio).mp3")) {
		t.Fatal("an official local file must be strict-compatible")
	}
}

// TestBuildAudioCatalogClassifiesLocal guards the catalog: local sources
// carry their filename-derived category and strict flag.
func TestBuildAudioCatalogClassifiesLocal(t *testing.T) {
	dir := t.TempDir()
	live := filepath.Join(dir, "Sultans of Swing (Live).mp3")
	if err := os.WriteFile(live, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	tab := &model.Tab{Title: "Sultans of Swing", Artist: "Dire Straits"}
	cat, err := BuildAudioCatalog(tab, "", []string{dir}, false)
	if err != nil {
		t.Fatal(err)
	}
	var local *AudioSource
	for i := range cat.Sources {
		if cat.Sources[i].Kind == SourceLocal {
			local = &cat.Sources[i]
		}
	}
	if local == nil {
		t.Fatal("local source missing from catalog")
	}
	if local.Category != CatLive || local.StrictOK {
		t.Fatalf("live local file should be flagged, got %+v", local)
	}
}

// TestScoreYouTubeResultDurationProximity guards the duration-proximity
// term: a studio-length candidate outranks a live marathon even when the
// keywords miss both.
func TestScoreYouTubeResultDurationProximity(t *testing.T) {
	bar := model.Bar{Strings: []model.StringLine{{Segments: []model.Segment{
		{Char: '0', Value: 0, Position: 0, Width: 1},
		{Char: '-', Position: 1}, {Char: '-', Position: 2}, {Char: '-', Position: 3},
		{Char: '3', Value: 3, Position: 4, Width: 1},
	}}}}
	// 240 identical bars -> ~240 s of schedule at the tab BPM: a realistic
	// song length, so the duration-proximity term engages.
	bars := make([]model.Bar, 240)
	for i := range bars {
		bars[i] = bar
	}
	tab := &model.Tab{Title: "Sultans of Swing", Artist: "Dire Straits", Bars: bars}
	want := ScheduleDurationSeconds(tab, TabBPM(tab))
	if want < 120 {
		t.Fatalf("schedule duration should be a realistic song length, got %.0fs", want)
	}
	studio := int(want)       // exact length
	marathon := int(want * 2) // live jam
	clip := int(want / 3)     // preview clip
	s := ScoreYouTubeResult(tab, "Sultans of Swing", "Some Channel", "", studio)
	m := ScoreYouTubeResult(tab, "Sultans of Swing", "Some Channel", "", marathon)
	c := ScoreYouTubeResult(tab, "Sultans of Swing", "Some Channel", "", clip)
	if !(s > m && s > c) {
		t.Fatalf("studio-length candidate should win: studio=%d marathon=%d clip=%d", s, m, c)
	}
}

// TestAudioSearchFallbackQueries guards the second-pass query strategy.
func TestAudioSearchFallbackQueries(t *testing.T) {
	got := AudioSearchFallbackQueries(&model.Tab{Title: "Sultans of Swing", Artist: "Dire Straits"})
	want := []string{"Sultans of Swing official audio", "Sultans of Swing lyrics", "Sultans of Swing"}
	if len(got) != len(want) {
		t.Fatalf("fallback queries = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("fallback queries = %v, want %v", got, want)
		}
	}
}

// TestParseMPVStatus guards the position feedback parser.
func TestParseMPVStatus(t *testing.T) {
	pos, dur, ok := parseMPVStatus(`{"pos": 12.5, "dur": 253.2}`)
	if !ok || pos != 12500*time.Millisecond || dur != 253200*time.Millisecond {
		t.Fatalf("parse = %v %v %v", pos, dur, ok)
	}
	if _, _, ok := parseMPVStatus("no status here"); ok {
		t.Fatal("garbage must not parse")
	}
}

// TestBuildAudioCandidatesExcludesMpg123 guards the never-silently-wrong
// rule: mpg123 is only offered at rate 1 without a seek.
func TestBuildAudioCandidatesExcludesMpg123(t *testing.T) {
	hasMpg123 := func(cands []candidate) bool {
		for _, c := range cands {
			if c.bin == "mpg123" {
				return true
			}
		}
		return false
	}
	if !hasMpg123(buildAudioCandidates("x.mp3", 0, 1, 80)) {
		t.Fatal("plain playback should offer mpg123")
	}
	if hasMpg123(buildAudioCandidates("x.mp3", 5*time.Second, 1, 80)) {
		t.Fatal("seek must exclude mpg123")
	}
	if hasMpg123(buildAudioCandidates("x.mp3", 0, 1.5, 80)) {
		t.Fatal("rate change must exclude mpg123")
	}
}

// TestBuildAudioCandidatesMpvFirst guards the candidate priority: mpv is the
// first choice because its --term-status-msg position feedback drives sync
// (Elapsed() follows the player's true output position); ffplay is the
// fallback and mpg123 stays last.
func TestBuildAudioCandidatesMpvFirst(t *testing.T) {
	writeFakePlayers(t, "mpv", "ffplay", "mpg123")
	cands := buildAudioCandidates("x.mp3", 0, 1, 80)
	if len(cands) != 3 {
		t.Fatalf("plain playback should offer 3 candidates, got %d: %+v", len(cands), cands)
	}
	if cands[0].bin != "mpv" {
		t.Fatalf("mpv must be the first candidate, got %+v", cands)
	}
	if cands[1].bin != "ffplay" || cands[2].bin != "mpg123" {
		t.Fatalf("expected order [mpv ffplay mpg123], got %+v", cands)
	}
}

// writeFakePlayers writes executable shims for the given player names into a
// fresh temp dir prepended to PATH, mirroring the fakebin_* helpers (a .cmd
// loop on Windows, an executable shell script elsewhere). The shims are never
// spawned by buildAudioCandidates itself; they exist so lookPath would find
// every player when the ordering logic is exercised with all of them present.
func writeFakePlayers(t *testing.T, names ...string) {
	t.Helper()
	dir := t.TempDir()
	for _, name := range names {
		path := filepath.Join(dir, name)
		if runtime.GOOS == "windows" {
			path += ".cmd"
			if err := os.WriteFile(path, []byte("@echo off\r\n:loop\r\ngoto loop\r\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		} else {
			if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
				t.Fatal(err)
			}
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// TestEngineMPVPositionFeedback guards the timing refactor: with mpv
// reporting its true position, Elapsed() follows the feedback (not the wall
// clock), and the duration fallback fills in when ffprobe is absent.
func TestEngineMPVPositionFeedback(t *testing.T) {
	writeFakeMPV(t, `{"pos": 1.5, "dur": 100}`)
	dir := t.TempDir()
	audio := filepath.Join(dir, "song.mp3")
	if err := os.WriteFile(audio, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	e := NewEngine()
	e.Volume = 80
	if err := e.playAudioFile(audio); err != nil {
		t.Fatalf("playAudioFile: %v", err)
	}
	defer e.Stop()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if el := e.Elapsed(); el >= 1500*time.Millisecond && el < 2*time.Second {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	el := e.Elapsed()
	if el < 1500*time.Millisecond || el > 5*time.Second {
		t.Fatalf("Elapsed should follow mpv feedback (~1.5s), got %v", el)
	}
	// Duration fallback: no ffprobe on PATH, so audioDuration comes from the
	// status line.
	if d := e.AudioDuration(); d != 100*time.Second {
		t.Fatalf("duration fallback from mpv should be 100s, got %v", d)
	}
}
