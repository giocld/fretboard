package export

import (
	"fmt"
	"html"
	"strconv"
	"strings"

	"fretboard/internal/model"
)

// htmlBarsPerPage groups this many bars per printed sheet; the CSS rule
// below starts a new page for each group after the first, so a song prints
// in readable chunks instead of one page per bar.
const htmlBarsPerPage = 6

// htmlPalette is one theme's CSS variable set: page background/foreground,
// muted accent, and one color per string (index = string, low to high).
type htmlPalette struct {
	bg, fg, muted string
	strings       [8]string
}

var htmlThemes = map[string]htmlPalette{
	"default": {
		bg: "#ffffff", fg: "#1a1a1a", muted: "#767676",
		strings: [8]string{"#c62828", "#e65100", "#b8860b", "#2e7d32", "#1565c0", "#6a1b9a", "#00695c", "#ad1457"},
	},
	"dark": {
		bg: "#101418", fg: "#d8dee9", muted: "#7f8c98",
		strings: [8]string{"#f07178", "#f78c6c", "#ffcb6b", "#c3e88d", "#82aaff", "#c792ea", "#89ddff", "#f07178"},
	},
	"dracula": {
		bg: "#282a36", fg: "#f8f8f2", muted: "#6272a4",
		strings: [8]string{"#ff5555", "#ffb86c", "#f1fa8c", "#50fa7b", "#8be9fd", "#bd93f3", "#ff79c6", "#ffb86c"},
	},
}

// paletteFor resolves a theme name; anything unknown falls back to the
// default (light) palette.
func paletteFor(theme string) htmlPalette {
	if p, ok := htmlThemes[strings.ToLower(strings.TrimSpace(theme))]; ok {
		return p
	}
	return htmlThemes["default"]
}

// HTMLTab renders a tab as a self-contained HTML document — inline <style>,
// no external assets — suitable for printing from a browser or viewing
// locally. theme selects the palette: "default" (light), "dark", or
// "dracula"; unknown themes fall back to "default". Bars are grouped
// htmlBarsPerPage to a printed page via a CSS page-break-before rule, every
// string row carries a color class, and all tab text is HTML-escaped. The
// rendered bars are exactly the plain text PrintTab produces, so a printed
// HTML file matches the lpr output.
func HTMLTab(tab *model.Tab, theme string) string {
	if tab == nil || len(tab.Bars) == 0 {
		return ""
	}
	pal := paletteFor(theme)
	header := tabHeaderLines(tab)
	title := html.EscapeString(header[0])

	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n")
	b.WriteString("<meta charset=\"utf-8\">\n")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	b.WriteString("<title>" + title + "</title>\n")
	b.WriteString("<style>\n")
	b.WriteString(cssFor(pal))
	b.WriteString("</style>\n</head>\n<body>\n")
	b.WriteString("<h1>" + title + "</h1>\n")
	if meta := html.EscapeString(header[1]); meta != "" {
		b.WriteString("<p class=\"meta\">" + meta + "</p>\n")
	}
	b.WriteString("<div class=\"tab\">\n")
	for i := range tab.Bars {
		if i%htmlBarsPerPage == 0 {
			if i > 0 {
				b.WriteString("</div>\n")
			}
			b.WriteString("<div class=\"page-group\">\n")
		}
		b.WriteString(barHTML(tab, i))
	}
	b.WriteString("</div>\n")
	b.WriteString("</div>\n</body>\n</html>\n")
	return b.String()
}

// cssFor emits the inline stylesheet for a palette: monospace body font,
// theme colors as CSS variables, per-string color classes, and the printed
// page-break rule for bar groups.
func cssFor(p htmlPalette) string {
	var b strings.Builder
	b.WriteString(":root {\n")
	b.WriteString("  --bg: " + p.bg + ";\n")
	b.WriteString("  --fg: " + p.fg + ";\n")
	b.WriteString("  --muted: " + p.muted + ";\n")
	for i := range p.strings {
		fmt.Fprintf(&b, "  --s%d: %s;\n", i, p.strings[i])
	}
	b.WriteString("}\n")
	b.WriteString("body { margin: 2em auto; max-width: 60em; padding: 0 1em;\n")
	b.WriteString("  background: var(--bg); color: var(--fg);\n")
	b.WriteString("  font-family: \"Courier New\", Courier, \"Liberation Mono\", Menlo, monospace; }\n")
	b.WriteString("h1 { font-size: 1.5em; margin: 0 0 0.2em; }\n")
	b.WriteString(".meta { color: var(--muted); margin: 0 0 1.5em; }\n")
	b.WriteString(".bar { margin: 0 0 1.2em; }\n")
	b.WriteString(".barhead { color: var(--muted); }\n")
	b.WriteString("pre.strings { margin: 0.1em 0 0; line-height: 1.35; }\n")
	b.WriteString("@media print {\n")
	b.WriteString("  .page-group + .page-group { page-break-before: always; }\n")
	b.WriteString("}\n")
	for i := range p.strings {
		fmt.Fprintf(&b, ".s%d { color: var(--s%d); }\n", i, i)
	}
	return b.String()
}

// barHTML renders one bar as a header div plus a <pre> of color-classed
// string rows; all text is escaped and trailing column padding trimmed.
func barHTML(tab *model.Tab, barIdx int) string {
	colWidth := max(5, barContentWidth(tab.Bars[barIdx])+3)
	lines := barLines(tab, barIdx, colWidth)
	var b strings.Builder
	b.WriteString("<div class=\"bar\">\n")
	b.WriteString("<div class=\"barhead\">" + html.EscapeString(lines[0]) + "</div>\n")
	b.WriteString("<pre class=\"strings\">")
	for s := 1; s < len(lines); s++ {
		b.WriteString(`<span class="s` + strconv.Itoa(s-1) + `">`)
		b.WriteString(html.EscapeString(strings.TrimRight(lines[s], " ")))
		b.WriteString("</span>")
		if s < len(lines)-1 {
			b.WriteString("\n")
		}
	}
	b.WriteString("</pre>\n</div>\n")
	return b.String()
}
