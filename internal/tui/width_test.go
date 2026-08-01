package tui

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
		if got := lipglossWidth(c.in); got != c.want {
			t.Errorf("lipglossWidth(%q) = %d, want %d (byte len %d)", c.in, got, c.want, len(c.in))
		}
	}
}

func TestRenderStatusBarMultibyteAlignment(t *testing.T) {
	const width = 90
	info := StatusInfo{Filename: "café.txt", Tuning: "E standard", BPM: 120}
	right := "j/k:scroll  Space:play  /:search  q:quit"

	out := RenderStatusBar(width, info)

	idx := strings.Index(out, "j/k:scroll")
	if idx < 0 {
		t.Fatal("right segment missing from status bar")
	}
	if !strings.Contains(out, "café.txt") {
		t.Fatal("left segment missing from status bar")
	}

	// statusBarStyle has 1 cell of horizontal padding on each side.
	want := width - lipgloss.Width(right) - 1
	if got := lipgloss.Width(out[:idx]); got != want {
		t.Fatalf("right segment starts at cell %d, want %d (status bar misaligned)", got, want)
	}
}

func TestTruncateNoBrokenUTF8(t *testing.T) {
	out := truncate("abcdefg中hijklmno", 10)
	if strings.Contains(out, "\uFFFD") {
		t.Fatalf("truncate produced replacement char: %q", out)
	}
	if !strings.HasSuffix(out, "…") {
		t.Fatalf("truncate result should end with …: %q", out)
	}
	if got := lipglossWidth(out); got > 10 {
		t.Fatalf("truncate result width %d exceeds max 10: %q", got, out)
	}
}

func TestTruncateErrNoBrokenUTF8(t *testing.T) {
	out := truncateErr("aaaaaaaézzzz", 10)
	if strings.Contains(out, "\uFFFD") {
		t.Fatalf("truncateErr produced replacement char: %q", out)
	}
	if !strings.HasSuffix(out, "…") {
		t.Fatalf("truncateErr result should end with …: %q", out)
	}
	if got := lipglossWidth(out); got > 10 {
		t.Fatalf("truncateErr result width %d exceeds max 10: %q", got, out)
	}
}
