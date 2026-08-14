package player

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// wavFmtChunk builds a "fmt " chunk with the same layout writeSyntheticWAV
// emits: 16-byte PCM header (compression, channels, rate, byte rate, block
// align, bits).
func wavFmtChunk(compression, channels, rate, bits int) []byte {
	var b bytes.Buffer
	b.WriteString("fmt ")
	_ = binary.Write(&b, binary.LittleEndian, uint32(16))
	_ = binary.Write(&b, binary.LittleEndian, uint16(compression))
	_ = binary.Write(&b, binary.LittleEndian, uint16(channels))
	_ = binary.Write(&b, binary.LittleEndian, uint32(rate))
	_ = binary.Write(&b, binary.LittleEndian, uint32(rate*channels*bits/8))
	_ = binary.Write(&b, binary.LittleEndian, uint16(channels*bits/8))
	_ = binary.Write(&b, binary.LittleEndian, uint16(bits))
	return b.Bytes()
}

func wavDataChunk(samples []int16) []byte {
	var b bytes.Buffer
	b.WriteString("data")
	_ = binary.Write(&b, binary.LittleEndian, uint32(len(samples)*2))
	for _, s := range samples {
		_ = binary.Write(&b, binary.LittleEndian, s)
	}
	return b.Bytes()
}

// wavBlob wraps chunks in a RIFF/WAVE container with the same header layout
// writeSyntheticWAV produces (RIFF size = 36 + data bytes).
func wavBlob(chunks ...[]byte) []byte {
	var data bytes.Buffer
	for _, c := range chunks {
		data.Write(c)
	}
	var b bytes.Buffer
	b.WriteString("RIFF")
	_ = binary.Write(&b, binary.LittleEndian, uint32(4+data.Len()))
	b.WriteString("WAVE")
	b.Write(data.Bytes())
	return b.Bytes()
}

// TestDecodeWAVPCM guards the stdlib WAV parser: a mono 8 kHz s16 blob built
// with the same header layout as writeSyntheticWAV round-trips byte for byte,
// stereo pairs downmix to mono by averaging, and a 16 kHz stream decimates
// to pcmSampleRate by nearest-sample picking.
func TestDecodeWAVPCM(t *testing.T) {
	// Mono 8 kHz: no downmix, no decimation, bytes unchanged.
	samples := make([]int16, 100)
	for i := range samples {
		samples[i] = int16((i*37)%200 - 100)
	}
	blob := wavBlob(wavFmtChunk(1, 1, 8000, 16), wavDataChunk(samples))
	out, err := decodeWAVPCM(blob)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, blob[44:]) {
		t.Fatalf("8 kHz mono must round-trip byte for byte, got %x want %x", out, blob[44:])
	}

	// Stereo 8 kHz: four (1000, 2000) pairs average to four 1500 samples.
	stereo := make([]int16, 0, 8)
	for i := 0; i < 4; i++ {
		stereo = append(stereo, 1000, 2000)
	}
	out, err = decodeWAVPCM(wavBlob(wavFmtChunk(1, 2, 8000, 16), wavDataChunk(stereo)))
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 4*2 {
		t.Fatalf("4 stereo pairs must downmix to 4 mono samples, got %d bytes", len(out))
	}
	for i := 0; i < 4; i++ {
		got := int16(binary.LittleEndian.Uint16(out[i*2 : i*2+2]))
		if got != 1500 {
			t.Fatalf("downmixed sample %d = %d, want 1500", i, got)
		}
	}

	// 16 kHz mono: 10 samples decimate 2:1 to 5 samples at pcmSampleRate.
	fast := make([]int16, 10)
	for i := range fast {
		fast[i] = int16(i * 100)
	}
	out, err = decodeWAVPCM(wavBlob(wavFmtChunk(1, 1, 16000, 16), wavDataChunk(fast)))
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 5*2 {
		t.Fatalf("16 kHz must decimate to half the samples, got %d bytes (%d samples)", len(out), len(out)/2)
	}
	for i := 0; i < 5; i++ {
		got := int16(binary.LittleEndian.Uint16(out[i*2 : i*2+2]))
		if want := int16(i * 200); got != want { // nearest-sample: samples 0,2,4,6,8
			t.Fatalf("decimated sample %d = %d, want %d", i, got, want)
		}
	}

	// Error paths: not a WAV, missing fmt, missing data, non-PCM.
	if _, err := decodeWAVPCM([]byte("not a wav file")); err == nil {
		t.Fatal("garbage must fail with not-a-WAV")
	}
	if _, err := decodeWAVPCM(wavBlob(wavDataChunk(samples))); err == nil {
		t.Fatal("a data chunk without fmt must fail")
	}
	if _, err := decodeWAVPCM(wavBlob(wavFmtChunk(1, 1, 8000, 16))); err == nil {
		t.Fatal("a fmt chunk without data must fail")
	}
	if _, err := decodeWAVPCM(wavBlob(wavFmtChunk(85, 1, 8000, 16), wavDataChunk(samples))); err == nil {
		t.Fatal("a non-PCM compression must fail")
	}
}

// TestExtractEnvelopeMissingDecoder guards F3: with no decoder on PATH the
// call must fail with errNoDecoder — never a silent nil envelope.
func TestExtractEnvelopeMissingDecoder(t *testing.T) {
	orig := findDecoder
	findDecoder = func() string { return "" }
	defer func() { findDecoder = orig }()

	path := filepath.Join(t.TempDir(), "clicks.wav")
	if err := writeSyntheticWAV(path, 8000, []time.Duration{time.Second}, 30*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	env, err := ExtractEnvelope(path)
	if err == nil || !errors.Is(err, errNoDecoder) {
		t.Fatalf("expected errNoDecoder, got env=%v err=%v", env, err)
	}
	if env != nil {
		t.Fatalf("no decoder must not yield an envelope, got %d values", len(env))
	}
}

// TestExtractEnvelopeMPG123Fallback guards the fallback decode chain: when
// mpg123 is selected, a synthetic WAV must decode through the stdlib parser
// into a non-empty envelope.
func TestExtractEnvelopeMPG123Fallback(t *testing.T) {
	if _, err := exec.LookPath("mpg123"); err != nil {
		t.Skip("mpg123 not available")
	}
	orig := findDecoder
	findDecoder = func() string { return "mpg123" }
	defer func() { findDecoder = orig }()

	path := filepath.Join(t.TempDir(), "clicks.wav")
	var clicks []time.Duration
	for i := 0; i < 10; i++ {
		clicks = append(clicks, time.Duration(i)*500*time.Millisecond)
	}
	if err := writeSyntheticWAV(path, 8000, clicks, 30*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	env, err := ExtractEnvelope(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(env) == 0 {
		t.Fatal("mpg123 fallback must produce a non-empty envelope")
	}
}
