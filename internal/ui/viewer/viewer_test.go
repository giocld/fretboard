package viewer

import (
	"strings"
	"testing"
	"time"

	"fretboard/internal/model"
	tea "github.com/charmbracelet/bubbletea"
)

func TestMaxPanOffsetGridAware(t *testing.T) {
	m := NewViewerModel()
	tab := &model.Tab{
		Title:  "Wide",
		Artist: "Test",
		Tuning: model.ParseTuning("EADGBE"),
		Bars: []model.Bar{
			{Number: 1, Strings: []model.StringLine{{Segments: []model.Segment{{Position: 0, Width: 24}}}}},
		},
	}
	m.LoadTab(tab, "", 0)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 86, Height: 30})
	if got := m.maxPanOffset(); got != 0 {
		t.Fatalf("maxPanOffset on wide terminal = %d, want 0 (grid fits)", got)
	}
	tab.Bars[0].Strings[0].Segments[0].Width = 80
	m, _ = m.Update(tea.WindowSizeMsg{Width: 40, Height: 30})
	if got := m.maxPanOffset(); got <= 0 {
		t.Fatalf("maxPanOffset on narrow terminal with wide bar = %d, want > 0", got)
	}
}

func TestLayoutToggleSwitchesRenderer(t *testing.T) {
	m := NewViewerModel()
	m.LoadTab(sampleTab(), "", 0)
	if m.linear {
		t.Fatal("default layout should be grid")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	if !m.linear {
		t.Fatal("v should toggle to linear layout")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	if m.linear {
		t.Fatal("v should toggle back to grid")
	}
}

func sampleTab() *model.Tab {
	lines := make([]model.StringLine, 6)
	for i := range lines {
		lines[i] = model.StringLine{Segments: []model.Segment{{Char: '1', Value: 1, Position: 0, Width: 1}}}
	}
	return &model.Tab{
		Title:    "Sample",
		Tuning:   model.Standard,
		Bars:     []model.Bar{{Number: 1, Strings: lines}, {Number: 2, Strings: lines}, {Number: 3, Strings: lines}},
		Metadata: map[string]string{},
	}
}

// TestTrackEndedBanner guards S2.4: an audio file that ends before the tab
// finishes produces an explanatory message with a restart hint.
func TestTrackEndedBanner(t *testing.T) {
	got := trackEndedBanner(4*time.Minute + 12*time.Second)
	for _, want := range []string{"4:12", "Track ended", "Space restarts"} {
		if !strings.Contains(got, want) {
			t.Fatalf("banner %q missing %q", got, want)
		}
	}
	if got := trackEndedBanner(0); !strings.Contains(got, "Track ended") {
		t.Fatalf("unknown-duration banner should still explain, got %q", got)
	}
}

func key(k string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}
