package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fretboard/internal/parser"
)

// riffTab is a classifiable six-string tab. The one-line "e|0-3-5|" sample
// used elsewhere falls below the parser's tab-evidence bar (30% of non-empty
// lines) once Title/Artist/Tuning count against it, and would be read as a
// chord sheet.
const riffTab = `Title: Riff
Artist: Band
Tuning: E Standard

e|-----------|-----------|
B|-----------|-----------|
G|-----------|-----------|
D|-------5---|-------5---|
A|--7--------|--7--------|
E|-0-----6-5-|-0-----6-5-|
`

func writeRiffTab(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(riffTab), 0o644); err != nil {
		t.Fatal(err)
	}
}

// showField returns the value of one "key   value" line from a show table,
// ignoring the tabwriter padding.
func showField(t *testing.T, out, key string) string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, key) {
			return strings.TrimSpace(strings.TrimPrefix(line, key))
		}
	}
	t.Fatalf("show output has no %q field:\n%s", key, out)
	return ""
}

func TestRunListEmptyLibrary(t *testing.T) {
	withConfigDir(t, func(dir string) {
		code, stdout, stderr := run("list")
		if code != 0 || !strings.Contains(stdout, "library is empty") {
			t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
		}
	})
}

// TestRunListShowsRowsWithTuningLabel imports a tab and checks list prints
// its id, title, artist and a human tuning label -- not the raw stored JSON.
func TestRunListShowsRowsWithTuningLabel(t *testing.T) {
	withConfigDir(t, func(dir string) {
		src := filepath.Join(dir, "riff.txt")
		writeRiffTab(t, src)
		if code, _, stderr := run("import", src); code != 0 {
			t.Fatalf("import: code=%d stderr=%q", code, stderr)
		}
		code, stdout, stderr := run("list")
		if code != 0 {
			t.Fatalf("list: code=%d stderr=%q", code, stderr)
		}
		for _, want := range []string{"TITLE", "ARTIST", "TUNING", "Riff", "Band", "EADGBE"} {
			if !strings.Contains(stdout, want) {
				t.Fatalf("list output missing %q:\n%s", want, stdout)
			}
		}
		if strings.Contains(stdout, "[40") {
			t.Fatalf("list leaked the stored tuning JSON:\n%s", stdout)
		}
	})
}

// TestRunShowTab summarises a tab file: metadata, bars, tempo, spacing and
// the playback order/length the player will use.
func TestRunShowTab(t *testing.T) {
	withConfigDir(t, func(dir string) {
		src := filepath.Join(dir, "riff.txt")
		writeRiffTab(t, src)
		code, stdout, stderr := run("show", src)
		if code != 0 {
			t.Fatalf("show: code=%d stderr=%q", code, stderr)
		}
		want := map[string]string{
			"source":         src,
			"title":          "Riff",
			"artist":         "Band",
			"tuning":         "EADGBE",
			"bars":           "2",
			"tempo":          "120 BPM",
			"playback order": "2 bar-visits (from 2 bars on the page)",
			"length":         "0:04",
		}
		for field, value := range want {
			if got := showField(t, stdout, field); got != value {
				t.Errorf("show %s = %q, want %q", field, got, value)
			}
		}
		if !strings.Contains(stdout, "cols/note") {
			t.Errorf("show is missing the spacing grid:\n%s", stdout)
		}
	})
}

// TestRunShowByLibraryID: a bare integer resolves through the library and
// reports the stored file path, so `list` ids feed straight into `show`.
func TestRunShowByLibraryID(t *testing.T) {
	withConfigDir(t, func(dir string) {
		src := filepath.Join(dir, "riff.txt")
		writeRiffTab(t, src)
		if code, _, stderr := run("import", src); code != 0 {
			t.Fatalf("import: code=%d stderr=%q", code, stderr)
		}
		code, stdout, stderr := run("show", "1")
		if code != 0 {
			t.Fatalf("show 1: code=%d stderr=%q", code, stderr)
		}
		if got := showField(t, stdout, "source"); got != src {
			t.Fatalf("show 1 source = %q, want %q", got, src)
		}
	})
}

