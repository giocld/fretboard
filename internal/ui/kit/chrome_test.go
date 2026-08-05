package kit

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// TestRenderFooterFitsWidth guards against footers that overflow narrow
// terminals: the hint bar must never wrap past the given width (old behavior
// rendered the full hint list and pushed the panel body out of view).
func TestRenderFooterFitsWidth(t *testing.T) {
	hints := []KeyHint{
		{Key: "q", Label: "quit"},
		{Key: "j/k", Label: "move"},
		{Key: "Enter", Label: "open"},
		{Key: "o", Label: "online search"},
		{Key: "f", Label: "favorite"},
		{Key: "d", Label: "delete"},
		{Key: "t", Label: "theme"},
		{Key: "/", Label: "filter"},
		{Key: "r", Label: "reload"},
		{Key: "s", Label: "sort"},
		{Key: "?", Label: "help"},
	}
	for _, width := range []int{40, 60, 80, 120} {
		got := RenderFooter(width, hints)
		lines := strings.Split(got, "\n")
		if len(lines) != 1 {
			t.Fatalf("width %d: footer wraps into %d lines", width, len(lines))
		}
		if w := lipgloss.Width(got); w > width {
			t.Fatalf("width %d: footer is %d columns", width, w)
		}
	}
	// A tiny width still yields a footer (or an empty one), never a panic.
	RenderFooter(10, hints)
	// Empty hints produce no footer.
	if got := RenderFooter(80, nil); got != "" {
		t.Fatalf("empty hints should render nothing, got %q", got)
	}
}
