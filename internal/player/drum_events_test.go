package player

import (
	"bytes"
	"strings"
	"testing"

	"fretboard/internal/model"
	"fretboard/internal/parser"
)

// TestDrumTabProducesPercussionEvents guards the end-to-end drum path: a
// labeled drum tab parses to bars, Events() emits hits from x segments, and
// WriteTabSMF routes them to GM channel 9 with percussion pitches.
func TestDrumTabProducesPercussionEvents(t *testing.T) {
	src := `Song
Tuning: E Standard

HH|--x---x---x---x-|
SD|x-------x-------|
BD|----x-------x---|
`
	tab, err := parser.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if !DetectDrumTab(tab) {
		t.Fatal("drum tab should be detected")
	}
	evts, err := Events(tab, 120)
	if err != nil {
		t.Fatal(err)
	}
	var ons []Event
	for _, e := range evts {
		if e.Type == NoteOn {
			ons = append(ons, e)
		}
	}
	if len(ons) < 6 {
		t.Fatalf("expected x/o hits as note events, got %d", len(ons))
	}
	data, err := WriteTabSMF(evts, 120, tab)
	if err != nil {
		t.Fatal(err)
	}
	// Channel 9 (0x99) NoteOns must be present; channel 0 (0x90) must not.
	if !bytes.Contains(data, []byte{0x99}) {
		t.Fatal("drum SMF should use GM channel 9 (0x99)")
	}
	if bytes.Contains(data, []byte{0x90}) {
		t.Fatal("drum SMF must not use channel 0")
	}
}

// TestDrumTabEventPitchMapping guards string-index → GM percussion mapping
// in the SMF writer: string 0 (kick) and string 1 (snare) get 36 and 38.
func TestDrumTabEventPitchMapping(t *testing.T) {
	evts := []Event{
		{Type: NoteOn, Tick: 0, String: 0, Note: 0, Vel: 100},
		{Type: NoteOff, Tick: 100, String: 0, Note: 0},
		{Type: NoteOn, Tick: 200, String: 1, Note: 0, Vel: 100},
		{Type: NoteOff, Tick: 300, String: 1, Note: 0},
	}
	tab := &model.Tab{Tuning: model.Standard, Bars: []model.Bar{{Strings: []model.StringLine{
		{Segments: []model.Segment{{Char: 'x', Position: 0}}},
		{Segments: []model.Segment{{Char: 'x', Position: 1}}},
	}}}}
	if !DetectDrumTab(tab) {
		t.Fatal("x-hit tab should be detected as drum")
	}
	data, err := WriteTabSMF(evts, 120, tab)
	if err != nil {
		t.Fatal(err)
	}
	// 36 = kick (0x24), 38 = snare (0x26) on channel 9.
	if !bytes.Contains(data, []byte{0x99, 0x24}) || !bytes.Contains(data, []byte{0x99, 0x26}) {
		t.Fatal("expected kick 36 and snare 38 on channel 9")
	}
}
