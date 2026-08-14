package player

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"time"
)

// errNoDecoder is returned when neither ffmpeg nor mpg123 is available to
// decode the recording: explicit degradation instead of a silent nil.
var errNoDecoder = errors.New("no audio decoder available (ffmpeg or mpg123)")

// findDecoder returns the name of the first available audio decoder:
// "ffmpeg" (primary) or "mpg123" (fallback), or "" when neither is on PATH.
// It is a package variable so tests can force each branch without touching
// the process PATH.
var findDecoder = func() string {
	if _, err := exec.LookPath("ffmpeg"); err == nil {
		return "ffmpeg"
	}
	if _, err := exec.LookPath("mpg123"); err == nil {
		return "mpg123"
	}
	return ""
}

// ExtractEnvelope decodes the audio to a mono energy envelope: one value per
// pcmFrameMs window (RMS of the windowed samples). ffmpeg is the primary
// decoder; when it is absent the mpg123 fallback decodes to WAV on stdout and
// a stdlib WAV parser takes over. When no decoder exists the call fails with
// errNoDecoder rather than silently returning nil. path == "" (no audio to
// analyze) keeps its legacy nil, nil meaning.
func ExtractEnvelope(path string) ([]float64, error) {
	if path == "" {
		return nil, nil
	}
	switch findDecoder() {
	case "ffmpeg":
		return decodeEnvelopeFFmpeg(path)
	case "mpg123":
		return decodeEnvelopeMPG123(path)
	default:
		return nil, errNoDecoder
	}
}

// decodeEnvelopeFFmpeg decodes the recording with ffmpeg straight to mono
// s16le at pcmSampleRate (bounded by -t and the context timeout) and derives
// the envelope.
func decodeEnvelopeFFmpeg(path string) ([]float64, error) {
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

// decodeEnvelopeMPG123 decodes the recording with mpg123 -w - (WAV to stdout)
// under the same timeout as ffmpeg. mpg123 has no duration-cap flag, so the
// decoded length is checked post-hoc against the maxPCMDuration budget; the
// WAV is then parsed with the stdlib parser and converted to the envelope.
func decodeEnvelopeMPG123(path string) ([]float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), pcmDecodeTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "mpg123", "-w", "-", path).Output()
	if err != nil {
		return nil, fmt.Errorf("decode envelope (mpg123): %w", err)
	}
	maxBytes := int(maxPCMDuration.Seconds() * float64(pcmSampleRate) * 2)
	if len(out) > maxBytes {
		return nil, fmt.Errorf("decode envelope (mpg123): audio exceeds %s (mpg123 cannot cap the decode)", maxPCMDuration)
	}
	pcm, err := decodeWAVPCM(out)
	if err != nil {
		return nil, fmt.Errorf("decode envelope (mpg123): %w", err)
	}
	return envelopeFromPCM(pcm, pcmSampleRate, pcmFrameMs, pcmWindowMs), nil
}

// decodeWAVPCM parses a RIFF/WAVE file with the standard library only and
// returns its audio as mono s16le PCM at pcmSampleRate. The fmt chunk fixes
// the layout (PCM, channels, rate, bits), stereo is downmixed by averaging
// channels, and other rates are resampled by nearest-sample picking.
// Errors: not RIFF/WAVE, missing fmt or data chunk, non-PCM compression,
// non-s16 bit depth, or a zero channel/rate layout.
func decodeWAVPCM(data []byte) ([]byte, error) {
	if len(data) < 12 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return nil, errors.New("not a RIFF/WAVE file")
	}
	var (
		channels int
		rate     int
		pcm      bool
		audio    []byte
	)
	for off := 12; off+8 <= len(data); {
		id := string(data[off : off+4])
		size := int(binary.LittleEndian.Uint32(data[off+4 : off+8]))
		body := off + 8
		if size > len(data)-body {
			break // truncated chunk: stop scanning
		}
		switch id {
		case "fmt ":
			if size < 16 {
				return nil, errors.New("WAV fmt chunk too small")
			}
			if comp := binary.LittleEndian.Uint16(data[body : body+2]); comp != 1 {
				return nil, fmt.Errorf("unsupported WAV compression %d (only PCM)", comp)
			}
			channels = int(binary.LittleEndian.Uint16(data[body+2 : body+4]))
			rate = int(binary.LittleEndian.Uint32(data[body+4 : body+8]))
			if bits := binary.LittleEndian.Uint16(data[body+14 : body+16]); bits != 16 {
				return nil, fmt.Errorf("unsupported WAV bit depth %d (only s16)", bits)
			}
			pcm = true
		case "data":
			audio = data[body : body+size]
		}
		off = body + size
		if size&1 != 0 {
			off++ // chunks are word-aligned
		}
	}
	if !pcm {
		return nil, errors.New("WAV missing fmt chunk")
	}
	if audio == nil {
		return nil, errors.New("WAV missing data chunk")
	}
	if channels == 0 || rate == 0 {
		return nil, errors.New("WAV with zero channels or rate")
	}
	return resampleMonoS16(audio, channels, rate), nil
}

// resampleMonoS16 converts interleaved s16 samples to mono s16 at
// pcmSampleRate: channels are averaged into one stream, then the stream is
// resampled by nearest-sample picking (decimation for higher rates, hold for
// lower ones).
func resampleMonoS16(audio []byte, channels, rate int) []byte {
	nIn := len(audio) / (2 * channels)
	if nIn == 0 {
		return nil
	}
	mono := make([]int16, nIn)
	for i := 0; i < nIn; i++ {
		var sum int64
		for c := 0; c < channels; c++ {
			sum += int64(int16(binary.LittleEndian.Uint16(audio[(i*channels+c)*2:])))
		}
		mono[i] = int16(sum / int64(channels))
	}
	nOut := nIn * pcmSampleRate / rate
	out := make([]byte, 0, nOut*2)
	var b [2]byte
	for i := 0; i < nOut; i++ {
		src := i * rate / pcmSampleRate
		binary.LittleEndian.PutUint16(b[:], uint16(mono[src]))
		out = append(out, b[0], b[1])
	}
	return out
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
