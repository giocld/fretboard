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
// strength and the prior break the tie. The offset search is seeded from
// the recording's own early onsets (each "onset i is the tab's first note"
// is a hypothesis), and the tempo search runs a second window around the
// tempo the audio itself implies, so a recording slower than the tab's BPM
// still aligns.

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

// The tempo search windows are bounded by bpmWindowMin/bpmWindowMax. The
// lower bound is 30, not 60: a slow tab (say 45 BPM) whose ±25% window is
// [34,56] would otherwise clamp to [60,56] and invert into an empty window.
// The scoring gates (the opening notes must line up, bar starts must land
// on accented onsets) reject the wrong slow tempos such a wide window
// admits.
const (
	bpmWindowMin = 30
	bpmWindowMax = 220
)

// offsetTieEps is the score band within which two offsets are considered
// equivalent: congruent alternatives (whole beats apart) differ only by
// onset jitter, so the earliest is preferred (see the near-tie pass in
// AlignAudio).
const offsetTieEps = 0.5

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
	// The detector emits the strong-pass onsets first and appends the
	// weak-pass onsets after, so the combined list can be out of time
	// order. The scorers walk it monotonically and the tempo window reads
	// its intervals, so sort it into a proper grid.
	sort.Slice(onsets, func(i, j int) bool { return onsets[i].Time < onsets[j].Time })
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

	// The onset times do not depend on the candidate BPM, so collect them
	// once: they feed both the audio-derived tempo window and the result.
	times := make([]time.Duration, len(onsets))
	for i, o := range onsets {
		times[i] = o.Time
	}

	// Primary window: the tab's tempo plus/minus 25%, clamped to the shared
	// bounds. When the clamp inverts it (tab slower than bpmWindowMin*4/3),
	// the window is empty and the audio-derived window carries the search.
	lo := max(baseBPM-baseBPM/4, bpmWindowMin)
	hi := min(baseBPM+baseBPM/4, bpmWindowMax)
	if lo > hi {
		lo, hi = 1, 0
	}
	// Second window: the tempo the recording itself implies, from the
	// median inter-onset interval. This rescues recordings whose tempo
	// differs from the tab by more than the primary window tolerates —
	// a 45 BPM tab over 33 BPM audio inverts the primary window entirely.
	// Skipped when no stable interval can be measured.
	dlo, dhi := 1, 0
	if interval := medianIntervals(times); interval > 0 {
		detected := int(60000 / interval.Milliseconds())
		dlo = max(detected-detected/4, bpmWindowMin)
		dhi = min(detected+detected/4, bpmWindowMax)
		if dlo > dhi {
			dlo, dhi = 1, 0
		}
	}

	// Candidate offsets are seeded from the recording's own early onsets
	// instead of a blind 0..15 s scan: the true offset is always within one
	// 250 ms neighborhood of some early onset when the recording's opening
	// is the tab's opening, and the search is unbounded for longer intros.
	offsets := onsetSeedOffsets(onsets)
	best := zero
	for bpm := max(min(lo, dlo), bpmWindowMin); bpm <= max(hi, dhi); bpm++ {
		inPrimary := lo <= hi && bpm >= lo && bpm <= hi
		inDetected := dlo <= dhi && bpm >= dlo && bpm <= dhi
		if !inPrimary && !inDetected {
			continue
		}
		scale := float64(baseBPM) / float64(bpm)
		for _, off := range offsets {
			score, matched := scoreAlignment(expected, onsets, strengths, scale, off, bpm, hint)
			if score > best.Score {
				best = Alignment{BPM: bpm, Offset: off,
					Score: score, Confidence: matched, Detected: len(onsets)}
			}
		}
	}
	if best.BPM > 0 {
		best.Onsets = times
	}
	// Congruent offsets — whole beats apart — score identically up to onset
	// jitter and floating-point noise, and any offset more than a beat below
	// the first onset fails the opening gate. So among the near-ties, the
	// earliest is the true one: the recording's first note is the tab's
	// first note. Prefer it explicitly instead of whichever noise won.
	if best.BPM > 0 {
		scale := float64(baseBPM) / float64(best.BPM)
		for _, off := range offsets {
			if off >= best.Offset {
				continue
			}
			score, matched := scoreAlignment(expected, onsets, strengths, scale, off, best.BPM, hint)
			if score > best.Score-offsetTieEps {
				best.Offset = off
				best.Score = score
				best.Confidence = matched
			}
		}
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

// onsetSeedOffsets proposes candidate offsets from the detected onsets. For
// each of the first few onsets, hypothesize that onset is the tab's first
// note — expected[0].Time is always zero (the tab's first note is the time
// reference), so that offset is just the onset time under every candidate
// BPM — and scan a 250 ms neighborhood at 25 ms. This replaces a blind
// 0..15 s scan at 100 ms: the offset is found whenever the recording's
// opening matches the tab's opening (or is preceded only by silence), and
// the finer grid needs no separate coarse pass.
func onsetSeedOffsets(onsets []Onset) []time.Duration {
	n := min(20, len(onsets))
	offsets := make([]time.Duration, 0, 21*n)
	seen := make(map[time.Duration]bool, 21*n)
	for i := 0; i < n; i++ {
		base := onsets[i].Time
		for off := -250 * time.Millisecond; off <= 250*time.Millisecond; off += 25 * time.Millisecond {
			o := base + off
			if o < 0 || seen[o] {
				continue
			}
			seen[o] = true
			offsets = append(offsets, o)
		}
	}
	return offsets
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
