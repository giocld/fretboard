package player

import (
	"sort"
	"time"
)

// scoreAlignment scores one (bpm, offset) hypothesis. Matches are weighted
// by the detected onset's normalized strength (accented onsets count more),
// and bar-start expectations must land on above-median onsets — that is how
// the offset gets pinned when the recording subdivides the tab's grid. The
// opening onsets gate the hypothesis entirely; the hint (leading silence)
// acts as an offset prior. Returns (score, wide-confidence).
func scoreAlignment(expected []ExpectedOnset, onsets []Onset, strengths []float64, scale float64, offset time.Duration, bpm int, hint time.Duration) (float64, float64) {
	tolerance := time.Duration(60000/bpm/4) * time.Millisecond // one quarter beat
	if tolerance > 150*time.Millisecond {
		tolerance = 150 * time.Millisecond
	}
	if tolerance < 60*time.Millisecond {
		tolerance = 60 * time.Millisecond
	}
	// Median onset strength: bar starts must land on the accent grid.
	med := medianStrength(strengths)
	idx := 0
	matches := 0
	var score float64
	var residSum float64
	total := 0
	opening := 0
	for i, want := range expected {
		t := time.Duration(float64(want.Time) * scale)
		t += offset
		if t < 0 {
			continue
		}
		total++
		for idx < len(onsets) && onsets[idx].Time < t-tolerance {
			idx++
		}
		best := -1
		for j := idx; j < len(onsets) && onsets[j].Time <= t+tolerance; j++ {
			if best < 0 || absDur(onsets[j].Time-t) < absDur(onsets[best].Time-t) {
				best = j
			}
		}
		matched := best >= 0
		if matched && want.BarStart && strengths[best] < med {
			matched = false // a downbeat on a weak onset is a misalignment
		}
		if matched {
			matches++
			residSum += float64(absDur(onsets[best].Time-t)) / float64(time.Millisecond)
			score += strengths[best]
			if i < 10 {
				opening++
			}
		} else {
			score -= 1.0
			if i < 10 {
				return -1e9, 0 // a wrong tempo cannot fake the opening
			}
		}
	}
	if opening < 5 || total == 0 {
		return -1e9, 0 // require the first beats to line up
	}
	// Residual penalty: sloppy aliased alignments lose to tight ones.
	if matches > 0 {
		score -= 0.02 * residSum / float64(matches)
	}
	// Offset prior: the leading-silence estimate anchors the search.
	if hint > 0 {
		d := absDur(offset - hint)
		if d < 2*time.Second {
			score += 8 * (1 - float64(d)/float64(2*time.Second))
		}
	}
	return score, float64(matches) / float64(total)
}

func medianStrength(strengths []float64) float64 {
	if len(strengths) == 0 {
		return 0
	}
	sorted := append([]float64(nil), strengths...)
	sort.Float64s(sorted)
	return sorted[len(sorted)/2]
}

// NearestOnset returns the detected onset closest to t, or false when there
// is none within maxGap.
func NearestOnset(onsets []time.Duration, t time.Duration, maxGap time.Duration) (time.Duration, bool) {
	i := sort.Search(len(onsets), func(i int) bool { return onsets[i] >= t })
	best, found := time.Duration(0), false
	if i < len(onsets) {
		best, found = onsets[i], true
	}
	if i > 0 {
		d := t - onsets[i-1]
		if !found || d < best-t {
			best, found = onsets[i-1], true
		}
	}
	if found && absDur(best-t) <= maxGap {
		return best, true
	}
	return 0, false
}

func absDur(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}