// TestRunShowChordSheet: a stored chord sheet summarizes as a playable
// sheet (chords, one 4/4 bar each) and timing lists its bars.
func TestRunShowChordSheet(t *testing.T) {
	withConfigDir(t, func(dir string) {
		src := filepath.Join("..", "..", "tests", "fixtures", "chords", "amazing_grace.txt")
		if code, _, stderr := run("import", src); code != 0 {
			t.Fatalf("import: code=%d stderr=%q", code, stderr)
		}
		code, stdout, stderr := run("show", "1")
		if code != 0 {
			t.Fatalf("show: code=%d stderr=%q", code, stderr)
		}
		if got := showField(t, stdout, "title"); got != "Amazing Grace" {
			t.Errorf("show title = %q, want Amazing Grace", got)
		}
		if got := showField(t, stdout, "kind"); got != "chord sheet" {
			t.Errorf("show kind = %q, want chord sheet", got)
		}
		if got := showField(t, stdout, "chords"); got != "13 (one 4/4 bar each)" {
			t.Errorf("show chords = %q, want 13 bars", got)
		}
		if got := showField(t, stdout, "bars"); got != "13" {
			t.Errorf("show bars = %q, want 13", got)
		}
		code, stdout, stderr = run("timing", "1")
		if code != 0 {
			t.Fatalf("timing on a chord sheet: code=%d stderr=%q", code, stderr)
		}
		if got := timingBars(stdout); len(got) != 13 {
			t.Fatalf("timing listed %d bars, want one per chord: %v", len(got), got)
		}
		if !strings.Contains(stdout, "1.00m") {
			t.Fatalf("each chord must be one 4/4 measure:\n%s", stdout)
		}
	})
}

// timingBars returns the bar numbers in a timing table, in order.
func timingBars(out string) []string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	var bars []string
	for _, line := range lines[1:] { // skip the header
		if f := strings.Fields(line); len(f) > 0 {
			bars = append(bars, f[0])
		}
	}
	return bars
}

// TestRunTiming pins the per-bar breakdown: one row per bar, each note as a
// beat.sixteenth onset, with the optional range limiting rows.
func TestRunTiming(t *testing.T) {
	withConfigDir(t, func(dir string) {
		src := filepath.Join(dir, "riff.txt")
		writeRiffTab(t, src)

		code, stdout, stderr := run("timing", src)
		if code != 0 {
			t.Fatalf("timing: code=%d stderr=%q", code, stderr)
		}
		for _, want := range []string{"BAR", "MEASURES", "ONSETS", "1.00m", "1.1 1.3 3.2 4.1"} {
			if !strings.Contains(stdout, want) {
				t.Fatalf("timing output missing %q:\n%s", want, stdout)
			}
		}
		if got := timingBars(stdout); len(got) != 2 || got[0] != "1" || got[1] != "2" {
			t.Fatalf("timing bars = %v, want [1 2]", got)
		}

		code, stdout, stderr = run("timing", src, "2-2")
		if code != 0 {
			t.Fatalf("timing 2-2: code=%d stderr=%q", code, stderr)
		}
		if got := timingBars(stdout); len(got) != 1 || got[0] != "2" {
			t.Fatalf("timing 2-2 bars = %v, want [2]", got)
		}

		if code, _, stderr := run("timing"); code != 1 || !strings.Contains(stderr, "usage: fretboard timing") {
			t.Fatalf("timing with no args: code=%d stderr=%q", code, stderr)
		}
		for _, bad := range []string{"0", "x", "2-1"} {
			if code, _, stderr := run("timing", src, bad); code != 1 || !strings.Contains(stderr, "invalid bar range") {
				t.Fatalf("timing %q: code=%d stderr=%q", bad, code, stderr)
			}
		}
	})
}

