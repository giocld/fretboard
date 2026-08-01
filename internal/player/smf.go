package player

import (
	"bytes"
	"encoding/binary"
)

// WriteSMF writes a single-track SMF (format 0) from a list of events.
// The returned bytes are a complete .mid file.
func WriteSMF(events []Event, bpm int) ([]byte, error) {
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
			track.WriteByte(0x90)
			track.WriteByte(byte(e.Note))
			track.WriteByte(byte(e.Vel))
		case NoteOff:
			track.WriteByte(0x80)
			track.WriteByte(byte(e.Note))
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
