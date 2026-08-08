package kit

import (
	"strings"
	"testing"

	"fretboard/internal/model"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestBarGridLayoutFitsWidth(t *testing.T) {	tab := &model.Tab{Tuning: model.ParseTuning("EADGBE")}
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
// their headers. Every row must span exactly bars × barWidth cells.
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
		// Assert per-row widths: each row spans barsInRow × barWidth cells
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
		model.Bar{Number: 1, Strings: []model.StringLine{{}}},                                  // 1 string
		model.Bar{Number: 2, Strings: []model.StringLine{{}}},                                  // 1 string
		model.Bar{Number: 3, Strings: make([]model.StringLine, 6)},                             // 6 strings
		model.Bar{Number: 4, Strings: make([]model.StringLine, 6)},                             // 6 strings
		model.Bar{Number: 5, Strings: []model.StringLine{{}, {}}},                              // 2 strings
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
	want := LoopBarStyle.Render(padToWidth("│ 2 "+strings.Repeat("─", 12), m.BarWidth))
	if !strings.Contains(rendered, want) {
		t.Fatalf("looped bar 2 header should use LoopBarStyle, got:\n%s", rendered)
	}
	if strings.Contains(rendered, LoopBarStyle.Render(padToWidth("│ 4 "+strings.Repeat("─", 12), m.BarWidth))) {
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
