package e2e_test

import (
	"strings"
	"testing"

	"fretboard/internal/parser"
	"fretboard/internal/ui/app"
	"fretboard/internal/ui/kit"
	"fretboard/tests/helpers"
)

func TestTUIViewerRender(t *testing.T) {
	tab, err := parser.Parse(strings.NewReader(helpers.SultansTab))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	rendered := kit.RenderTab(tab)
	if !strings.Contains(rendered, "Sultans of Swing") {
		t.Errorf("rendered tab should contain title, got:\n%s", rendered)
	}
	// After parsing, the B string fret 3 is present in the raw model.
	// The rendered output should contain the fret digits.
	if !strings.Contains(rendered, "3") {
		t.Errorf("rendered tab should contain fret 3, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "│ 1") {
		t.Errorf("rendered tab should contain bar markers, got:\n%s", rendered)
	}
}

func TestTUIStatusBar(t *testing.T) {
	tab, err := parser.Parse(strings.NewReader(helpers.SultansTab))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	info := kit.StatusInfo{
		Filename: "sultans.txt",
		Tuning:   tab.Tuning.Label(),
		BPM:      120,
		Playing:  false,
	}
	bar := kit.RenderStatusBar(80, info)
	if !strings.Contains(bar, "sultans.txt") {
		t.Errorf("status bar should contain filename, got: %s", bar)
	}
	if !strings.Contains(bar, "EADGBE") {
		t.Errorf("status bar should contain tuning label, got: %s", bar)
	}
}

func TestTUIAppModel(t *testing.T) {
	app := app.NewApp()
	view := app.View()
	if view == "" {
		t.Fatalf("initial view should not be empty")
	}
	if !strings.Contains(view, "fretboard") {
		t.Errorf("initial view should show app branding, got: %s", view)
	}
	if !strings.Contains(view, "home") {
		t.Errorf("initial view should start on home, got: %s", view)
	}
	if !strings.Contains(view, "quit") {
		t.Errorf("initial view should hint at quit key, got: %s", view)
	}
}

func TestTUIHomeLayout(t *testing.T) {
	view := app.NewApp().View()
	for _, want := range []string{"fretboard", "home", "[l]", "[o]", "[i]", "[q]"} {
		if !strings.Contains(view, want) {
			t.Errorf("home view should contain %q, got:\n%s", want, view)
		}
	}
}

func TestTUIChromeLayout(t *testing.T) {
	screen := kit.LayoutScreen(80, 24, kit.FormatBreadcrumb("home", "library"), "body", "")
	if !strings.Contains(screen, "fretboard") {
		t.Errorf("layout should include header branding, got: %s", screen)
	}
	if !strings.Contains(screen, "home › library") {
		t.Errorf("layout should include breadcrumb trail, got: %s", screen)
	}
}

func TestLayoutScreenDoesNotFillTallTerminal(t *testing.T) {
	screen := kit.LayoutScreen(80, 1568, kit.FormatBreadcrumb("home"), "\nHello library\n", "[q]quit")
	lines := strings.Split(strings.TrimRight(screen, "\n"), "\n")
	if len(lines) > 40 {
		t.Fatalf("tall terminal should not produce stripe fill (%d lines)", len(lines))
	}
	if !strings.Contains(screen, "Hello library") {
		t.Fatalf("layout should include body text, got:\n%s", screen)
	}
}

func TestTUIViewerMultiDigitFret(t *testing.T) {
	tab, err := parser.Parse(strings.NewReader(helpers.MultiDigitTab))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	rendered := kit.RenderTab(tab)
	if !strings.Contains(rendered, "12") {
		t.Errorf("rendered multi-digit fret should contain '12', got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "10") {
		t.Errorf("rendered multi-digit fret should contain '10', got:\n%s", rendered)
	}
}

func TestThemeCycle(t *testing.T) {
	t.Cleanup(func() {
		kit.SetTheme("default")
	})
	start := kit.CurrentTheme().Name
	names := kit.ThemeNames()
	if len(names) < 2 {
		t.Fatalf("expected at least 2 themes, got %v", names)
	}
	for i := 0; i < len(names); i++ {
		kit.SetTheme(names[i])
		if got := kit.CurrentTheme().Name; got != names[i] {
			t.Errorf("theme %d: got %q, want %q", i, got, names[i])
		}
	}
	kit.SetTheme(start)
}
