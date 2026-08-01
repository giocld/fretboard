package home

import (
	"fmt"
	"strings"
	"testing"

	"github.com/YOUR_USERNAME/fretboard/internal/library"
	"github.com/YOUR_USERNAME/fretboard/internal/ui/msgs"
	"github.com/charmbracelet/lipgloss"
)

func TestHomeTabsLoadErrorShowsMessage(t *testing.T) {
	m := NewHomeModel(nil)
	m, _ = m.Update(msgs.TabsLoadErrorMsg{Err: fmt.Errorf("disk full")})
	if !m.loaded {
		t.Fatal("should mark loaded after error")
	}
	if m.errMsg == "" {
		t.Fatal("expected load error message")
	}
	body := m.renderBody()
	if body == "" {
		t.Fatal("expected body")
	}
}

func TestHomePreviewFollowsRecentCursor(t *testing.T) {
	m := HomeModel{
		store:  nil,
		loaded: true,
		tabs: []library.TabRow{
			{ID: 10, Title: "Older"},
			{ID: 20, Title: "Newer"},
		},
		cursor: homeActionCount + 1,
	}
	preview := m.loadPreview()
	if preview != "" {
		t.Fatal("preview should be empty without store")
	}
}

func TestHomeRecentTabsPreferLastPlayed(t *testing.T) {
	m := HomeModel{
		loaded: true,
		tabs: []library.TabRow{
			{ID: 10, Title: "Older", LastPlayed: "2026-01-01 10:00:00"},
			{ID: 20, Title: "Newer", LastPlayed: "2026-03-01 10:00:00"},
			{ID: 30, Title: "Never"},
		},
	}
	recent := m.recentTabs()
	if len(recent) != 3 || recent[0].ID != 20 || recent[1].ID != 10 || recent[2].ID != 30 {
		t.Fatalf("recent tabs order = %+v", recent)
	}
}

func TestHomePreviewRenderedInBody(t *testing.T) {
	m := HomeModel{loaded: true, preview: "PREVIEW_MARKER"}
	body := m.renderBody()
	if !strings.Contains(body, "PREVIEW_MARKER") {
		t.Fatal("renderBody should include preview panel content")
	}
}

func TestHomeClampCursorOnTabsLoaded(t *testing.T) {
	m := HomeModel{loaded: true, cursor: 10}
	m, _ = m.Update(msgs.TabsLoadedMsg{Tabs: []library.TabRow{{ID: 1, Title: "Only"}}})
	if m.cursor > m.maxCursor() {
		t.Fatalf("cursor=%d should be clamped to max=%d", m.cursor, m.maxCursor())
	}
}

func TestHomeStatRowFitsAvailableWidth(t *testing.T) {
	cases := []struct {
		width int
	}{
		{40},
		{60},
	}
	for _, c := range cases {
		m := HomeModel{store: nil, width: c.width, height: 24, loaded: true}
		body := m.renderBody()

		found := false
		for _, line := range strings.Split(body, "\n") {
			if !strings.Contains(line, "Favorites") && !strings.ContainsAny(line, "│┌└") {
				continue
			}
			found = true
			if got := lipgloss.Width(line); got > c.width {
				t.Fatalf("width %d: stat line is %d cols wide, want ≤ %d: %q", c.width, got, c.width, line)
			}
		}
		if !found {
			t.Fatalf("width %d: no stat lines found in rendered body", c.width)
		}
	}
}
