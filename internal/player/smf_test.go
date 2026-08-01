package player

import (
	"strings"
	"testing"

	"github.com/YOUR_USERNAME/fretboard/internal/parser"
)

func TestWriteSMFTempoMetaIsThreeBytes(t *testing.T) {
	tab, err := parser.Parse(strings.NewReader("Tuning: E Standard\n\ne|0-3-5|\n"))
	if err != nil {
		t.Fatal(err)
	}
	evts, err := Events(tab, 120)
	if err != nil {
		t.Fatal(err)
	}
	data, err := WriteSMF(evts, 120)
	if err != nil {
		t.Fatal(err)
	}
	idx := strings.Index(string(data), "MTrk")
	if idx < 0 {
		t.Fatal("no MTrk chunk")
	}
	track := data[idx+8:]
	if len(track) < 11 {
		t.Fatalf("track too short: %d", len(track))
	}
	if track[0] != 0 || track[1] != 0xFF || track[2] != 0x51 || track[3] != 0x03 {
		t.Fatalf("unexpected tempo header: % x", track[:8])
	}
	if track[4] != 0x07 || track[5] != 0xA1 || track[6] != 0x20 {
		t.Fatalf("bad tempo bytes: % x want 07 a1 20", track[4:7])
	}
	if track[7] != 0x00 || track[8] != 0x90 {
		t.Fatalf("expected note-on after tempo, got % x", track[7:11])
	}
}
