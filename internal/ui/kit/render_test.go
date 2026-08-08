package kit

import (
	"strings"
	"testing"

	"fretboard/internal/model"
	"fretboard/internal/parser"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestBarGridLayoutFitsWidth(t *testing.T) {
	tab := &model.Tab{Tuning: model.ParseTuning("EADGBE")}
	for i := 0; i < 8; i++ {
		tab.Bars = append(tab.Bars, model.Bar{Number: i + 1, Strings: []model.StringLine{
			{Segments: []model.Segment{{Position: 0, Width: 24}}},
		}})
	}
	m := BarGridLayout(tab, 86)
	gridWidth := m.BarsPerRow * m.BarWidth
	if gridWidth > 86 {
		t.Fatalf("grid %d exceeds width 86", gridWidth)
	}
	if m.BarWidth < 24 {
		t.Fatalf("BarWidth %d clips 24-wide content", m.BarWidth)
	}
	if m.BarsPerRow < 2 {
		t.Fatalf("expected multiple bars per row, got %d", m.BarsPerRow)
	}
}

func TestBarGridLayoutWideBarPans(t *testing.T) {
	tab := &model.Tab{Tuning: model.ParseTuning("EADGBE")}
	tab.Bars = append(tab.Bars, model.Bar{Number: 1, Strings: []model.StringLine{
		{Segments: []model.Segment{{Position: 0, Width: 80}}},
	}})
	m := BarGridLayout(tab, 40)
	if m.BarsPerRow != 1 {
		t.Fatalf("wide bar should force one per row, got %d", m.BarsPerRow)
	}
	if m.BarWidth < 80 {
		t.Fatalf("BarWidth %d clips 80-wide content", m.BarWidth)
	}
}

// headerLine finds the rendered line holding the bar header "│ N ". In the
// grid layout several headers share one line, so a mid-line match counts.
func headerLine(t *testing.T, lines []string, number int) int {
	t.Helper()
	marker := "│ " + itoa(number) + " "
	for i, l := range lines {
		if strings.Contains(l, marker) {
			return i
		}
	}
	t.Fatalf("no header line for bar %d in %d rendered lines", number, len(lines))
	return -1
}

// TestGridContentRowsAlignWithHeaders guards the per-bar off-by-one: content
// rows were padded to a fixed barWidth-4 after a prefix whose label style is
// actually 5 cells wide, making string rows drift one cell per bar under
// their headers. Every row must span exactly bars x barWidth cells.
func TestGridContentRowsAlignWithHeaders(t *testing.T) {
	tab := &model.Tab{Tuning: model.ParseTuning("EADGBE")}
	for i := 1; i <= 6; i++ {
		tab.Bars = append(tab.Bars, model.Bar{Number: i, Strings: []model.StringLine{{
			Segments: []model.Segment{{Char: '0', Value: 0, Position: 1, Width: 1}},
		}}})
	}
	for _, w := range []int{60, 76, 120} {
		lines := strings.Split(RenderTabGrid(tab, w, 0, nil), "\n")
		m := BarGridLayout(tab, w)
		// Assert per-row widths: each row spans barsInRow x barWidth cells
		// (the last row may be partial).
		lineIdx := 0
		for rowStart := 0; rowStart < len(tab.Bars); rowStart += m.BarsPerRow {
			rowEnd := rowStart + m.BarsPerRow
			if rowEnd > len(tab.Bars) {
				rowEnd = len(tab.Bars)
			}
			rowWidth := (rowEnd - rowStart) * m.BarWidth
			rowLines := 1 + stringsPerRow(tab, rowStart, rowEnd) + 1
			for l := 0; l < rowLines-1; l++ {
				if got := lipgloss.Width(lines[lineIdx+l]); got != rowWidth {
					t.Fatalf("width %d: line %d is %d cells, want %d", w, lineIdx+l, got, rowWidth)
				}
			}
			lineIdx += rowLines
		}
	}
}

// TestGridBarLineOffsetsMatchRenderer verifies follow-scroll math against the
// actual rendered output, including rows with mixed string counts where the
// global max row height used to overshoot the playhead.
func TestGridBarLineOffsetsMatchRenderer(t *testing.T) {
	tab := &model.Tab{Title: "Mixed", Tuning: model.ParseTuning("EADGBE")}
	tab.Bars = append(tab.Bars,
		model.Bar{Number: 1, Strings: []model.StringLine{{}}},      // 1 string
		model.Bar{Number: 2, Strings: []model.StringLine{{}}},      // 1 string
		model.Bar{Number: 3, Strings: make([]model.StringLine, 6)}, // 6 strings
		model.Bar{Number: 4, Strings: make([]model.StringLine, 6)}, // 6 strings
		model.Bar{Number: 5, Strings: []model.StringLine{{}, {}}},  // 2 strings
	)
	for _, w := range []int{40, 76, 120} {
		offsets := GridBarLineOffsets(tab, w)
		lines := strings.Split(RenderTabGrid(tab, w, 0, nil), "\n")
		for b := range tab.Bars {
			want := headerLine(t, lines, b+1)
			if got := offsets[b]; got != want {
				t.Fatalf("width %d: bar %d offset = %d, rendered at %d", w, b+1, got, want)
			}
		}
	}
}

// TestLinearBarLineOffsetsMatchRenderer verifies the linear-layout follow
// math against the renderer; the old code used grid math in linear mode, so
// the playhead was never scrolled into view.
func TestLinearBarLineOffsetsMatchRenderer(t *testing.T) {
	tab := &model.Tab{Title: "Mixed", Tuning: model.ParseTuning("EADGBE")}
	tab.Bars = append(tab.Bars,
		model.Bar{Number: 1, Strings: make([]model.StringLine, 3)},
		model.Bar{Number: 2, Strings: make([]model.StringLine, 6)},
		model.Bar{Number: 3, Strings: make([]model.StringLine, 1)},
		model.Bar{Number: 4, Strings: make([]model.StringLine, 6)},
	)
	offsets := LinearBarLineOffsets(tab)
	lines := strings.Split(RenderTabLinear(tab, 0, nil), "\n")
	for b := range tab.Bars {
		want := headerLine(t, lines, b+1)
		if got := offsets[b]; got != want {
			t.Fatalf("bar %d offset = %d, rendered at %d", b+1, got, want)
		}
	}
	// Sanity: linear blocks stack under each other, so bar 4 must sit below
	// the viewport of a small terminal (the old grid math returned ~line 2).
	if offsets[3] <= 20 {
		t.Fatalf("bar 4 should start far down the linear layout, got %d", offsets[3])
	}
}

// TestGridLoopHeadersHighlighted guards the US-6 loop-region indicator: bar
// headers inside the A-B loop are rendered with LoopBarStyle (and the playhead
// bar keeps CursorStyle precedence).
func TestGridLoopHeadersHighlighted(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })
	tab := &model.Tab{Tuning: model.ParseTuning("EADGBE")}
	for i := 0; i < 4; i++ {
		tab.Bars = append(tab.Bars, model.Bar{Number: i + 1, Strings: []model.StringLine{{}}})
	}
	rendered := RenderTabGrid(tab, 120, 0, &TabCursor{LoopStartBar: 1, LoopEndBar: 3, Bar: 2})
	m := BarGridLayout(tab, 120)
	want := LoopBarStyle.Render("│ 2 " + strings.Repeat("─", m.BarWidth-4))
	if !strings.Contains(rendered, want) {
		t.Fatalf("looped bar 2 header should use LoopBarStyle, got:\n%s", rendered)
	}
	if strings.Contains(rendered, LoopBarStyle.Render("│ 4 "+strings.Repeat("─", m.BarWidth-4))) {
		t.Fatalf("bar 4 is outside the loop and must not use LoopBarStyle:\n%s", rendered)
	}
	loop := TabCursor{LoopStartBar: 0, LoopEndBar: 2}
	var none TabCursor
	if !loop.InLoop(1) || none.InLoop(1) {
		t.Fatal("InLoop boundary math wrong")
	}
}

