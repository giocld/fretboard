package kit

import (
	"reflect"
	"testing"
)

func TestThemeNamesDeterministic(t *testing.T) {
	first := ThemeNames()
	if len(first) == 0 {
		t.Fatal("expected at least one theme")
	}
	// The order must be stable across calls (map iteration is random).
	for i := 0; i < 20; i++ {
		if got := ThemeNames(); !reflect.DeepEqual(got, first) {
			t.Fatalf("ThemeNames() order not deterministic: %v vs %v", got, first)
		}
	}
	// Cycling through the order should visit every theme exactly once.
	seen := map[string]bool{}
	for _, name := range first {
		if seen[name] {
			t.Fatalf("duplicate theme name %q", name)
		}
		seen[name] = true
	}
	for name := range Themes {
		if !seen[name] {
			t.Fatalf("theme %q missing from ThemeNames()", name)
		}
	}
}

// TestStringColorIsThemeTintedNotRainbow guards the staff-color fix: every
// string index gets the same color (the theme's tint, not a per-string
// rainbow), and it changes with the active theme.
func TestStringColorIsThemeTintedNotRainbow(t *testing.T) {
	SetTheme("default")
	defaultC := StringColor(0)
	for i := 1; i < 8; i++ {
		if got := StringColor(i); got != defaultC {
			t.Fatalf("string %d color = %v, want the same tint as string 0 (%v)", i, got, defaultC)
		}
	}
	SetTheme("dracula")
	if got := StringColor(0); got == defaultC {
		t.Fatalf("dracula staff color should differ from default, both %v", got)
	}
	SetTheme("default")
}
