// Package export writes tabs out of the app: a plain-ASCII tab file in the
// working directory plus a clipboard copy when a tool is available. It keeps
// file and clipboard I/O out of the presentational kit package.
package export

import (
	"os"
	"strings"

	"fretboard/internal/clipboard"
	"fretboard/internal/model"
	"fretboard/internal/ui/kit"
)

// Tab writes a tab's plain ASCII form (kit.RenderTabPlain) to
// "<Title>.txt" in the working directory and copies it to the clipboard when
// a tool is available. The returned message reports both outcomes for a
// status line.
func Tab(tab *model.Tab) (string, string) {
	if tab == nil {
		return "", "Export failed: no tab loaded"
	}
	text := kit.RenderTabPlain(tab)
	name := sanitizeName(tab.Title)
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

// sanitizeName keeps filename-safe characters for exports.
func sanitizeName(s string) string {
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