// TestLinearRulerAlignsWithStringPlayhead guards US-8: the ruler line above
// the strings in the linear layout must place its ┊ at the same display column
// as the string rows' own playhead markers. The ruler used a 3-space prefix
// while string rows use a 5-column label prefix, so the ruler floated 2
// columns left of the notes.
func TestLinearRulerAlignsWithStringPlayhead(t *testing.T) {
	tab := &model.Tab{Title: "T", Tuning: model.ParseTuning("EADGBE")}
	tab.Bars = append(tab.Bars, model.Bar{Number: 1, Strings: []model.StringLine{
		{Segments: []model.Segment{{Char: '-', Position: 0, Width: 1}, {Char: '3', Value: 3, Position: 3, Width: 1}}},
		{Segments: []model.Segment{{Char: '-', Position: 0, Width: 1}}},
	}})
	lines := strings.Split(RenderTabLinear(tab, 0, &TabCursor{Bar: 0, Col: 3}), "\n")
	headerIdx := headerLine(t, lines, 1)
	rulerCol := displayColumnOf(lines[headerIdx+1], "┊")
	if rulerCol < 0 {
		t.Fatalf("ruler line has no playhead: %q", lines[headerIdx+1])
	}
	// Every string row of the highlighted bar must carry the playhead at the
	// same column, even the row whose content ends before the cursor column.
	for row := headerIdx + 2; row < headerIdx+2+len(tab.Bars[0].Strings); row++ {
		stringCol := displayColumnOf(lines[row], "┊")
		if stringCol < 0 {
			t.Fatalf("string row %d has no playhead: %q", row, lines[row])
		}
		if stringCol != rulerCol {
			t.Fatalf("ruler playhead at column %d, string row %d at %d — misaligned", rulerCol, row, stringCol)
		}
	}
}

