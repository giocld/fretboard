package player

import (
	"strings"
	"testing"
)

func TestCheckAnchorSanityGood(t *testing.T) {
	// 120 bpm, 4/4: two bars span exactly 4 seconds.
	anchors := []SyncPoint{{Bar: 0, Seconds: 0}, {Bar: 2, Seconds: 4}, {Bar: 4, Seconds: 8}}
	warnings, ok := CheckAnchorSanity(anchors, 120)
	if !ok {
		t.Fatalf("exact-tempo anchors rejected: %v", warnings)
	}
	if len(warnings) != 0 {
		t.Fatalf("exact-tempo anchors produced warnings: %v", warnings)
	}
}

func TestCheckAnchorSanityDeviationWarns(t *testing.T) {
	// One bar over 1.6s implies 150 bpm vs tab 120: 25% off — flagged, but
	// within [40,300] so the anchors stay usable.
	anchors := []SyncPoint{{Bar: 0, Seconds: 0}, {Bar: 1, Seconds: 1.6}}
	warnings, ok := CheckAnchorSanity(anchors, 120)
	if !ok {
		t.Fatalf("25%% deviation should warn but stay usable: %v", warnings)
	}
	if len(warnings) != 2 {
		t.Fatalf("want 2 warnings (both anchors), got %v", warnings)
	}
	for _, w := range warnings {
		if !strings.Contains(w, "implies ~150 bpm vs tab 120") {
			t.Fatalf("warning %q missing implied tempo", w)
		}
	}
}

func TestCheckAnchorSanityRejectsOutliers(t *testing.T) {
	fast := []SyncPoint{{Bar: 0, Seconds: 0}, {Bar: 1, Seconds: 0.6}} // 400 bpm
	slow := []SyncPoint{{Bar: 0, Seconds: 0}, {Bar: 1, Seconds: 10}}  // 24 bpm
	for _, tc := range []struct {
		name    string
		anchors []SyncPoint
	}{
		{"over ceiling", fast},
		{"under floor", slow},
	} {
		t.Run(tc.name, func(t *testing.T) {
			warnings, ok := CheckAnchorSanity(tc.anchors, 120)
			if ok {
				t.Fatalf("out-of-range segment must be rejected, warnings %v", warnings)
			}
			if len(warnings) == 0 {
				t.Fatal("rejected anchors must be named in a warning")
			}
			for _, w := range warnings {
				if !strings.Contains(w, "— verify") {
					t.Fatalf("rejection warning %q must carry the verify hint", w)
				}
			}
		})
	}
}

func TestCheckAnchorSanityWorstNeighbor(t *testing.T) {
	// Segments imply 240 bpm (bar 0->1 in 1s) and 80 bpm (bar 1->2 in 3s):
	// the middle anchor is measured against both neighbors and reported at
	// the worst deviation (240 bpm).
	anchors := []SyncPoint{{Bar: 0, Seconds: 0}, {Bar: 1, Seconds: 1}, {Bar: 2, Seconds: 4}}
	warnings, ok := CheckAnchorSanity(anchors, 120)
	if !ok {
		t.Fatalf("240/80 bpm segments warn but stay usable: %v", warnings)
	}
	if len(warnings) != 3 {
		t.Fatalf("all three anchors should warn, got %v", warnings)
	}
	if !strings.Contains(warnings[1], "~240 bpm") {
		t.Fatalf("middle anchor warning %q should report worst-case 240 bpm", warnings[1])
	}
}

func TestCheckAnchorSanityEdges(t *testing.T) {
	if warnings, ok := CheckAnchorSanity(nil, 120); !ok || len(warnings) != 0 {
		t.Fatalf("empty anchors: ok=%v warnings=%v", ok, warnings)
	}
	if warnings, ok := CheckAnchorSanity([]SyncPoint{{Bar: 3, Seconds: 7}}, 120); !ok || len(warnings) != 0 {
		t.Fatalf("single anchor must not warn: ok=%v warnings=%v", ok, warnings)
	}
	// Duplicate bars or identical times carry no tempo info and are skipped.
	dupBar := []SyncPoint{{Bar: 0, Seconds: 0}, {Bar: 0, Seconds: 5}}
	if warnings, ok := CheckAnchorSanity(dupBar, 120); !ok || len(warnings) != 0 {
		t.Fatalf("duplicate-bar anchors: ok=%v warnings=%v", ok, warnings)
	}
	// bpm <= 0 falls back to DefaultBPM (120), so exact-tempo anchors stay clean.
	noBPM := []SyncPoint{{Bar: 0, Seconds: 0}, {Bar: 2, Seconds: 4}}
	if warnings, ok := CheckAnchorSanity(noBPM, 0); !ok || len(warnings) != 0 {
		t.Fatalf("bpm=0 fallback: ok=%v warnings=%v", ok, warnings)
	}
}

func TestCheckAnchorSanityOrderIndependentNamesOriginalIndex(t *testing.T) {
	// The segment between bar 0 at 10s and bar 1 at 0s implies 24 bpm.
	// Input is scrambled (bar 1 first); the check sorts by seconds internally
	// but names the anchors by their caller-visible positions.
	anchors := []SyncPoint{{Bar: 0, Seconds: 10}, {Bar: 1, Seconds: 0}}
	warnings, ok := CheckAnchorSanity(anchors, 120)
	if ok {
		t.Fatal("24 bpm segment must be rejected regardless of input order")
	}
	joined := strings.Join(warnings, "|")
	for _, want := range []string{"anchor 1 implies ~24 bpm", "anchor 2 implies ~24 bpm"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("warnings %q missing %q", joined, want)
		}
	}
}
