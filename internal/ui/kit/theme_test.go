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
