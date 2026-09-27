package kit

import (
	"fmt"
	"strings"

	"fretboard/internal/model"
	"github.com/charmbracelet/lipgloss"
)

const (
	sheetDiagramFrets = 4  // fret rows drawn per diagram
	sheetDiagramCell  = 17 // columns per diagram: 11 of fretboard, a fret label, a gap
)

// RenderChordSheet renders a chord sheet: the song's distinct chords as
// diagrams, then the lyrics with chord names above them, the chord at
// cur.Bar highlighted. It returns the content and the content line the
// current chord sits on, for follow-scroll.
func RenderChordSheet(tab *model.Tab, cur *TabCursor, width int) (string, int) {
	if tab == nil {
		return "", 0
	}
	highlight := -1
	if cur != nil {
		highlight = cur.Bar
	}
	diagrams := sheetDiagramRows(tab, highlight, width)
	body, barLine := sheetBodyRows(tab, highlight)
	rows := make([]string, 0, len(diagrams)+1+len(body))
	rows = append(rows, diagrams...)
	if len(diagrams) > 0 {
		rows = append(rows, "")
	}
	rows = append(rows, body...)
	curLine := 0
	if line, ok := barLine[highlight]; ok {
		curLine = len(diagrams)
		if len(diagrams) > 0 {
			curLine++
		}
		curLine += line
	}
	return strings.Join(rows, "\n"), curLine
}

// ChordSheetBarLine returns the content line where bar's chord name is drawn,
// matching RenderChordSheet's layout. It is used for follow-scroll.
func ChordSheetBarLine(tab *model.Tab, width, bar int) int {
	if tab == nil {
		return 0
	}
	diagrams := len(sheetDiagramRows(tab, -1, width))
	_, barLine := sheetBodyRows(tab, -1)
	line, ok := barLine[bar]
	if !ok {
		return 0
	}
	if diagrams > 0 {
		diagrams++ // the blank line under the diagrams
	}
	return diagrams + line
}

// sheetBodyRows renders the lyrics and chord lines. barLine maps each bar to
// the row its chord name is drawn on.
func sheetBodyRows(tab *model.Tab, highlightBar int) (rows []string, barLine map[int]int) {
	barLine = map[int]int{}
	for _, sl := range tab.Sheet {
		if len(sl.Chords) > 0 {
			var row strings.Builder
			col := 0
			for _, c := range sl.Chords {
				pad := c.Col - col
				if pad < 1 && col > 0 {
					pad = 1 // never let two chord names run together
				}
				row.WriteString(strings.Repeat(" ", max(pad, 0)))
				style := LogoStyle
				if c.Bar == highlightBar {
					style = LogoStyle.Reverse(true)
				}
				barLine[c.Bar] = len(rows)
				row.WriteString(style.Render(c.Name))
				col += max(pad, 0) + lipgloss.Width(c.Name)
			}
			rows = append(rows, row.String())
			if sl.Text == "" {
				continue
			}
		}
		text := sl.Text
		if t := strings.TrimSpace(text); strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
			text = PanelTitleStyle.Render(text)
		} else {
			text = ListNormal.Render(text)
		}
		rows = append(rows, text)
	}
	return rows, barLine
}

// sheetDiagramRows lays the sheet's distinct chords out as diagrams, as many
// per row as fit the width. The highlighted chord's diagram is drawn in the
// playhead color.
func sheetDiagramRows(tab *model.Tab, highlightBar int, width int) []string {
	var names []string
	seen := map[string]bool{}
	barOf := map[string]int{}
	highlight := ""
	for _, sl := range tab.Sheet {
		for _, c := range sl.Chords {
			if !seen[c.Name] {
				seen[c.Name] = true
				names = append(names, c.Name)
				barOf[c.Name] = c.Bar
			}
			if c.Bar == highlightBar {
				highlight = c.Name
			}
		}
	}
	perRow := max(1, width/sheetDiagramCell)
	var rows []string
	for start := 0; start < len(names); start += perRow {
		end := min(start+perRow, len(names))
		var cells [][]string
		for _, name := range names[start:end] {
			cells = append(cells, ChordDiagram(name, barShape(tab, barOf[name]), name == highlight))
		}
		for l := range cells[0] {
			var row strings.Builder
			for _, cell := range cells {
				row.WriteString(padToWidth(cell[l], sheetDiagramCell))
			}
			rows = append(rows, strings.TrimRight(row.String(), " "))
		}
	}
	return rows
}

