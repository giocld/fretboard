package export

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"fretboard/internal/model"
)

// Print constants. A printed page holds ~55 lines total (page header
// included) so each sheet reads like a screenful. Pages are separated by a
// line carrying a single form feed (\f); lpr and PDF converters honor it as
// a page eject, so a song prints on clean sheet boundaries.
const (
	printPageLines  = 55
	printMaxWidth   = 100
	printMinWidth   = 22 // below this not even a 6-bar row fits; clamp up
	printHeaderRows = 3  // two header lines + one blank
	minBarWidth     = 18 // mirror internal/ui/kit grid packing
	maxBarsPerRow   = 6
)

// PrintTab renders a tab as paginated fixed-width plain text for printing
// (lpr) or PDF conversion. The page width is min(width, 100) columns — a
// width <= 0 means 80 — and every emitted line fits that width. Each page
// repeats the header "Title — Artist" and "Tuning · Tempo · Capo" lines,
// and pages are separated by a form feed (\f). Bars are packed side by side
// in rows exactly like the on-screen grid layout (18-column minimum per
// bar, at most 6 bars per row), reimplemented here in plain ASCII because
// the viewer's grid renderer emits ANSI styling.
func PrintTab(tab *model.Tab, width int) string {
	if tab == nil || len(tab.Bars) == 0 {
		return ""
	}
	pageWidth := clampWidth(width)
	layout := printLayoutFor(tab, pageWidth)
	header := tabHeaderLines(tab)
	head1 := truncateRunes(header[0], pageWidth)
	head2 := truncateRunes(header[1], pageWidth)

	var b strings.Builder
	pageLines := 0
	writeHeader := func() {
		b.WriteString(head1)
		b.WriteString("\n")
		b.WriteString(head2)
		b.WriteString("\n\n")
		pageLines = printHeaderRows
	}
	writeHeader()

	for rowStart := 0; rowStart < len(tab.Bars); rowStart += layout.barsPerRow {
		rowEnd := min(rowStart+layout.barsPerRow, len(tab.Bars))
		rowStrings := 0
		for bar := rowStart; bar < rowEnd; bar++ {
			rowStrings = max(rowStrings, len(tab.Bars[bar].Strings))
		}
		rowLines := 1 + rowStrings + 1 // bar-header row + string rows + blank separator
		if pageLines+rowLines > printPageLines {
			b.WriteString("\f") // page eject: next sheet starts with the header
			writeHeader()
		}
		for bar := rowStart; bar < rowEnd; bar++ {
			b.WriteString(barHeaderLine(tab, bar, layout.barWidth))
		}
		b.WriteString("\n")
		for s := range rowStrings {
			for bar := rowStart; bar < rowEnd; bar++ {
				b.WriteString(stringRow(tab, tab.Bars[bar], s, layout.barWidth))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
		pageLines += rowLines
	}
	return b.String()
}

// clampWidth applies the page-width rule: min(width, 100), defaulting 0 to
// 80 and never going below the 22 columns a bar row needs.
func clampWidth(width int) int {
	if width <= 0 {
		width = 80
	}
	if width > printMaxWidth {
		width = printMaxWidth
	}
	if width < printMinWidth {
		width = printMinWidth
	}
	return width
}

// printLayout is the grid packing for one tab at a fixed page width:
// how many bars sit side by side and how wide each bar column is (string
// prefix included). Guarantees barsPerRow*barWidth <= pageWidth.
type printLayout struct {
	barsPerRow int
	barWidth   int
}

func printLayoutFor(tab *model.Tab, pageWidth int) printLayout {
	natural := 5 // "| 1 |" is the narrowest meaningful column
	for _, bar := range tab.Bars {
		natural = max(natural, barContentWidth(bar)+3)
	}
	barsPerRow := pageWidth / minBarWidth
	barsPerRow = min(max(barsPerRow, 1), maxBarsPerRow)
	barWidth := pageWidth / barsPerRow
	if barWidth < natural {
		barWidth = natural
		barsPerRow = pageWidth / barWidth
		if barsPerRow < 1 {
			barsPerRow = 1
		}
	}
	if barWidth > pageWidth { // one absurdly wide bar: clip column to the page
		barWidth = pageWidth
	}
	return printLayout{barsPerRow: barsPerRow, barWidth: barWidth}
}

// barContentWidth is the natural width in columns of a bar's string rows.
func barContentWidth(bar model.Bar) int {
	m := 0
	for _, str := range bar.Strings {
		for _, seg := range str.Segments {
			m = max(m, seg.Position+seg.Width)
		}
	}
	return m
}

// barHeaderLine renders a bar's header row padded to width: "| 1 ------|"
// with the section name (when the bar starts a section) and repeat/ending
// markers ("|:", ":|", "1."/"2.") mirroring the on-screen grid header.
func barHeaderLine(tab *model.Tab, barIdx int, width int) string {
	bar := tab.Bars[barIdx]
	open, closeM, ending := "|", "|", ""
	if bar.RepeatStart {
		open = "|:"
	}
	if bar.RepeatEnd {
		closeM = ":|"
	}
	if bar.Ending == 1 || bar.Ending == 2 {
		ending = fmt.Sprintf("%d.", bar.Ending)
	}
	num := bar.Number
	if num <= 0 {
		num = barIdx + 1
	}
	head := open + " " + strconv.Itoa(num) + " "
	if ending != "" {
		head += ending + " "
	}
	if name := sectionStart(tab, barIdx); name != "" {
		room := width - runeLen(head) - runeLen(closeM) - 1
		if room >= 1 {
			head += truncateRunes(name, room)
		}
	}
	fill := width - runeLen(head) - runeLen(closeM)
	if fill < 1 {
		fill = 1
	}
	line := head + strings.Repeat("-", fill) + closeM
	if runeLen(line) > width { // pathological narrow column: force fit
		line = truncateRunes(head, width-runeLen(closeM)) + closeM
	}
	return padTo(line, width)
}

// sectionStart returns the bar's section name when it begins a new section
// (the previous bar has a different or no section), mirroring the viewer.
func sectionStart(tab *model.Tab, barIdx int) string {
	if barIdx < 0 || barIdx >= len(tab.Bars) {
		return ""
	}
	s := strings.TrimSpace(tab.Bars[barIdx].Section)
	if s == "" {
		return ""
	}
	if barIdx > 0 && strings.TrimSpace(tab.Bars[barIdx-1].Section) == s {
		return ""
	}
	return s
}

// stringRow renders one string's row for a bar padded to colWidth:
// "|E|--0--3--" — the 3-column string prefix then the dash content with
// fret digits and technique characters. Bars with fewer strings than the
// row maximum render an empty content area so columns stay aligned.
func stringRow(tab *model.Tab, bar model.Bar, s, colWidth int) string {
	prefix := "|?|"
	if tab.Tuning != nil && s >= 0 && s < len(tab.Tuning) {
		if name := tab.Tuning.NoteName(s); name != "" {
			prefix = "|" + string([]rune(name)[0]) + "|"
		}
	}
	content := renderStringContentPlain(bar, s)
	area := colWidth - 3
	if area < 0 {
		area = 0
	}
	if len(content) > area { // degenerate ultra-wide bar: clip, never overflow
		content = content[:area]
	}
	return prefix + content + strings.Repeat(" ", area-len(content))
}

// renderStringContentPlain draws a string line as the classic ASCII tab
// form: a dash row with fret digits and technique characters (h, p, /, x,
// ~, ...) in place. Rests and other non-display characters stay dashes.
func renderStringContentPlain(bar model.Bar, s int) string {
	if s >= len(bar.Strings) {
		return ""
	}
	width := 0
	for _, seg := range bar.Strings[s].Segments {
		width = max(width, seg.Position+seg.Width)
	}
	row := []byte(strings.Repeat("-", width))
	for _, seg := range bar.Strings[s].Segments {
		var digits string
		switch {
		case seg.Value > 0:
			digits = strconv.Itoa(seg.Value)
		case seg.Char == '0':
			digits = "0" // open string
		default:
			if seg.Char == '-' || seg.Char == 0 {
				continue // dash already in the row; skip non-characters
			}
			digits = string(seg.Char) // technique/rest character
		}
		for i := 0; i < len(digits) && seg.Position+i < width; i++ {
			row[seg.Position+i] = digits[i]
		}
	}
	return string(row)
}

// barLines returns the plain-text lines for one bar: line 0 is the bar
// header, lines 1..N the string rows, all padded to colWidth. PrintTab
// composes its grid from barHeaderLine/stringRow directly; HTMLTab uses
// this so the exported HTML shows exactly the printed bars.
func barLines(tab *model.Tab, barIdx int, colWidth int) []string {
	bar := tab.Bars[barIdx]
	lines := make([]string, 0, len(bar.Strings)+1)
	lines = append(lines, barHeaderLine(tab, barIdx, colWidth))
	for s := range bar.Strings {
		lines = append(lines, stringRow(tab, bar, s, colWidth))
	}
	return lines
}

// tabHeaderLines returns the two header lines shown atop every printed page
// and in the HTML export: "Title — Artist" and "Tuning · Tempo · Capo"
// (only the metadata present is included).
func tabHeaderLines(tab *model.Tab) [2]string {
	title := strings.TrimSpace(tab.Title)
	if title == "" {
		title = "Untitled"
	}
	artist := strings.TrimSpace(tab.Artist)
	line1 := title
	if artist != "" {
		line1 += " — " + artist
	}
	var parts []string
	if tab.Tuning != nil && len(tab.Tuning) > 0 {
		parts = append(parts, "Tuning: "+tab.Tuning.Label())
	}
	if bpm := strings.TrimSpace(tab.Metadata[model.MetaKeyBPM]); bpm != "" {
		parts = append(parts, "Tempo: "+bpm+" BPM")
	} else if tempo := strings.TrimSpace(tab.Metadata[model.MetaKeyTempo]); tempo != "" {
		parts = append(parts, "Tempo: "+tempo)
	}
	if capo := strings.TrimSpace(tab.Metadata[model.MetaKeyCapo]); capo != "" {
		parts = append(parts, "Capo: "+capo)
	}
	return [2]string{line1, strings.Join(parts, " · ")}
}

// truncateRunes shortens s to at most max runes, appending "..." when it
// had to cut. Rune-based so section names with non-ASCII text stay valid.
func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	rs := []rune(s)
	if max < 4 {
		return string(rs[:max])
	}
	return string(rs[:max-3]) + "..."
}

func runeLen(s string) int { return utf8.RuneCountInString(s) }

func padTo(s string, width int) string {
	if n := width - runeLen(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
}
