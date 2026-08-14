package player

import (
	"encoding/binary"
	"math"
	"time"
)

// Onset analysis pipeline: decode the recording to a low-rate energy
// envelope with ffmpeg, derive onset strength, and pick note-onset times.
// This is the raw material for automatic audio alignment (S2), the tempo
// map (S3), and the live drift meter (S3).

const (
	pcmSampleRate    = 8000             // Hz — plenty for onsets
	pcmFrameMs       = 10               // energy frame hop
	pcmWindowMs      = 40               // energy window
	maxPCMDuration   = 12 * time.Minute // bound the decode
	pcmDecodeTimeout = 60 * time.Second
	minOnsetGap      = 100 * time.Millisecond
)

// envelopeFromPCM computes the RMS energy envelope of little-endian s16 PCM.
func envelopeFromPCM(pcm []byte, rate, frameMs, windowMs int) []float64 {
	frameSamples := rate * frameMs / 1000
	windowSamples := rate * windowMs / 1000
	if frameSamples < 1 || windowSamples < 1 {
		return nil
	}
	samples := make([]float64, 0, len(pcm)/2)
	for i := 0; i+1 < len(pcm); i += 2 {
		s := int16(binary.LittleEndian.Uint16(pcm[i : i+2]))
		samples = append(samples, float64(s)/32768.0)
	}
	nFrames := len(samples) / frameSamples
	env := make([]float64, nFrames)
	for f := 0; f < nFrames; f++ {
		start := f * frameSamples
		end := min(start+windowSamples, len(samples))
		var sum float64
		for i := start; i < end; i++ {
			sum += samples[i] * samples[i]
		}
		env[f] = math.Sqrt(sum / float64(end-start))
	}
	return env
}

// onsetStrength converts the energy envelope into a half-wave-rectified,
// smoothed positive-delta signal in linear energy: rises in energy mark
// onsets, and the delta magnitude preserves amplitude contrast (an accented
// downbeat is ~7x a weak offbeat), which the log domain flattens.
func onsetStrength(env []float64) []float64 {
	if len(env) == 0 {
		return nil
	}
	energy := make([]float64, len(env))
	for i, e := range env {
		energy[i] = e * e
	}
	delta := make([]float64, len(env))
	for i := 1; i < len(env); i++ {
		d := energy[i] - energy[i-1]
		if d > 0 {
			delta[i] = d
		}
	}
	sm := make([]float64, len(delta))
	for i := range delta {
		sum, n := 0.0, 0.0
		for j := i - 1; j <= i+1; j++ {
			if j >= 0 && j < len(delta) {
				sum += delta[j]
				n++
			}
		}
		sm[i] = sum / n
	}
	return sm
}

// Onset is a detected note onset with its strength (the peak energy-delta
// value at the onset). Strength distinguishes accented downbeats from weak
// offbeats — the cue automatic alignment uses to pin the offset.
type Onset struct {
	Time     time.Duration
	Strength float64
}

// pickOnsets finds onset times from the strength signal: local maxima above
// an adaptive threshold (local mean + k*std), separated by at least
// minOnsetGap, refined to the nearest local peak.
func pickOnsets(strength []float64, frameMs int, gap time.Duration, k float64) []Onset {
	if len(strength) == 0 {
		return nil
	}
	minGapFrames := int(gap.Milliseconds()) / frameMs
	var onsets []Onset
	last := -minGapFrames
	// Floor relative to the global peak: weak-but-real onsets (unaccented
	// offbeats) survive while noise-floor fluctuations stay filtered.
	globalMax := 0.0
	for _, s := range strength {
		globalMax = max(globalMax, s)
	}
	floor := globalMax * 0.04
	// Local threshold from a sliding window (2 s each side).
	win := 2000 / frameMs
	for i := 1; i < len(strength)-1; i++ {
		if strength[i] <= strength[i-1] || strength[i] <= strength[i+1] {
			continue
		}
		lo := max(i-win, 0)
		hi := min(i+win, len(strength))
		mean, m2 := 0.0, 0.0
		for j := lo; j < hi; j++ {
			mean += strength[j]
		}
		mean /= float64(hi - lo)
		for j := lo; j < hi; j++ {
			d := strength[j] - mean
			m2 += d * d
		}
		std := math.Sqrt(m2 / float64(hi-lo))
		thr := max(mean+k*std, floor)
		if strength[i] < thr {
			continue
		}
		if i-last < minGapFrames {
			continue
		}
		best := sharpestPeak(strength, i)
		onsets = append(onsets, Onset{
			Time:     time.Duration(best*frameMs) * time.Millisecond,
			Strength: strength[best],
		})
		last = best
		i = best
	}
	// Weak pass: local maxima above the global-peak floor only — catches
	// unaccented onsets the adaptive term suppresses in loud windows, so
	// strength-graded alignment sees the full grid. Merged with the strong
	// pass, keeping the max strength per onset.
	if floor > 0 {
		weakLast := -minGapFrames
		for i := 1; i < len(strength)-1; i++ {
			if strength[i] <= strength[i-1] || strength[i] <= strength[i+1] {
				continue
			}
			if strength[i] < floor {
				continue
			}
			if i-weakLast < minGapFrames {
				continue
			}
			best := sharpestPeak(strength, i)
			bestTime := time.Duration(best*frameMs) * time.Millisecond
			merged := false
			for oi := range onsets {
				if absDur(onsets[oi].Time-bestTime) <= time.Duration(frameMs)*time.Millisecond {
					onsets[oi].Strength = max(onsets[oi].Strength, strength[best])
					merged = true
					break
				}
			}
			if !merged {
				onsets = append(onsets, Onset{
					Time:     bestTime,
					Strength: strength[best],
				})
			}
			weakLast = best
			i = best
		}
	}
	return onsets
}

// sharpestPeak refines a local maximum at i to the strongest sample within
// two frames, so picked times sit on the sharpest point of the onset.
func sharpestPeak(strength []float64, i int) int {
	best := i
	for j := i + 1; j < len(strength) && j <= i+2; j++ {
		if strength[j] > strength[best] {
			best = j
		}
	}
	return best
}

// DetectOnsets returns the detected note-onset times of the recording, or an
// empty slice when analysis is impossible (no ffmpeg, decode failure).
func DetectOnsets(path string) ([]time.Duration, error) {
	onsets, err := DetectOnsetsWithStrength(path)
	if err != nil {
		return nil, err
	}
	out := make([]time.Duration, len(onsets))
	for i, o := range onsets {
		out[i] = o.Time
	}
	return out, nil
}

// DetectOnsetsWithStrength is DetectOnsets plus per-onset strength.
func DetectOnsetsWithStrength(path string) ([]Onset, error) {
	env, err := ExtractEnvelope(path)
	if err != nil {
		return nil, err
	}
	if len(env) == 0 {
		return nil, nil
	}
	return pickOnsets(onsetStrength(env), pcmFrameMs, minOnsetGap, 1.5), nil
}
