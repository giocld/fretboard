package player

import (
	"encoding/json"
	"slices"
	"sort"
	"time"
)

// Automatic tempo map: the recording's own onset grid, once aligned, yields
// measured bar positions. Emitting those as anchors every few bars gives the
// existing anchor mapper a dense, tempo-following reference — rubato and
// tempo drift stop accumulating because the anchors were measured, not
// extrapolated.

// TempoAnchors derives (bar, audio-time) anchors from the aligned onset
// match. The first anchor is found by direct matching; every later anchor is
// PREDICTED from the previous one using the recording's own local median
// beat interval, then confirmed against a nearby onset. Anchors therefore
// follow the recording's tempo changes instead of locking the tab's grid
// onto sloppy matches. Bars are 1-based to match the viewer's sync points.
func TempoAnchors(expected []ExpectedOnset, onsets []time.Duration, scale float64, offset time.Duration, bpm int, anchorEvery int) []SyncPoint {
	if anchorEvery < 1 {
		anchorEvery = 4
	}
	tolerance := time.Duration(60000/bpm/4) * time.Millisecond
	if tolerance > 150*time.Millisecond {
		tolerance = 150 * time.Millisecond
	}
	if tolerance < 60*time.Millisecond {
		tolerance = 60 * time.Millisecond
	}
	type barStart struct {
		bar int
		t   time.Duration
	}
	var starts []barStart
	for _, w := range expected {
		if w.BarStart {
			starts = append(starts, barStart{w.Bar, time.Duration(float64(w.Time)*scale) + offset})
		}
	}
	if len(starts) == 0 || len(onsets) < 4 {
		return nil
	}
	// Global interval: the median of the OPENING inter-onset intervals (the
	// dominant tempo, before any mid-song changes).
	globalInterval := medianIntervals(onsets)
	if len(onsets) > 13 {
		if g := medianIntervals(onsets[:13]); g > 0 {
			globalInterval = g
		}
	}
	if globalInterval <= 0 {
		return nil
	}
	// Tab bar duration at the aligned BPM, straight from the expected grid:
	// the time between the first two tab bar starts. This is exact for any
	// tab (the schedule's tick content is already baked into it).
	tabBarSecs := 1.0
	if len(starts) >= 2 {
		if d := starts[1].t - starts[0].t; d > 0 {
			tabBarSecs = d.Seconds()
		}
	}

	localInterval := func(after time.Duration) time.Duration {
		window := after + 8*globalInterval
		var gaps []int64
		prev := time.Duration(0)
		first := true
		for _, o := range onsets {
			if o < after || o > window {
				continue
			}
			if !first {
				gaps = append(gaps, int64(o-prev))
			}
			prev = o
			first = false
		}
		if len(gaps) < 3 {
			return globalInterval
		}
		return medianIntervalsFrom(gaps)
	}

	var anchors []SyncPoint
	// First anchor: the first tab bar start that matches a detected onset.
	var lastBar int
	var lastTime time.Duration
	haveLast := false
	for _, s := range starts {
		if n, ok := NearestOnset(onsets, s.t, tolerance); ok {
			anchors = append(anchors, SyncPoint{Bar: s.bar + 1, Seconds: n.Seconds()})
			lastBar, lastTime = s.bar, n
			haveLast = true
			break
		}
	}
	if !haveLast {
		return nil
	}
	for _, s := range starts {
		if s.bar-lastBar < anchorEvery {
			continue
		}
		// Predict from the local measured tempo: the local beat interval
		// relative to the global one scales the tab's bar duration, so the
		// map follows tempo changes regardless of whether the detector sees
		// eighths or quarters.
		interval := localInterval(lastTime)
		ratio := float64(interval) / float64(globalInterval)
		predicted := lastTime + time.Duration(float64(s.bar-lastBar)*tabBarSecs*ratio*float64(time.Second))
		if n, ok := NearestOnset(onsets, predicted, tolerance); ok {
			anchors = append(anchors, SyncPoint{Bar: s.bar + 1, Seconds: n.Seconds()})
			lastBar, lastTime = s.bar, n
		} else {
			// No confirmation: keep the prediction as a soft anchor anyway so
			// the map stays continuous (the drift meter corrects residuals).
			anchors = append(anchors, SyncPoint{Bar: s.bar + 1, Seconds: predicted.Seconds()})
			lastBar, lastTime = s.bar, predicted
		}
	}
	return anchors
}

// medianIntervalsFrom returns the median of pre-collected gaps.
func medianIntervalsFrom(gaps []int64) time.Duration {
	sorted := append([]int64(nil), gaps...)
	slices.Sort(sorted)
	return time.Duration(sorted[len(sorted)/2])
}

// MergeAnchors merges auto anchors with user anchors: a user anchor at a bar
// wins over the auto anchor for the same bar. The result is sorted by time.
func MergeAnchors(user, auto []SyncPoint) []SyncPoint {
	userBars := map[int]bool{}
	for _, p := range user {
		userBars[p.Bar] = true
	}
	out := append([]SyncPoint(nil), user...)
	for _, a := range auto {
		if userBars[a.Bar] {
			continue
		}
		out = append(out, a)
	}
	// Stable sort by bar (the mapper requires ascending anchors).
	sort.SliceStable(out, func(i, j int) bool { return out[i].Bar < out[j].Bar })
	return out
}

// CorrectStepSnap snaps an audio-time mapping to the nearest detected onset
// when the mapping has drifted beyond tolerance: returns the step index at
// the onset time (and true) when a trustworthy onset exists nearby.
func CorrectStepSnap(schedule []PlaybackStep, points []SyncPoint, elapsed time.Duration, onsets []time.Duration, bpm int) (int, bool) {
	if len(onsets) == 0 || len(schedule) == 0 {
		return 0, false
	}
	// Only correct when the elapsed time maps near a detected onset: a
	// silence gap has no onsets and must not cause a jump.
	if n, ok := NearestOnset(onsets, elapsed, 250*time.Millisecond); ok {
		if absDur(elapsed-n) >= 150*time.Millisecond {
			idx := StepIndexAtSyncPoints(schedule, points, n.Seconds(), bpm)
			if idx >= 0 && idx < len(schedule) {
				return idx, true
			}
		}
	}
	return 0, false
}

// TempoMap persists the auto tempo map for a source: the measured anchors
// and the detected onsets, so a later session can restore the map without
// re-running the analysis.
type TempoMap struct {
	Anchors []SyncPoint `json:"anchors"`
	Onsets  []float64   `json:"onsets"` // seconds
}

// MarshalTempoMap serializes the map for tab metadata.
func MarshalTempoMap(anchors []SyncPoint, onsets []time.Duration) string {
	tm := TempoMap{Anchors: anchors, Onsets: make([]float64, len(onsets))}
	for i, o := range onsets {
		tm.Onsets[i] = o.Seconds()
	}
	// json.Marshal cannot fail here: the map holds only numbers and slices.
	data, _ := json.Marshal(tm)
	return string(data)
}

// UnmarshalTempoMap restores the map from tab metadata.
func UnmarshalTempoMap(raw string) ([]SyncPoint, []time.Duration) {
	var tm TempoMap
	if err := json.Unmarshal([]byte(raw), &tm); err != nil {
		return nil, nil
	}
	onsets := make([]time.Duration, len(tm.Onsets))
	for i, s := range tm.Onsets {
		onsets[i] = time.Duration(s * float64(time.Second))
	}
	return tm.Anchors, onsets
}
