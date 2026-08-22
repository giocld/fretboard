package viewer

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"fretboard/internal/export"
	"fretboard/internal/model"
	"fretboard/internal/parser"
	"fretboard/internal/ui/kit"
	"fretboard/internal/ui/msgs"
	tea "github.com/charmbracelet/bubbletea"
)

// editDoneMsg is delivered when the quick-edit editor process exits. The
// path names the temp file the editor worked on; err is nil on a clean exit.
type editDoneMsg struct {
	path string
	err  error
}

// browserOpenMsg reports a failed external-browser launch (S5.2 g key).
type browserOpenMsg struct{ err error }

// startQuickEdit writes the tab's raw text to a temp file and spawns
// $EDITOR (fallback vi/nano/notepad) on it. When the editor exits, the
// re-parsed result replaces the loaded tab and msgs.EditPersistMsg asks the
// app to persist it. Only local tabs are editable: an online tab has no file
// the editor could write.
func (m ViewerModel) startQuickEdit() (ViewerModel, tea.Cmd) {
	m.jumpBuffer = ""
	if m.tab == nil {
		m.errMsg = "No tab to edit"
		m.refresh()
		return m, nil
	}
	if m.editing {
		return m, nil
	}
	raw, err := os.ReadFile(m.tabPath)
	if err != nil && m.tab.Metadata != nil {
		// Chord sheets keep their raw text in metadata, so they are
		// editable even without a local file.
		if r := m.tab.Metadata["raw"]; r != "" {
			raw, err = []byte(r), nil
		}
	}
	if err != nil {
		m.errMsg = "Cannot read the tab for editing: " + err.Error()
		m.refresh()
		return m, nil
	}
	tmp, err := os.CreateTemp("", "fretboard-edit-*.txt")
	if err != nil {
		m.errMsg = "Cannot create the edit file: " + err.Error()
		m.refresh()
		return m, nil
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		m.errMsg = "Cannot write the edit file: " + err.Error()
		m.refresh()
		return m, nil
	}
	tmp.Close()
	m.editing = true
	m.editPath = tmpName
	m.errMsg = ""
	m.infoMsg = "Editing — save and quit the editor to apply"
	m.refresh()
	return m, editorCmd(pickEditor(), tmpName)
}

// editorCmd runs the editor through tea.ExecProcess so the TUI suspends
// while the editor owns the terminal, then reports back via editDoneMsg.
func editorCmd(editor, path string) tea.Cmd {
	c := exec.Command(editor, path)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return editDoneMsg{path: path, err: err}
	})
}

// pickEditor resolves the editor binary: $EDITOR first, then a per-OS
// fallback (vi/nano on unix, notepad on windows).
func pickEditor() string {
	if e := strings.TrimSpace(os.Getenv("EDITOR")); e != "" {
		return e
	}
	if runtime.GOOS == "windows" {
		return "notepad"
	}
	for _, e := range []string{"vi", "nano"} {
		if _, err := exec.LookPath(e); err == nil {
			return e
		}
	}
	return "vi"
}

// handleEditDone re-reads the edited temp file, re-parses it, and either
// swaps the new tab in (with a changed-bars summary and an EditPersistMsg)
// or keeps the old tab and surfaces the parse error.
func (m ViewerModel) handleEditDone(msg editDoneMsg) (ViewerModel, tea.Cmd) {
	m.editing = false
	m.editPath = ""
	if msg.err != nil {
		m.errMsg = "Editor failed: " + msg.err.Error()
		m.refresh()
		return m, nil
	}
	content, err := os.ReadFile(msg.path)
	os.Remove(msg.path)
	if err != nil {
		m.errMsg = "Cannot read the edited file: " + err.Error()
		m.refresh()
		return m, nil
	}
	newTab, err := parser.Parse(bytes.NewReader(content))
	if err != nil || (len(newTab.Bars) == 0 && newTab.Metadata["kind"] != "chords") {
		// Keep the previous tab. Parse rarely errors on text, so a re-parse
		// that produced no playable content counts as a failed edit too.
		reason := "the edit produced no tab"
		if err != nil {
			reason = err.Error()
		}
		m.errMsg = "Edit re-parse failed — keeping the previous tab: " + reason
		m.refresh()
		return m, nil
	}
	if newTab.Title == "" && m.tab != nil {
		newTab.Title = m.tab.Title
	}
	if newTab.Artist == "" && m.tab != nil {
		newTab.Artist = m.tab.Artist
	}
	// Practice time survives an edit: it is session-acquired, not textual.
	if m.tab != nil && m.tab.Metadata != nil {
		if newTab.Metadata == nil {
			newTab.Metadata = map[string]string{}
		}
		if v := m.tab.Metadata["practice_seconds"]; v != "" {
			newTab.Metadata["practice_seconds"] = v
		}
	}
	summary := changedBarsSummary(m.tab, newTab)
	m.LoadTab(newTab, m.tabPath, m.tabID)
	m.infoMsg = "edit applied — " + summary
	m.errMsg = ""
	m.warnMsg = ""
	m.refresh()
	emit := msgs.EditPersistMsg{
		TabID:   m.tabID,
		Path:    m.tabPath,
		Content: string(content),
		Title:   newTab.Title,
		Artist:  newTab.Artist,
	}
	return m, func() tea.Msg { return emit }
}

