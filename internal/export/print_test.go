package export

import (
	"strings"
	"testing"
	"unicode/utf8"

	"fretboard/internal/model"
)

// lineFrom builds a StringLine whose segments mirror a compact ASCII tab
// row like "--0--3--": every character a 1-column segment, digits carrying
// their fret value.
func lineFrom(pattern string) model.StringLine {
	segs := make([]model.Segment, 0, len(pattern))
	for i, r := range pattern {
		seg := model.Segment{Char: r, Position: i, Width: 1}
		if r >= '0' && r <= '9' {
			seg.Value = int(r - '0')
		}
		segs = append(segs, seg)
	}
	return model.StringLine{Segments: segs}
}

// sampleTab builds a 6-string tab whose bars are "--0--3--" rows, with the
// metadata the page header shows.
func sampleTab(bars int) *model.Tab {
	tab := &model.Tab{
		Title:  "Wonderwall",
		Artist: "Oasis",
		Tuning: model.Standard,
		Metadata: map[string]string{
			model.MetaKeyBPM:  "96",
			model.MetaKeyCapo: "2",
		},
	}
	for i := range bars {
		bar := model.Bar{Number: i + 1}
		for range 6 {
			bar.Strings = append(bar.Strings, lineFrom("--0--3--"))
		}
		tab.Bars = append(tab.Bars, bar)
	}
	return tab
}

func TestPrintTabPagination(t *testing.T) {
	// 40 bars × 5 per row = 8 rows of 8 lines; the page header is 3 lines,
	// so the first page holds 6 rows (51 lines) and page 2 the rest.
	out := PrintTab(sampleTab(40), 100)

	if n := strings.Count(out, "\f"); n != 1 {
		t.Fatalf("expected 1 form feed for 40 bars, got %d", n)
	}
	pages := strings.Split(out, "\f")
	if len(pages) != 2 {
		t.Fatalf("expected 2 pages, got %d", len(pages))
	}
	for i, page := range pages {
		if !strings.Contains(page, "Wonderwall — Oasis") {
			t.Errorf("page %d does not repeat the header", i)
		}
		if n := strings.Count(page, "\n"); n > printPageLines {
			t.Errorf("page %d has %d lines, want <= %d", i, n, printPageLines)
		}
	}
	if strings.Count(out, "Wonderwall — Oasis") != 2 {
		t.Errorf("header should appear exactly once per page")
	}
}

func TestPrintTabNoLineExceedsWidth(t *testing.T) {
	cases := []struct {
		width int
		want  int
	}{
		{40, 40},
		{100, 100},
		{200, 100}, // capped at printMaxWidth
		{0, 80},    // default width
		{10, 22},   // raised to the printable minimum
	}
	for _, c := range cases {
		out := PrintTab(sampleTab(12), c.width)
		for _, line := range strings.Split(out, "\n") {
			if n := utf8.RuneCountInString(line); n > c.want {
				t.Fatalf("width %d: line %q is %d columns, want <= %d", c.width, line, n, c.want)
			}
		}
	}
}

func TestPrintTabRendersDigitsAndTechnique(t *testing.T) {
	bar := model.Bar{
		Number: 2,
		Strings: []model.StringLine{
			{Segments: []model.Segment{
				{Char: '-', Position: 0, Width: 1},
				{Char: '-', Position: 1, Width: 1},
				{Char: '1', Value: 12, Position: 2, Width: 2}, // two-digit fret
				{Char: '-', Position: 4, Width: 1},
				{Char: 'h', Position: 5, Width: 1},
				{Char: '5', Value: 5, Position: 6, Width: 1},
				{Char: '-', Position: 7, Width: 1},
			}},
			{Segments: []model.Segment{
				{Char: '-', Position: 0, Width: 1},
				{Char: '0', Value: 0, Position: 1, Width: 1},
				{Char: '-', Position: 2, Width: 1},
			}},
		},
	}
	tab := &model.Tab{Title: "T", Tuning: model.Standard, Bars: []model.Bar{bar}}
	out := PrintTab(tab, 60)
	for _, want := range []string{"--12-h5-", "|E|", "|A|", "| 2 "} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestPrintTabRoundTripsMultiBarTab(t *testing.T) {
	tab := &model.Tab{
		Title:  "Round Trip",
		Artist: "A",
		Tuning: model.Standard,
		Bars: []model.Bar{
			{Number: 1, Section: "Intro", Strings: []model.StringLine{
				lineFrom("--0--3--"), lineFrom("----5---"), lineFrom("--7--7--"),
				lineFrom("--0--3--"), lineFrom("----5---"), lineFrom("--7--7--"),
			}},
			{Number: 2, Strings: []model.StringLine{
				lineFrom("--3h5---"), lineFrom("--7p5---"), lineFrom("--9--9--"),
				lineFrom("--3h5---"), lineFrom("--7p5---"), lineFrom("--9--9--"),
			}},
			{Number: 3, Strings: []model.StringLine{
				lineFrom("--12-3--"), lineFrom("--10-1--"), lineFrom("--12-3--"),
				lineFrom("--12-3--"), lineFrom("--10-1--"), lineFrom("--12-3--"),
			}},
		},
	}
	out := PrintTab(tab, 80)
	for _, want := range []string{"--0--3--", "--3h5---", "--7p5---", "--12-3--", "--10-1--", "Round Trip — A"} {
		if !strings.Contains(out, want) {
			t.Errorf("round-trip: output lost %q", want)
		}
	}
	if strings.Count(out, "| 1 Intro") != 1 {
		t.Errorf("section header missing or repeated:\n%s", out)
	}
}

func TestPrintTabRepeatMarkers(t *testing.T) {
	tab := &model.Tab{Title: "R", Tuning: model.Standard, Bars: []model.Bar{
		{Number: 1, RepeatStart: true, Strings: []model.StringLine{lineFrom("--0--3--")}},
		{Number: 2, RepeatEnd: true, Ending: 1, Strings: []model.StringLine{lineFrom("--0--3--")}},
	}}
	out := PrintTab(tab, 60)
	for _, want := range []string{"|: 1 ", "| 2 1.", ":|"} {
		if !strings.Contains(out, want) {
			t.Errorf("repeat marker %q missing:\n%s", want, out)
		}
	}
}

func TestPrintTabNilOrEmpty(t *testing.T) {
	if out := PrintTab(nil, 80); out != "" {
		t.Fatalf("nil tab should print empty, got %q", out)
	}
	if out := PrintTab(&model.Tab{Title: "empty"}, 80); out != "" {
		t.Fatalf("tab with no bars should print empty, got %q", out)
	}
}
