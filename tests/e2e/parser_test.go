package e2e_test

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/YOUR_USERNAME/fretboard/internal/parser"
	"github.com/YOUR_USERNAME/fretboard/tests/helpers"
)

func fixturePath(name string) string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "fixtures", name)
}

func TestParseSimpleE2E(t *testing.T) {
	tab, err := parser.Parse(strings.NewReader(helpers.SultansTab))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if tab.Title == "" {
		t.Errorf("expected a title, got empty")
	}
	if tab.Artist == "" {
		t.Errorf("expected an artist, got empty")
	}
	if len(tab.Tuning) != 6 {
		t.Errorf("expected 6-string tuning, got %d (%v)", len(tab.Tuning), tab.Tuning)
	}
	if len(tab.Bars) < 2 {
		t.Fatalf("expected at least 2 bars, got %d", len(tab.Bars))
	}

	firstBar := tab.Bars[0]
	if len(firstBar.Strings) < 6 {
		t.Fatalf("first bar has %d strings, want 6", len(firstBar.Strings))
	}
	bString := firstBar.Strings[4]
	foundFret3 := false
	for _, seg := range bString.Segments {
		if seg.Value == 3 {
			foundFret3 = true
			break
		}
	}
	if !foundFret3 {
		t.Errorf("B string of first bar should contain a fret 3, got segments: %+v", bString.Segments)
	}
}

func TestParseDropD(t *testing.T) {
	tab, err := parser.Parse(strings.NewReader(helpers.DropDTab))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(tab.Tuning) != 6 {
		t.Errorf("Drop D tab should have 6 strings, got %d", len(tab.Tuning))
	}
	if tab.Tuning[0] != 38 {
		t.Errorf("Drop D lowest string should be MIDI 38 (D2), got %d", tab.Tuning[0])
	}
	if len(tab.Bars) < 1 {
		t.Fatalf("Drop D tab should have at least 1 bar, got %d", len(tab.Bars))
	}
}

func TestParseFreeform(t *testing.T) {
	tab, err := parser.Parse(strings.NewReader(helpers.FreeformTab))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(tab.Bars) < 1 {
		t.Errorf("freeform tab should still produce at least 1 bar, got %d", len(tab.Bars))
	}
	if len(tab.Bars) > 0 {
		eString := tab.Bars[0].Strings[5]
		found5 := false
		for _, seg := range eString.Segments {
			if seg.Value == 5 {
				found5 = true
				break
			}
		}
		if !found5 {
			t.Errorf("high e line should contain fret 5, got %+v", eString.Segments)
		}
	}
}

func TestParseMultiDigit(t *testing.T) {
	tab, err := parser.Parse(strings.NewReader(helpers.MultiDigitTab))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(tab.Bars) < 1 {
		t.Fatalf("multi-digit tab should have at least 1 bar")
	}
	found12 := false
	for _, sl := range tab.Bars[0].Strings {
		for _, seg := range sl.Segments {
			if seg.Value == 12 {
				found12 = true
				if seg.Width != 2 {
					t.Errorf("fret 12 segment should have width 2, got %d", seg.Width)
				}
			}
		}
	}
	if !found12 {
		t.Errorf("expected to find fret 12 in tab, didn't")
	}
}

func TestParseEmpty(t *testing.T) {
	tab, err := parser.Parse(strings.NewReader(""))
	if err != nil {
		t.Errorf("empty input should not error, got %v", err)
	}
	if tab == nil {
		t.Fatal("expected non-nil tab for empty input")
	}
}

func TestParseFile(t *testing.T) {
	tab, err := parser.ParseFile(fixturePath("sultans.txt"))
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}
	if tab == nil || len(tab.Bars) < 2 {
		t.Errorf("sultans.txt should parse to at least 2 bars, got %d", len(tab.Bars))
	}
}