// changedBarsSummary compares the pre-edit and re-parsed tabs bar by bar
// (structure, sections, repeat flags and every segment) and names the bars
// that differ, e.g. "bar 12, 14 modified".
func changedBarsSummary(oldT, newT *model.Tab) string {
	if oldT == nil || newT == nil {
		return "full re-parse"
	}
	n := min(len(oldT.Bars), len(newT.Bars))
	var changed []int
	for i := 0; i < n; i++ {
		if !barsEqual(oldT.Bars[i], newT.Bars[i]) {
			changed = append(changed, i+1)
		}
	}
	for i := n; i < len(newT.Bars); i++ {
		changed = append(changed, i+1) // bars added by the edit
	}
	if len(oldT.Bars) != len(newT.Bars) {
		return fmt.Sprintf("bar count %d → %d (bar %s)", len(oldT.Bars), len(newT.Bars), barList(changed))
	}
	if len(changed) == 0 {
		return "no bar changes"
	}
	return "bar " + barList(changed) + " modified"
}

func barList(bars []int) string {
	parts := make([]string, 0, len(bars))
	for _, b := range bars {
		parts = append(parts, strconv.Itoa(b))
	}
	return strings.Join(parts, ", ")
}

// barsEqual compares two bars structurally: section, repeat structure,
// ending, and every segment's rendered character and position.
func barsEqual(x, y model.Bar) bool {
	if x.Section != y.Section || x.RepeatStart != y.RepeatStart ||
		x.RepeatEnd != y.RepeatEnd || x.Ending != y.Ending || x.Capo != y.Capo {
		return false
	}
	if len(x.Strings) != len(y.Strings) || len(x.Rhythm) != len(y.Rhythm) {
		return false
	}
	for i := range x.Rhythm {
		if x.Rhythm[i] != y.Rhythm[i] {
			return false
		}
	}
	for i := range x.Strings {
		sx, sy := x.Strings[i], y.Strings[i]
		if len(sx.Segments) != len(sy.Segments) {
			return false
		}
		for j := range sx.Segments {
			a, b := sx.Segments[j], sy.Segments[j]
			if a.Char != b.Char || a.Value != b.Value || a.Position != b.Position || a.Width != b.Width {
				return false
			}
		}
	}
	return true
}

// printHTML writes the HTML export next to the tab file (same dir, .html
// suffix) and reports the written path in the status line. Handles a nil
// tab gracefully.
func (m ViewerModel) printHTML() (ViewerModel, tea.Cmd) {
	m.jumpBuffer = ""
	if m.tab == nil {
		m.errMsg = "No tab to print"
		m.refresh()
		return m, nil
	}
	html := export.HTMLTab(m.tab, kit.CurrentTheme().Name)
	if html == "" {
		m.errMsg = "Nothing to print (empty tab)"
		m.refresh()
		return m, nil
	}
	dir := "."
	base := sanitizeBase(m.tab.Title)
	if m.tabPath != "" {
		dir = filepath.Dir(m.tabPath)
		if p := filepath.Base(m.tabPath); p != "." {
			if ext := filepath.Ext(p); ext != "" {
				base = strings.TrimSuffix(p, ext)
			} else {
				base = strings.TrimSuffix(p, filepath.Ext(p))
			}
		}
	}
	if base == "" {
		base = "tab"
	}
	path := filepath.Join(dir, base+".html")
	if err := os.WriteFile(path, []byte(html), 0o644); err != nil {
		m.errMsg = "Print failed: " + err.Error()
		m.refresh()
		return m, nil
	}
	m.infoMsg = "printed " + path
	m.errMsg = ""
	m.refresh()
	return m, nil
}

// sanitizeBase strips characters that are unsafe in file names so a tab
// title can become an export file name.
func sanitizeBase(title string) string {
	var b strings.Builder
	for _, r := range title {
		switch {
		case r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' ||
			r == '"' || r == '<' || r == '>' || r == '|':
			b.WriteRune(' ')
		default:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

// sourceURL returns the canonical provider page for the loaded tab, if one
// was recorded at import (S5.2: the g key opens it).
func (m ViewerModel) sourceURL() string {
	if m.tab == nil || m.tab.Metadata == nil {
		return ""
	}
	return strings.TrimSpace(m.tab.Metadata["source_url"])
}

// openBrowserCmd opens a URL in the platform's default browser without
// blocking the Update loop, reporting failures via browserOpenMsg.
func openBrowserCmd(url string) tea.Cmd {
	return func() tea.Msg {
		if err := openURL(url); err != nil {
			return browserOpenMsg{err: err}
		}
		return nil
	}
}

// openURL launches the default browser for the platform: open on macOS,
// rundll32 on Windows, xdg-open elsewhere.
func openURL(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}

// handleBrowserOpen surfaces a browser-launch failure (the common case —
// a headless session — is a hint, not a crash).
func (m ViewerModel) handleBrowserOpen(msg browserOpenMsg) (ViewerModel, tea.Cmd) {
	m.errMsg = "Cannot open the browser: " + msg.err.Error()
	m.refresh()
	return m, nil
}
