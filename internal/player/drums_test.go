package player

import (
	"bytes"
	"strings"
	"testing"

	"fretboard/internal/model"
	"fretboard/internal/parser"
)

func parseTab(t *testing.T, s string) *model.Tab {
	t.Helper()
	tab, err := parser.Parse(strings.NewReader(s))
	if err != nil {
		t.Fatal(err)
	}
	return tab
}

func TestDetectDrumTabLabeledRows(t *testing.T) {
	// Labeled drum rows are tab evidence (HH/SD/BD labels), so the parser
	// classifies them as a tab with bars; detection then fires on the x
	// hits spread across SD and HH rows.
	tab := parseTab(t, `Drum Song

B |o---------------|o---------------|
SD|x---x---x---x---|x---x---x---x---|
HH|--x---x---x---x-|--x---x---x---x-|
`)
	if !DetectDrumTab(tab) {
		t.Fatal("labeled drum tab must be detected")
	}
	if len(tab.Bars) == 0 {
		t.Fatal("drum tab should parse bars, not a chord sheet")
	}
}

func TestDetectDrumTabXOHitRows(t *testing.T) {
	// Unlabeled x/o rows are pure bar-grid lines, which the parser treats
	// as a tab; x hits on multiple rows still read as drums.
	tab := parseTab(t, `Drum3

|--x---x---x---x-|
|-x---x---x---x--|
|x---------------|
`)
	if !DetectDrumTab(tab) {
		t.Fatal("x/o drum tab must be detected")
	}
}

func TestDetectDrumTabParsedBarsXOHits(t *testing.T) {
	// A tab parsed into bars with x segments on 2 distinct strings (the
	// path that matters if the parser starts keeping drum rows as bars).
	tab := &model.Tab{Metadata: map[string]string{}, Tuning: model.Standard,
		Bars: []model.Bar{{Strings: []model.StringLine{
			{Segments: []model.Segment{{Char: '-', Position: 0}, {Char: 'x', Position: 1}}},
			{Segments: []model.Segment{{Char: '-', Position: 0}, {Char: 'x', Position: 1}}},
			{Segments: []model.Segment{{Char: '-', Position: 0}}},
		}}}}
	if !DetectDrumTab(tab) {
		t.Fatal("x segments on 2 distinct strings must be detected")
	}
}

func TestDetectDrumTabFalse(t *testing.T) {
	cases := []struct {
		name string
		tab  *model.Tab
	}{
		{"nil", nil},
		{"normal guitar tab", parseTab(t, "Tuning: E Standard\n\ne|0-3-5|\n")},
		{"chord sheet lyrics", parseTab(t, "Song\n\nAm C\nF G\n")},
		{"single muted string", parseTab(t, "Riff\n\ne|----------------|\nB|----------------|\nG|----------------|\nD|----------------|\nA|----x---x-------|\nE|----------------|\n")},
		{"empty raw", &model.Tab{Metadata: map[string]string{"raw": "just text, no pipes"}}},
	}
	for _, c := range cases {
		if DetectDrumTab(c.tab) {
			t.Errorf("%s: must not be a drum tab", c.name)
		}
	}
}

func TestDrumNoteForIndex(t *testing.T) {
	cases := []struct {
		i    int
		want int
	}{
		{0, 36}, // kick
		{1, 38}, // snare
		{2, 42}, // closed hat
		{3, 43}, // high floor tom
		{4, 45}, // low tom
		{5, 48}, // hi-mid tom
		{6, 50}, // high tom
		{7, 50}, // clamped
		{-1, 50},
	}
	for _, c := range cases {
		if got := drumNoteForIndex(c.i); got != c.want {
			t.Errorf("drumNoteForIndex(%d) = %d, want %d", c.i, got, c.want)
		}
	}
}

