//go:build !noytdlp

package player

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"fretboard/internal/model"
)

// songTab builds a tab whose written length is a realistic song duration:
// 120 bars of 16 columns (4/4) at the default 120 BPM → 240 s expected.
// The 16-column span matters: ExpectedDuration derives the meter from the
// bar's column width at the sixteenth-note-per-column rule.
func songTab(artist, title string) *model.Tab {
	bar := model.Bar{Strings: []model.StringLine{{Segments: []model.Segment{
		{Char: '0', Value: 0, Position: 0, Width: 1},
		{Char: '-', Position: 15},
	}}}}
	bars := make([]model.Bar, 120)
	for i := range bars {
		bars[i] = bar
	}
	return &model.Tab{Artist: artist, Title: title, Bars: bars}
}

// writeJSONYtDlp installs a fake yt-dlp that prints the given playlist JSON
// on stdout, so SearchOnlineCandidates runs its full search-and-rank path
// against fixed entries. Handles both Unix and Windows shim styles inline.
func writeJSONYtDlp(t *testing.T, playlistJSON string) {
	t.Helper()
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "entries.json")
	if err := os.WriteFile(jsonPath, []byte(playlistJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		path := filepath.Join(dir, "yt-dlp.cmd")
		script := "@echo off\r\ntype \"" + jsonPath + "\"\r\n"
		if err := os.WriteFile(path, []byte(script), 0o644); err != nil {
			t.Fatal(err)
		}
	} else {
		path := filepath.Join(dir, "yt-dlp")
		script := "#!/bin/sh\ncat " + strconv.Quote(jsonPath) + "\n"
		if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// TestPinVideoForRoundTrip guards the pin API: write, read, overwrite, and
// the error paths (nil tab, empty id). A pin lives in tab.Metadata under
// MetaKeyPinnedVideo and survives the tab's metadata map untouched.
func TestPinVideoForRoundTrip(t *testing.T) {
	tab := &model.Tab{Title: "Layla", Artist: "Eric Clapton"}
	if _, ok := PinnedVideoFor(tab); ok {
		t.Fatal("unpinned tab should report no pin")
	}
	if err := PinVideoFor(tab, "dQw4w9WgXcQ"); err != nil {
		t.Fatal(err)
	}
	id, ok := PinnedVideoFor(tab)
	if !ok || id != "dQw4w9WgXcQ" {
		t.Fatalf("PinnedVideoFor = %q, %v; want dQw4w9WgXcQ, true", id, ok)
	}
	if tab.Metadata[MetaKeyPinnedVideo] != "dQw4w9WgXcQ" {
		t.Fatalf("metadata key not written: %v", tab.Metadata)
	}
	// Re-pin replaces the earlier choice.
	if err := PinVideoFor(tab, "newid"); err != nil {
		t.Fatal(err)
	}
	if id, _ := PinnedVideoFor(tab); id != "newid" {
		t.Fatalf("re-pin did not replace: %q", id)
	}
	// Errors: nil tab, whitespace-only id.
	if err := PinVideoFor(nil, "x"); err == nil {
		t.Fatal("nil tab must error")
	}
	if err := PinVideoFor(tab, "  "); err == nil {
		t.Fatal("empty id must error")
	}
	// A nil metadata map is created on demand.
	raw := &model.Tab{Title: "T"}
	if err := PinVideoFor(raw, "abc"); err != nil {
		t.Fatal(err)
	}
	if id, ok := PinnedVideoFor(raw); !ok || id != "abc" {
		t.Fatalf("pin on nil-metadata tab = %q, %v", id, ok)
	}
}

// TestSearchOnlineCandidatesPinBeatsGuessing guards the core pin contract:
// a pinned video leads the ranking unconditionally, even when a search
// heuristic would pick someone else (here: a nightcore rework that every
// keyword scorer despises is pinned, and it still wins).
func TestSearchOnlineCandidatesPinBeatsGuessing(t *testing.T) {
	writeJSONYtDlp(t, `{"entries":[
		{"id":"clean123","title":"Dire Straits - Sultans of Swing (Official Audio)","channel":"DireStraitsVEVO","description":"","duration":240},
		{"id":"night456","title":"Sultans of Swing NIGHTCORE SPED UP","channel":"Random Channel","description":"","duration":180}
	]}`)
	tab := songTab("Dire Straits", "Sultans of Swing")
	if err := PinVideoFor(tab, "night456"); err != nil {
		t.Fatal(err)
	}
	cands, err := SearchOnlineCandidates(tab, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) == 0 {
		t.Fatal("expected candidates")
	}
	if cands[0].VideoID != "night456" {
		t.Fatalf("pinned candidate must rank first, got %s (%q, score %d)", cands[0].VideoID, cands[0].Label, cands[0].Score)
	}
	if cands[0].PickReason != "pinned source" {
		t.Fatalf("pinned PickReason = %q, want %q", cands[0].PickReason, "pinned source")
	}
}

// TestSearchOnlineCandidatesPinSynthesizesMissingVideo guards the "wins
// forever" half of the pin contract: when the search engine no longer
// surfaces the pinned video, a minimal source is synthesized from the pin so
// the user's choice still leads the list.
func TestSearchOnlineCandidatesPinSynthesizesMissingVideo(t *testing.T) {
	writeJSONYtDlp(t, `{"entries":[
		{"id":"clean123","title":"Dire Straits - Sultans of Swing (Official Audio)","channel":"DireStraitsVEVO","description":"","duration":240}
	]}`)
	tab := songTab("Dire Straits", "Sultans of Swing")
	if err := PinVideoFor(tab, "goneFromSearch"); err != nil {
		t.Fatal(err)
	}
	cands, err := SearchOnlineCandidates(tab, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) == 0 || cands[0].VideoID != "goneFromSearch" {
		t.Fatalf("pinned video missing from results must still lead: %+v", cands)
	}
	if cands[0].PickReason != "pinned source" {
		t.Fatalf("PickReason = %q, want %q", cands[0].PickReason, "pinned source")
	}
}

// TestBestOnlineSourcePinnedSkipsSearch guards the return-it-directly shape:
// BestOnlineSource answers from the pin alone and must not require the
// search engine (fake yt-dlp exits 1 here).
func TestBestOnlineSourcePinnedSkipsSearch(t *testing.T) {
	writeFailingYtDlp(t)
	tab := songTab("Dire Straits", "Sultans of Swing")
	if err := PinVideoFor(tab, "pinnedID123"); err != nil {
		t.Fatal(err)
	}
	src, err := BestOnlineSource(tab)
	if err != nil {
		t.Fatalf("BestOnlineSource with a pin must not need the search engine: %v", err)
	}
	if src.VideoID != "pinnedID123" || src.PickReason != "pinned source" {
		t.Fatalf("pinned source = %+v", src)
	}
}

// TestExpectedDurationMath guards the bars × beats-per-bar × 60/BPM formula
// and its defaults: metadata BPM overrides the 120 default, a 3/4 bar yields
// 3 beats, GP-imported per-column ticks are authoritative, and a tab with no
// bars has no estimate.
func TestExpectedDurationMath(t *testing.T) {
	// 120 bars × 4/4 at the default 120 BPM → 240 s.
	tab := songTab("Dire Straits", "Sultans of Swing")
	if got := ExpectedDuration(tab); got != 240*time.Second {
		t.Fatalf("ExpectedDuration = %v, want 4m0s", got)
	}
	// Tempo metadata overrides the default.
	tab.Metadata = map[string]string{model.MetaKeyBPM: "240"}
	if got := ExpectedDuration(tab); got != 120*time.Second {
		t.Fatalf("ExpectedDuration at 240 BPM = %v, want 2m0s", got)
	}
	// No bars → no estimate.
	if got := ExpectedDuration(&model.Tab{Artist: "A", Title: "T"}); got != 0 {
		t.Fatalf("ExpectedDuration with no bars = %v, want 0", got)
	}
	// 3/4 meter: 12 columns → 3 beats per bar → 2 bars × 3 × 0.5 s.
	waltz := model.Bar{Strings: []model.StringLine{{Segments: []model.Segment{
		{Char: '0', Value: 0, Position: 0, Width: 1},
		{Char: '-', Position: 11},
	}}}}
	tab3 := &model.Tab{Artist: "A", Title: "Waltz", Bars: []model.Bar{waltz, waltz}}
	if got := ExpectedDuration(tab3); got != 3*time.Second {
		t.Fatalf("3/4 ExpectedDuration = %v, want 3s", got)
	}
	// GP-imported per-column ticks are authoritative: 4 whole-beat columns.
	gp := &model.Tab{Artist: "A", Title: "T", Bars: []model.Bar{{ColumnTicks: []int{480, 480, 480, 480}}}}
	if got := ExpectedDuration(gp); got != 2*time.Second {
		t.Fatalf("GP-ticks ExpectedDuration = %v, want 2s", got)
	}
}
