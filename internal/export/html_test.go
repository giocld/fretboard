package export

import (
	"strings"
	"testing"

	"fretboard/internal/model"
)

func TestHTMLTabEscapesText(t *testing.T) {
	tab := &model.Tab{
		Title:  "A<B&C>D",
		Artist: "X&Y",
		Tuning: model.Standard,
		Bars: []model.Bar{
			{Number: 1, Section: "Riff <A>", Strings: []model.StringLine{
				lineFrom("--0--3--3--3--"), lineFrom("--<&>3--3--"), lineFrom("--7--7--7--7--"),
				lineFrom("--0--3--3--3--"), lineFrom("--<&>3--3--"), lineFrom("--7--7--7--7--"),
			}},
		},
	}
	out := HTMLTab(tab, "default")
	for _, want := range []string{
		"A&lt;B&amp;C&gt;D", "X&amp;Y",
		"Riff &lt;A&gt;", // section name in the bar header
		"--&lt;&amp;&gt;3--3--",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected escaped %q in output", want)
		}
	}
	for _, bad := range []string{"A<B&C>D", "--<&>3--3--", "<script", "<link", "http://", "https://"} {
		if strings.Contains(out, bad) {
			t.Errorf("unescaped or external asset %q in output", bad)
		}
	}
}

func TestHTMLTabStructure(t *testing.T) {
	out := HTMLTab(sampleTab(8), "dark")
	for _, want := range []string{
		"<!DOCTYPE html>",
		"<style>",
		"font-family:",
		"monospace",
		"page-break-before",
		"--bg: #101418",
		`<span class="s0">|E|--0--3--</span>`,
		`<span class="s5">|E|--0--3--</span>`,
		"Wonderwall — Oasis",
		"Tuning: EADGBE",
		"Tempo: 96 BPM",
		"Capo: 2",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("HTML missing %q", want)
		}
	}
	// 8 bars at htmlBarsPerPage per sheet -> two page groups.
	if n := strings.Count(out, `<div class="page-group">`); n != 2 {
		t.Errorf("expected 2 page groups, got %d", n)
	}
}

func TestHTMLTabThemes(t *testing.T) {
	cases := []struct {
		theme string
		bg    string
	}{
		{"default", "#ffffff"},
		{"dark", "#101418"},
		{"dracula", "#282a36"},
		{"DRACULA", "#282a36"},  // case-insensitive
		{"nonsense", "#ffffff"}, // unknown -> default
		{"", "#ffffff"},
	}
	for _, c := range cases {
		out := HTMLTab(sampleTab(2), c.theme)
		if !strings.Contains(out, "--bg: "+c.bg) {
			t.Errorf("theme %q: expected background %s", c.theme, c.bg)
		}
	}
}

func TestHTMLTabRoundTripsMultiBarTab(t *testing.T) {
	tab := &model.Tab{Title: "RT", Tuning: model.Standard, Bars: []model.Bar{
		{Number: 1, Strings: []model.StringLine{
			lineFrom("--0--3--"), lineFrom("--12-3--"), lineFrom("--3h5---"),
			lineFrom("--0--3--"), lineFrom("--12-3--"), lineFrom("--3h5---"),
		}},
		{Number: 2, Strings: []model.StringLine{
			lineFrom("--7--7--"), lineFrom("--9--9--"), lineFrom("--5--5--"),
			lineFrom("--7--7--"), lineFrom("--9--9--"), lineFrom("--5--5--"),
		}},
	}}
	out := HTMLTab(tab, "dracula")
	for _, want := range []string{
		"|E|--0--3--", "|A|--12-3--", "|D|--3h5---",
		"|G|--7--7--", "|B|--9--9--", "|E|--5--5--",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("HTML round-trip lost %q", want)
		}
	}
	if !strings.Contains(out, "page-break-before") {
		t.Errorf("missing page-break rule")
	}
}

func TestHTMLTabNil(t *testing.T) {
	if out := HTMLTab(nil, "default"); out != "" {
		t.Fatalf("nil tab should render empty, got %q", out)
	}
}
