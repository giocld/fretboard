package player

import (
	"bytes"
	"encoding/binary"

	"fretboard/internal/model"
)

// WriteSMF writes a single-track SMF (format 0) from a list of events.
// The returned bytes are a complete .mid file.
func WriteSMF(events []Event, bpm int) ([]byte, error) {
	return writeSMF(events, bpm, false)
}

// WriteTabSMF writes a single-track SMF for events generated from tab.
// When tab is a drum tab (DetectDrumTab) note events are routed to MIDI
// channel 9 (zero-based channel 10, GM percussion) and pitches are mapped
// to GM drum sounds by string index (drumNoteForIndex). For any other tab
// the output is byte-identical to WriteSMF.
func WriteTabSMF(events []Event, bpm int, tab *model.Tab) ([]byte, error) {
	return writeSMF(events, bpm, DetectDrumTab(tab))
}

// writeSMF is WriteSMF's body; drum selects channel-9 percussion routing.
// The non-drum path must stay byte-identical to the historical output.
func writeSMF(events []Event, bpm int, drum bool) ([]byte, error) {
	if bpm <= 0 {
		bpm = 120
	}
	const ticksPerQuarter = 480

	var buf bytes.Buffer

	// Header chunk: MThd + 6 + format 0 + 1 track + ticksPerQuarter
	buf.WriteString("MThd")
	binary.Write(&buf, binary.BigEndian, uint32(6))
	binary.Write(&buf, binary.BigEndian, uint16(0))
	binary.Write(&buf, binary.BigEndian, uint16(1))
	binary.Write(&buf, binary.BigEndian, uint16(ticksPerQuarter))

	var track bytes.Buffer

	// Tempo meta event (microseconds per quarter note).
	usPerQuarter := 60_000_000 / bpm
	writeVarLen(&track, 0)
	track.WriteByte(0xFF)
	track.WriteByte(0x51)
	track.WriteByte(0x03)
	// Must be exactly 3 bytes — binary.Write(uint32) emits 4 and corrupts the track.
	track.WriteByte(byte(usPerQuarter >> 16))
	track.WriteByte(byte(usPerQuarter >> 8))
	track.WriteByte(byte(usPerQuarter))

	var lastTick int64
	for _, e := range events {
		delta := e.Tick - lastTick
		if delta < 0 {
			delta = 0
		}
		writeVarLen(&track, delta)
		lastTick = e.Tick

		switch e.Type {
		case NoteOn:
			status, note := byte(0x90), byte(e.Note)
			if drum {
				// GM channel 10 is always percussion: only the status
				// nibble changes, the mapped pitch is the drum sound.
				status, note = 0x99, byte(drumNoteForIndex(e.String))
			}
			track.WriteByte(status)
			track.WriteByte(note)
			track.WriteByte(byte(e.Vel))
		case NoteOff:
			status, note := byte(0x80), byte(e.Note)
			if drum {
				status, note = 0x89, byte(drumNoteForIndex(e.String))
			}
			track.WriteByte(status)
			track.WriteByte(note)
			track.WriteByte(byte(0))
		}
	}

	writeVarLen(&track, 0)
	track.WriteByte(0xFF)
	track.WriteByte(0x2F)
	track.WriteByte(0x00)

	buf.WriteString("MTrk")
	binary.Write(&buf, binary.BigEndian, uint32(track.Len()))
	buf.Write(track.Bytes())

	return buf.Bytes(), nil
}

func writeVarLen(w *bytes.Buffer, value int64) {
	if value == 0 {
		w.WriteByte(0)
		return
	}
	var buf [4]byte
	n := 0
	v := value
	for {
		buf[n] = byte(v & 0x7F)
		n++
		v >>= 7
		if v == 0 {
			break
		}
	}
	for i := n - 1; i > 0; i-- {
		w.WriteByte(buf[i] | 0x80)
	}
	w.WriteByte(buf[0])
}
