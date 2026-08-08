package kit

import (
	"os"
	"strings"

	"fretboard/internal/clipboard"
	"fretboard/internal/model"
)

// ExportTab writes a tab's plain ASCII form (RenderTabPlain) to
// "<Title>.txt" in the working directory and copies it to the clipboard when
// a tool is available. The returned message reports both outcomes for a
// status line.
func ExportTab(tab *model.Tab) (string, string) {
	if tab == nil {
		return "", "Export failed: no tab loaded"
	}
	text := RenderTabPlain(tab)
	name := sanitizeExportName(tab.Title)
	if name == "" {
		name = "tab"
	}
	path := name + ".txt"
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		return "", "Export failed: " + err.Error()
	}
	msg := "Exported " + path
	if err := clipboard.Copy(text); err == nil {
		msg += " · copied to clipboard"
	} else {
		msg += " · clipboard unavailable"
	}
	return path, msg
}

// sanitizeExportName keeps filename-safe characters for exports.
func sanitizeExportName(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|', '\n', '\r', '\t':
			continue
		default:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}
