package player

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"os/exec"
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

// ExtractEnvelope decodes the audio to a mono energy envelope: one value per
// pcmFrameMs window (RMS of the windowed samples). Returns nil when ffmpeg
// is unavailable or the decode fails.
func ExtractEnvelope(path string) ([]float64, error) {
	if path == "" {
		return nil, nil
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return nil, nil // optional assist: no ffmpeg, no analysis
	}
	args := []string{
		"-hide_banner", "-loglevel", "error", "-i", path,
		"-t", fmt.Sprintf("%.0f", maxPCMDuration.Seconds()),
		"-ac", "1", "-ar", fmt.Sprintf("%d", pcmSampleRate),
		"-f", "s16le", "-",
	}
	ctx, cancel := context.WithTimeout(context.Background(), pcmDecodeTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ffmpeg", args...).Output()
	if err != nil {
		return nil, fmt.Errorf("decode envelope: %w", err)
	}
	return envelopeFromPCM(out, pcmSampleRate, pcmFrameMs, pcmWindowMs), nil
}

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
		end := start + windowSamples
		if end > len(samples) {
			end = len(samples)
		}
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
		if s > globalMax {
			globalMax = s
		}
	}
	floor := globalMax * 0.04
	// Local threshold from a sliding window (2 s each side).
	win := 2000 / frameMs
	for i := 1; i < len(strength)-1; i++ {
		if strength[i] <= strength[i-1] || strength[i] <= strength[i+1] {
			continue
		}
		lo := i - win
		if lo < 0 {
			lo = 0
		}
		hi := i + win
		if hi > len(strength) {
			hi = len(strength)
		}
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
		thr := mean + k*std
		if thr < floor {
			thr = floor
		}
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
					if strength[best] > onsets[oi].Strength {
						onsets[oi].Strength = strength[best]
					}
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

// writeSyntheticWAV writes a mono 16-bit WAV with short clicks (tone bursts)
// at the given times — the hermetic test fixture for the analysis pipeline.
func writeSyntheticWAV(path string, rate int, clicks []time.Duration, clickDur time.Duration) error {
	return writeSyntheticWAVAlt(path, rate, clicks, clickDur, 0)
}

// writeSyntheticWAVAlt writes a mono 16-bit WAV with short clicks at the
// given times; every strongEvery-th click (1-based) is accented (louder) so
// recordings can carry a downbeat/quarter grid like real music. strongEvery
// 0 means uniform strength.
func writeSyntheticWAVAlt(path string, rate int, clicks []time.Duration, clickDur time.Duration, strongEvery int) error {
	clickSamples := make(map[int]bool)
	strongSamples := make(map[int]bool)
	clickLen := int(clickDur.Seconds() * float64(rate))
	for ci, c := range clicks {
		start := int(c.Seconds() * float64(rate))
		strong := strongEvery > 0 && ci%strongEvery == 0
		for i := 0; i < clickLen; i++ {
			clickSamples[start+i] = true
			if strong {
				strongSamples[start+i] = true
			}
		}
	}
	total := 0
	if len(clicks) > 0 {
		last := clicks[len(clicks)-1]
		total = int(last.Seconds()*float64(rate)) + rate*2
	}
	var buf bytes.Buffer
	buf.WriteString("RIFF")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(36+total*2))
	buf.WriteString("WAVEfmt ")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(16)) // fmt chunk size
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))  // PCM
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))  // mono
	_ = binary.Write(&buf, binary.LittleEndian, uint32(rate))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(rate*2)) // byte rate
	_ = binary.Write(&buf, binary.LittleEndian, uint16(2))      // block align
	_ = binary.Write(&buf, binary.LittleEndian, uint16(16))     // bits
	buf.WriteString("data")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(total*2))
	phase := 0.0
	phaseInc := 2 * math.Pi * 440 / float64(rate)
	for i := 0; i < total; i++ {
		var v int16
		switch {
		case strongSamples[i]:
			phase += phaseInc
			v = int16(12000 * math.Sin(phase))
		case clickSamples[i]:
			phase += phaseInc
			v = int16(4500 * math.Sin(phase))
		default:
			// Quiet noise floor so silence detection never confuses us.
			phase = 0
			v = int16(120 * math.Sin(float64(i)*0.7))
		}
		_ = binary.Write(&buf, binary.LittleEndian, v)
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}
