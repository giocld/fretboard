package player

import (
	"sort"
	"time"

	"fretboard/internal/model"
)

// Automatic audio alignment: match the schedule's expected note onsets to
// the recording's detected onsets over a BPM x offset grid. This replaces
// the fragile "leading silence + single global BPM" guesswork with a
// data-driven estimate that survives count-ins, non-silent intros, and
// tempo differences between the tab and the recording.
//
// Two cues pin the alignment: onset strength (accented downbeats line up
// with the tab's bar starts) and the leading-silence estimate as an offset
// prior. A pure onset-time match is ambiguous when the recording has more
// onsets than the tab (a 2:1 or 3:1 subdivision aliases at many offsets);
// strength and the prior break the tie.

// Alignment is the result of aligning a recording to a tab.
type Alignment struct {
	BPM        int             // detected tempo
	Offset     time.Duration   // where the tab's first note lands in the audio
	Score      float64         // raw match score (higher is better)
	Confidence float64         // 0..1 — fraction of expected onsets matched (strict)
	Detected   int             // how many onsets the analysis found
	Onsets     []time.Duration // the detected onset times (for the drift meter)
}

// ExpectedOnset is one expected note time with its bar-start flag and bar.
type ExpectedOnset struct {
	Time     time.Duration
	BarStart bool // the first note of a bar (downbeat in the tab)
	Bar      int  // 0-based bar index of the step
}

// alignWindow bounds how much of the song's opening we use for scoring:
// enough to be robust, short enough to be fast and to weight the part the
// user hears first.
const alignWindow = 90 * time.Second

// ExpectedOnsets returns the wall-clock times of the schedule's first notes
// at the given BPM, from the tab start, flagging bar starts.
func ExpectedOnsets(tab *model.Tab, bpm int) []ExpectedOnset {
	if tab == nil || bpm <= 0 {
		return nil
	}
	var out []ExpectedOnset
	var acc float64
	limit := float64(alignWindow) / float64(time.Second)
	prevBar := -1
	for _, s := range BuildSchedule(tab) {
		secs := float64(s.Ticks) * 60.0 / float64(bpm) / float64(ticksPerQuarter)
		if acc >= limit {
			break
		}
		out = append(out, ExpectedOnset{
			Time:     time.Duration(acc * float64(time.Second)),
			BarStart: s.Bar != prevBar,
			Bar:      s.Bar,
		})
		prevBar = s.Bar
		acc += secs
	}
	return out
}

// AlignAudio finds the tempo and tab-start offset that best match the
// recording's detected onsets. hint is the leading-silence estimate used as
// an offset prior (0 = none). Returns a zero-confidence alignment when the
// analysis can't produce anything usable (no ffmpeg, too few onsets).
func AlignAudio(tab *model.Tab, path string, hint time.Duration) Alignment {
	var zero Alignment
	onsets, err := DetectOnsetsWithStrength(path)
	if err != nil || len(onsets) < 10 {
		return zero
	}
	baseBPM := TabBPM(tab)
	if baseBPM <= 0 {
		baseBPM = 120
	}
	expected := ExpectedOnsets(tab, baseBPM)
	if len(expected) < 20 {
		return zero // not enough musical content in the tab
	}

	// Normalize onset strengths so weighting is comparable.
	maxStr := 0.0
	for _, o := range onsets {
		maxStr = max(maxStr, o.Strength)
	}
	if maxStr <= 0 {
		return zero
	}
	strengths := make([]float64, len(onsets))
	for i, o := range onsets {
		strengths[i] = o.Strength / maxStr
	}

	lo := max(baseBPM-baseBPM/4, 60)
	hi := min(baseBPM+baseBPM/4, 220)
	best := zero
	for bpm := lo; bpm <= hi; bpm++ {
		scale := float64(baseBPM) / float64(bpm)
		for offsetMs := 0; offsetMs <= 15000; offsetMs += 100 {
			score, matched := scoreAlignment(expected, onsets, strengths, scale, time.Duration(offsetMs)*time.Millisecond, bpm, hint)
			if score > best.Score {
				best = Alignment{BPM: bpm, Offset: time.Duration(offsetMs) * time.Millisecond,
					Score: score, Confidence: matched, Detected: len(onsets)}
			}
		}
	}
	// The onset times do not depend on the candidate, so fill them once.
	best.Onsets = make([]time.Duration, len(onsets))
	for i, o := range onsets {
		best.Onsets[i] = o.Time
	}
	// Refine the offset around the winner at 25 ms resolution.
	if best.BPM > 0 {
		scale := float64(baseBPM) / float64(best.BPM)
		base := best.Offset
		for off := -75 * time.Millisecond; off <= 75*time.Millisecond; off += 25 * time.Millisecond {
			o := base + off
			if o < 0 {
				continue
			}
			score, matched := scoreAlignment(expected, onsets, strengths, scale, o, best.BPM, hint)
			if score > best.Score {
				best.Offset = o
				best.Score = score
				best.Confidence = matched
			}
		}
	}
	// Strict verification: the final confidence comes from a 60 ms tolerance
	// re-check, which collapses harmonic aliases the wide pass tolerates.
	if best.BPM > 0 {
		best.Confidence = verifyStrict(expected, onsets, float64(baseBPM)/float64(best.BPM), best.Offset)
	}
	return best
}

// verifyStrict re-scores a hypothesis with a tight 60 ms tolerance: the true
// alignment keeps most matches, harmonic aliases collapse.
func verifyStrict(expected []ExpectedOnset, onsets []Onset, scale float64, offset time.Duration) float64 {
	const tol = 60 * time.Millisecond
	idx := 0
	matches, total := 0, 0
	for _, want := range expected {
		t := time.Duration(float64(want.Time)*scale) + offset
		if t < 0 {
			continue
		}
		total++
		for idx < len(onsets) && onsets[idx].Time < t-tol {
			idx++
		}
		if idx < len(onsets) && onsets[idx].Time <= t+tol {
			matches++
		}
	}
	if total == 0 {
		return 0
	}
	return float64(matches) / float64(total)
}

// medianIntervals returns the median inter-onset interval of the detected
// onsets (the beat period), or 0 when there are too few.
func medianIntervals(onsets []time.Duration) time.Duration {
	if len(onsets) < 6 {
		return 0
	}
	gaps := make([]int64, 0, len(onsets)-1)
	for i := 1; i < len(onsets); i++ {
		gaps = append(gaps, int64(onsets[i]-onsets[i-1]))
	}
	sort.Slice(gaps, func(i, j int) bool { return gaps[i] < gaps[j] })
	return time.Duration(gaps[len(gaps)/2])
}
