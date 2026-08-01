package kit

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestLipglossWidthReportsDisplayWidth(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"abc", 3},
		{"é", 1},
		{"中", 2},
		{"café.txt", 8},
	}
	for _, c := range cases {
		if got := lipgloss.Width(c.in); got != c.want {
			t.Errorf("lipgloss.Width(%q) = %d, want %d (byte len %d)", c.in, got, c.want, len(c.in))
		}
	}
}

func TestRenderStatusBarMultibyteAlignment(t *testing.T) {
	const width = 90
	info := StatusInfo{Filename: "café.txt", Tuning: "E standard", BPM: 120}
	right := "j/k:scroll  Space:play  /:search  q:quit"
	out := RenderStatusBar(width, info)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) == 0 {
		t.Fatal("no status bar output")
	}
	last := lines[len(lines)-1]
	if lipgloss.Width(last) > width {
		t.Fatalf("status bar width %d exceeds %d: %q", lipgloss.Width(last), width, last)
	}
	if !strings.Contains(last, right) {
		t.Fatalf("status bar missing right-aligned hints: %q", last)
	}
}

func TestTruncateNoBrokenUTF8(t *testing.T) {
	got := Truncate("café 中文", 7)
	if strings.Contains(got, "\uFFFD") {
		t.Fatalf("truncate split a rune: %q", got)
	}
	if lipgloss.Width(got) > 7 {
		t.Fatalf("truncate overflow: %q is %d cols", got, lipgloss.Width(got))
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("truncate should append ellipsis: %q", got)
	}
}

func TestTruncateErrNoBrokenUTF8(t *testing.T) {
	got := Truncate("x", 2)
	if got != "x" {
		t.Fatalf("short input should be returned unchanged, got %q", got)
	}
}
