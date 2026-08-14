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