// displayColumnOf returns the display column of the first occurrence of marker
// in s, or -1. Column math must use display width (marker may sit after styled
// multibyte runes).
func displayColumnOf(s, marker string) int {
	idx := strings.Index(s, marker)
	if idx < 0 {
		return -1
	}
	return lipgloss.Width(s[:idx])
}

// TestBarHeaderShowsRepeatMarkers guards S2.3: repeat structure is visible on
// the grid bar headers so the player sees the same form playback follows.
func TestBarHeaderShowsRepeatMarkers(t *testing.T) {
	tab := &model.Tab{
		Tuning: model.Standard,
		Bars: []model.Bar{
			{Number: 1, RepeatStart: true, Strings: []model.StringLine{{Segments: []model.Segment{{Char: '0', Value: 0, Position: 0, Width: 1}}}}},
			{Number: 2, Ending: 1, Strings: []model.StringLine{{Segments: []model.Segment{{Char: '3', Value: 3, Position: 0, Width: 1}}}}},
			{Number: 3, Ending: 2, RepeatEnd: true, Strings: []model.StringLine{{Segments: []model.Segment{{Char: '5', Value: 5, Position: 0, Width: 1}}}}},
		},
	}
	out := RenderTabGridBody(tab, 60, 0, nil)
	for _, want := range []string{"│:", ":│", "1.", "2."} {
		if !strings.Contains(out, want) {
			t.Fatalf("grid header missing repeat marker %q in:\n%s", want, out)
		}
	}
	out = RenderTabLinearBody(tab, 0, nil)
	for _, want := range []string{"│:", ":│", "1.", "2."} {
		if !strings.Contains(out, want) {
			t.Fatalf("linear header missing repeat marker %q in:\n%s", want, out)
		}
	}
}

// TestRenderTabPlain guards S4.3: the plain renderer emits uncolored ASCII
// with pipes, fret digits, tuning, and repeat markers that re-parse cleanly.
func TestRenderTabPlain(t *testing.T) {
	tab := &model.Tab{
		Title:  "Plain",
		Artist: "Artist",
		Tuning: model.Standard,
		Bars: []model.Bar{
			{Number: 1, RepeatStart: true, Strings: []model.StringLine{
				{Segments: []model.Segment{{Char: '0', Value: 0, Position: 0, Width: 1}, {Char: '-', Position: 1}, {Char: '-', Position: 2}, {Char: '3', Value: 3, Position: 3, Width: 1}}},
			}},
		},
	}
	out := RenderTabPlain(tab)
	for _, want := range []string{"Plain", "Artist", "Tuning: EADGBE", "|:0--3|"} {
		if !strings.Contains(out, want) {
			t.Fatalf("plain render missing %q:\n%s", want, out)
		}
	}
	// Re-parse: the export round-trips into the same bar content.
	parsed, err := parser.Parse(strings.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Bars) != 1 || !parsed.Bars[0].RepeatStart {
		t.Fatalf("round-trip lost structure: %+v", parsed.Bars)
	}
}