// barShape reads the chord shape a one-column bar carries: the fret of each
// string, -1 for a muted or unvoiced string.
func barShape(tab *model.Tab, bar int) [6]int {
	out := [6]int{-1, -1, -1, -1, -1, -1}
	if tab == nil || bar < 0 || bar >= len(tab.Bars) {
		return out
	}
	for s, sl := range tab.Bars[bar].Strings {
		if s >= len(out) {
			break
		}
		if len(sl.Segments) > 0 && sl.Segments[0].Char >= '0' && sl.Segments[0].Char <= '9' {
			out[s] = sl.Segments[0].Value
		}
	}
	return out
}

// sheetPlain renders a chord sheet's text: chord names over their lyrics,
// uncolored, for export and print.
func sheetPlain(tab *model.Tab) string {
	var b strings.Builder
	for _, sl := range tab.Sheet {
		if len(sl.Chords) > 0 {
			col := 0
			for _, c := range sl.Chords {
				pad := c.Col - col
				if pad < 1 && col > 0 {
					pad = 1
				}
				b.WriteString(strings.Repeat(" ", max(pad, 0)))
				b.WriteString(c.Name)
				col += max(pad, 0) + len(c.Name)
			}
			b.WriteString("\n")
			if sl.Text == "" {
				continue
			}
		}
		b.WriteString(sl.Text + "\n")
	}
	return b.String()
}

// ChordDiagram draws a chord's fingering, strings left (low E) to right:
//
//	G
//	    o o o
//	═══════════
//	│ │ │ │ │ │
//	│ ● │ │ │ │
//	● │ │ │ │ ●
//	│ │ │ │ │ │
//
// x marks a muted string, o an open one. A shape up the neck drops the nut
// and labels its first fret ("5fr"). A chord with no shape (all frets -1)
// shows only its name.
func ChordDiagram(name string, shape [6]int, current bool) []string {
	nameStyle, dot := PanelTitleStyle, FretDigitStyle
	if current {
		nameStyle, dot = PlayheadStyle.Reverse(true), PlayheadStyle
	}
	out := []string{nameStyle.Render(name)}
	voiced := false
	for _, f := range shape {
		if f >= 0 {
			voiced = true
			break
		}
	}
	if !voiced {
		return append(out, make([]string, sheetDiagramFrets+2)...)
	}
	base, top := 1, 0
	for _, f := range shape {
		top = max(top, f)
	}
	if top > sheetDiagramFrets {
		base = 99
		for _, f := range shape {
			if f > 0 {
				base = min(base, f)
			}
		}
	}
	marks := make([]string, len(shape))
	for s, f := range shape {
		switch {
		case f < 0:
			marks[s] = "x"
		case f == 0:
			marks[s] = "o"
		default:
			marks[s] = " "
		}
	}
	out = append(out, MutedStyle.Render(strings.TrimRight(strings.Join(marks, " "), " ")))
	edge := strings.Repeat("─", 2*len(shape)-1)
	if base == 1 {
		edge = strings.Repeat("═", 2*len(shape)-1)
	}
	out = append(out, StaffStyle.Render(edge))
	for fret := base; fret < base+sheetDiagramFrets; fret++ {
		cells := make([]string, len(shape))
		for s, f := range shape {
			cells[s] = StaffStyle.Render("│")
			if f == fret {
				cells[s] = dot.Render("●")
			}
		}
		row := strings.Join(cells, " ")
		if fret == base && base > 1 {
			row += MutedStyle.Render(fmt.Sprintf(" %dfr", base))
		}
		out = append(out, row)
	}
	return out
}