// TestRunShowErrors: inspecting nothing, or something with neither bars nor
// chords, fails instead of printing a blank summary.
func TestRunShowErrors(t *testing.T) {
	withConfigDir(t, func(dir string) {
		if code, _, stderr := run("show"); code != 1 || !strings.Contains(stderr, "usage: fretboard show") {
			t.Fatalf("show with no args: code=%d stderr=%q", code, stderr)
		}
		empty := filepath.Join(dir, "empty.txt")
		if err := os.WriteFile(empty, []byte("just some prose, no chords here\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		// A prose file classifies as a chord sheet, so use a truly empty one.
		if err := os.WriteFile(empty, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		code, stdout, stderr := run("show", empty)
		if code != 1 || !strings.Contains(stderr, "no tab bars or chord lines found") {
			t.Fatalf("show on an empty file: code=%d stdout=%q stderr=%q", code, stdout, stderr)
		}
	})
}

// TestRunChords prints diagrams and skips bad names, failing only the exit
// code when nothing was printed for a name.
func TestRunChords(t *testing.T) {
	withConfigDir(t, func(dir string) {
		if code, _, stderr := run("chords"); code != 1 || !strings.Contains(stderr, "usage: fretboard chords") {
			t.Fatalf("chords with no args: code=%d stderr=%q", code, stderr)
		}

		code, stdout, stderr := run("chords", "Am7", "C")
		if code != 0 {
			t.Fatalf("chords: code=%d stderr=%q", code, stderr)
		}
		for _, want := range []string{"Am7", "C", "#", "x"} {
			if !strings.Contains(stdout, want) {
				t.Fatalf("chords output missing %q:\n%s", want, stdout)
			}
		}
		// C is an A-shape barre (x35553): the diagram window must move up
		// to fret 3, or the fifth-fret notes vanish.
		if !strings.Contains(stdout, " 3|") || !strings.Contains(stdout, "|- - # # # -|") {
			t.Fatalf("C diagram does not show its third-fret window:\n%s", stdout)
		}

		code, stdout, stderr = run("chords", "Am7", "notachord")
		if code != 1 || !strings.Contains(stderr, "notachord: not a recognized chord name") {
			t.Fatalf("chords with a bad name: code=%d stderr=%q", code, stderr)
		}
		if !strings.Contains(stdout, "Am7") {
			t.Fatalf("a bad name must not suppress the good diagram:\n%s", stdout)
		}
	})
}

// TestChordDiagramShowsEveryFrettedNote guards the diagram window: every
// fretted position in a shape must appear as '#' regardless of how high the
// barre sits.
func TestChordDiagramShowsEveryFrettedNote(t *testing.T) {
	for _, name := range []string{"C", "G", "F", "Am7", "Fsus2", "Bm7", "D#"} {
		c, ok := parser.ParseChord(name)
		if !ok {
			t.Fatalf("%s: not parseable", name)
		}
		shape := c.FretShape()
		want := 0
		for _, f := range shape {
			if f > 0 {
				want++
			}
		}
		// Count only the note grid: a name like "D#" has a '#' of its own.
		grid := chordDiagram(name, shape)
		grid = grid[strings.Index(grid, "\n")+1:]
		if got := strings.Count(grid, "#"); got != want {
			t.Errorf("%s diagram shows %d fretted notes, shape has %d:\n%s", name, got, want, chordDiagram(name, shape))
		}
	}
}

func TestParseBarRange(t *testing.T) {
	if from, to, err := parseBarRange("2", 5); err != nil || from != 2 || to != 2 {
		t.Fatalf("2: from=%d to=%d err=%v", from, to, err)
	}
	if from, to, err := parseBarRange("2-4", 5); err != nil || from != 2 || to != 4 {
		t.Fatalf("2-4: from=%d to=%d err=%v", from, to, err)
	}
	if _, to, err := parseBarRange("4-9", 5); err != nil || to != 5 {
		t.Fatalf("4-9 should clamp to the last bar: to=%d err=%v", to, err)
	}
	for _, bad := range []string{"0", "x", "3-1", "-2", ""} {
		if _, _, err := parseBarRange(bad, 5); err == nil {
			t.Fatalf("%q should be rejected", bad)
		}
	}
}