// TestRenderTabShowsNoteNames guards S5.3: with ShowNotes the fret digits
// render as note names while the grid width is preserved.
func TestRenderTabShowsNoteNames(t *testing.T) {
	tab := &model.Tab{
		Tuning: model.Standard,
		Bars: []model.Bar{{Strings: []model.StringLine{
			{Segments: []model.Segment{
				{Char: '0', Value: 0, Position: 0, Width: 1},
				{Char: '-', Position: 1},
				{Char: '3', Value: 3, Position: 2, Width: 1},
			}},
		}}},
	}
	plain := RenderTabGridBody(tab, 40, 0, nil)
	notes := RenderTabGridBody(tab, 40, 0, &TabCursor{Bar: -1, ShowNotes: true})
	if strings.Contains(notes, "3") && !strings.Contains(notes, "G") {
		t.Fatalf("note-name view should show the pitch name:\n%s", notes)
	}
	if !strings.Contains(notes, "G") {
		t.Fatalf("3rd fret low E should render as G:\n%s", notes)
	}
	_ = plain
}

// TestRenderTabSearchHighlight guards S5.1: the current match bar gets the
// search header style in the grid layout.
func TestRenderTabSearchHighlight(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })
	tab := &model.Tab{
		Tuning: model.Standard,
		Bars: []model.Bar{
			{Number: 1, Strings: []model.StringLine{{Segments: []model.Segment{{Char: '0', Value: 0, Position: 0, Width: 1}}}}},
			{Number: 2, Strings: []model.StringLine{{Segments: []model.Segment{{Char: '3', Value: 3, Position: 0, Width: 1}}}}},
		},
	}
	out := RenderTabGridBody(tab, 60, 0, &TabCursor{Bar: 0, SearchBar: 1, SearchCol: 0})
	m := BarGridLayout(tab, 60)
	want := SearchBarStyle.Render("│ 2 " + strings.Repeat("─", m.BarWidth-4))
	if !strings.Contains(out, want) {
		t.Fatalf("match bar header should use SearchBarStyle:\n%s", out)
	}
}

// TestBarHeaderShowsSectionName guards G2.2: the first bar of a section
// carries the section name in its header, in both layouts; later bars of
// the same section do not.
func TestBarHeaderShowsSectionName(t *testing.T) {
	tab := &model.Tab{
		Tuning: model.Standard,
		Bars: []model.Bar{
			{Number: 1, Section: "Intro", Strings: []model.StringLine{{Segments: []model.Segment{{Char: '0', Value: 0, Position: 0, Width: 1}}}}},
			{Number: 2, Section: "Intro", Strings: []model.StringLine{{Segments: []model.Segment{{Char: '3', Value: 3, Position: 0, Width: 1}}}}},
			{Number: 3, Section: "Chorus", Strings: []model.StringLine{{Segments: []model.Segment{{Char: '5', Value: 5, Position: 0, Width: 1}}}}},
		},
	}
	grid := RenderTabGridBody(tab, 60, 0, nil)
	if !strings.Contains(grid, "Intro") {
		t.Fatalf("grid header should show the section name:\n%s", grid)
	}
	linear := RenderTabLinearBody(tab, 0, nil)
	if !strings.Contains(linear, "Chorus") || !strings.Contains(linear, "Intro") {
		t.Fatalf("linear headers should show section names:\n%s", linear)
	}
	// Only the FIRST bar of a section carries the label.
	if got := sectionLabel(tab, 1); got != "" {
		t.Fatalf("second Intro bar must not repeat the label, got %q", got)
	}
	if got := sectionLabel(tab, 2); got != "Chorus" {
		t.Fatalf("section-start bar should be labeled, got %q", got)
	}
}
