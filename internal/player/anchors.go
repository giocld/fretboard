package player

import (
	"fmt"
	"sort"
)

// beatsPerBar is the 4/4-time assumption the bar-based sanity check uses to
// convert a bar span into beats: sync anchors carry no time signature, and a
// 4/4 tab is the overwhelmingly common case. The tick-based SegmentBPM
// remains the precise tool when a schedule is available.
const beatsPerBar = 4

// sanityBPMFloor and sanityBPMCeiling bound the tempo an anchor segment may
// imply before the anchors are judged unusable rather than merely suspicious:
// below floor or above ceiling the timing is clearly wrong, not just drifted.
const (
	sanityBPMFloor   = 40
	sanityBPMCeiling = 300
)

// sanityDeviation is the relative tempo deviation from the tab BPM that
// turns a suspicious anchor into a flagged one (20%).
const sanityDeviation = 0.20

// CheckAnchorSanity verifies that sync anchors imply a playback tempo close
// to the tab BPM. The implied tempo between two neighboring anchors is
// beatsPerBar * 60 * deltaBars / deltaSeconds (see impliedTempoBPM), and each
// anchor is measured against every neighbor it has — endpoints against their
// single neighbor, interior anchors against both, using the worst deviation.
// An anchor whose implied tempo deviates more than 20% from bpm produces a
// warning; one implying less than sanityBPMFloor or more than
// sanityBPMCeiling is rejected outright (ok=false) — the warning names it all
// the same. A lone anchor has no neighbor to measure against and never warns;
// duplicate bar or time pairs carry no tempo information and are skipped.
// Anchors are compared in seconds order (the order the time mapper consumes
// them in), so the check is order-independent; warnings name the anchor's
// 1-based position in the input slice.
func CheckAnchorSanity(anchors []SyncPoint, bpm int) (warnings []string, ok bool) {
	ok = true
	if bpm <= 0 {
		bpm = DefaultBPM
	}
	if len(anchors) < 2 {
		return nil, true
	}
	// Pair each anchor with its original index, then sort by seconds (stable,
	// bar order wins ties) exactly as StepIndexAtSyncPoints consumes anchors.
	entries := make([]anchorEntry, len(anchors))
	for i, pt := range anchors {
		entries[i] = anchorEntry{pt: pt, idx: i}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].pt.Seconds < entries[j].pt.Seconds
	})
	for i, e := range entries {
		worst, found := 0, false
		for _, j := range [2]int{i - 1, i + 1} {
			if j < 0 || j >= len(entries) {
				continue
			}
			implied, ok := impliedTempoBPM(entries[i].pt, entries[j].pt)
			if !ok {
				continue
			}
			if !found || tempoDeviation(implied, bpm) > tempoDeviation(worst, bpm) {
				worst, found = implied, true
			}
		}
		if !found {
			continue
		}
		if worst < sanityBPMFloor || worst > sanityBPMCeiling {
			ok = false
		}
		if tempoDeviation(worst, bpm) > sanityDeviation {
			warnings = append(warnings, fmt.Sprintf(
				"anchor %d implies ~%d bpm vs tab %d — verify", e.idx+1, worst, bpm))
		}
	}
	return warnings, ok
}

// anchorEntry pairs an anchor with its position in the caller's slice so
// warnings can name the input index after internal reordering.
type anchorEntry struct {
	pt  SyncPoint
	idx int
}

// impliedTempoBPM derives the tempo two anchors imply: the bar span between
// them converted to quarter notes (beatsPerBar per bar) divided by their
// wall-clock span. ok=false when either span is non-positive (duplicate
// anchor bars or identical times), which carries no tempo information.
func impliedTempoBPM(a, b SyncPoint) (int, bool) {
	deltaBars := a.Bar - b.Bar
	if deltaBars < 0 {
		deltaBars = -deltaBars
	}
	deltaSeconds := a.Seconds - b.Seconds
	if deltaSeconds < 0 {
		deltaSeconds = -deltaSeconds
	}
	if deltaBars <= 0 || deltaSeconds <= 0 {
		return 0, false
	}
	implied := float64(deltaBars) * beatsPerBar * 60.0 / deltaSeconds
	return int(implied + 0.5), true
}

// tempoDeviation is the relative difference of implied from bpm; callers pass
// a bpm already normalized to a positive value.
func tempoDeviation(implied, bpm int) float64 {
	d := implied - bpm
	if d < 0 {
		d = -d
	}
	return float64(d) / float64(bpm)
}