func TestDrumNoteForLabel(t *testing.T) {
	cases := []struct {
		label string
		open  bool
		want  int
		ok    bool
	}{
		{"BD", false, 36, true}, {"B", false, 36, true}, {"bd", false, 36, true},
		{"SD", false, 38, true}, {"Sd", false, 38, true}, {"s", false, 38, true},
		{"HH", false, 42, true}, {"HH", true, 46, true}, {"h", true, 46, true},
		{"C", false, 49, true}, {"CC", false, 49, true}, {"CH", false, 49, true}, {"CY", false, 49, true},
		{"T", false, 41, true},
		{"FT", false, 43, true}, {"F", false, 43, true},
		{"HT", false, 45, true},
		{"R", false, 51, true},
		{"ZZ", false, 0, false}, {"", false, 0, false},
	}
	for _, c := range cases {
		got, ok := drumNoteForLabel(c.label, c.open)
		if got != c.want || ok != c.ok {
			t.Errorf("drumNoteForLabel(%q, open=%v) = (%d, %v), want (%d, %v)", c.label, c.open, got, ok, c.want, c.ok)
		}
	}
}

// smfTrackEvents walks a format-0 SMF track after the MTrk header and
// returns (status, note, vel) triples for every note event.
func smfTrackEvents(t *testing.T, data []byte) [][3]byte {
	t.Helper()
	idx := bytes.Index(data, []byte("MTrk"))
	if idx < 0 {
		t.Fatal("no MTrk chunk")
	}
	track := data[idx+8:]
	var out [][3]byte
	i := 0
	for i < len(track) {
		// delta (var-len)
		for i < len(track) && track[i]&0x80 != 0 {
			i++
		}
		i++
		if i >= len(track) {
			t.Fatal("track truncated at delta")
		}
		st := track[i]
		switch {
		case st == 0xFF: // meta: type, len, payload
			i += 3 + int(track[i+2])
		case st == 0x90 || st == 0x99 || st == 0x80 || st == 0x89:
			out = append(out, [3]byte{st, track[i+1], track[i+2]})
			i += 3
		case st == 0x2F:
			return out
		default:
			i++ // unknown/other event: skip status byte
		}
	}
	return out
}

func TestWriteTabSMFDrumRoutesChannel9(t *testing.T) {
	evts := []Event{
		{Type: NoteOn, Tick: 0, String: 0, Fret: 0, Note: 40, Vel: 100},
		{Type: NoteOn, Tick: 0, String: 1, Fret: 0, Note: 47, Vel: 100},
		{Type: NoteOn, Tick: 0, String: 2, Fret: 0, Note: 52, Vel: 100},
		{Type: NoteOff, Tick: 480, String: 0, Fret: 0, Note: 40, Vel: 0},
		{Type: NoteOff, Tick: 480, String: 1, Fret: 0, Note: 47, Vel: 0},
		{Type: NoteOff, Tick: 480, String: 2, Fret: 0, Note: 52, Vel: 0},
	}
	tab := parseTab(t, "Drums\n\nSD|x---|\nBD|o---|\nHH|--x-|\n")

	data, err := WriteTabSMF(evts, 120, tab)
	if err != nil {
		t.Fatal(err)
	}
	got := smfTrackEvents(t, data)
	want := [][3]byte{
		{0x99, 36, 100}, // string 0 -> kick
		{0x99, 38, 100}, // string 1 -> snare
		{0x99, 42, 100}, // string 2 -> closed hat
		{0x89, 36, 0},
		{0x89, 38, 0},
		{0x89, 42, 0},
	}
	if len(got) != len(want) {
		t.Fatalf("note events = %d, want %d: % x", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("event %d = % x, want % x", i, got[i], want[i])
		}
	}
}

func TestWriteTabSMFNonDrumIdenticalToWriteSMF(t *testing.T) {
	evts := []Event{
		{Type: NoteOn, Tick: 0, String: 0, Fret: 5, Note: 45, Vel: 100},
		{Type: NoteOff, Tick: 480, String: 0, Fret: 5, Note: 45, Vel: 0},
	}
	plain, err := WriteSMF(evts, 96)
	if err != nil {
		t.Fatal(err)
	}
	guitar := parseTab(t, "Tuning: E Standard\n\ne|0-3-5|\n")
	viaTab, err := WriteTabSMF(evts, 96, guitar)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(plain, viaTab) {
		t.Fatalf("non-drum WriteTabSMF must be byte-identical to WriteSMF")
	}
	if _, err := WriteTabSMF(evts, 96, nil); err != nil {
		t.Fatal(err)
	}
	if viaNil, err := WriteTabSMF(evts, 96, nil); err != nil || !bytes.Equal(plain, viaNil) {
		t.Fatalf("nil tab must take the non-drum path (equal=%v, err=%v)", bytes.Equal(plain, viaNil), err)
	}
}
