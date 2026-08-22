package player

import (
	"reflect"
	"testing"
)

// withHumanize sets the package-level HumanizeMIDI flag for the test and
// restores it afterwards so other tests keep the default (off).
func withHumanize(t *testing.T, on bool) {
	t.Helper()
	prev := HumanizeMIDI
	HumanizeMIDI = on
	t.Cleanup(func() { HumanizeMIDI = prev })
}

// isolatedPairs builds well-separated single-note on/off pairs: no two
// note-ons attack within the strum window, so only jitter applies.
func isolatedPairs() []Event {
	return []Event{
		{Type: NoteOn, Tick: 0, String: 0, Fret: 3, Note: 52, Vel: 100},
		{Type: NoteOff, Tick: 240, String: 0, Fret: 3, Note: 52, Vel: 0},
		{Type: NoteOn, Tick: 480, String: 2, Fret: 5, Note: 64, Vel: 100},
		{Type: NoteOff, Tick: 960, String: 2, Fret: 5, Note: 64, Vel: 0},
		{Type: NoteOn, Tick: 1440, String: 3, Fret: 2, Note: 57, Vel: 100},
		{Type: NoteOff, Tick: 1680, String: 3, Fret: 2, Note: 57, Vel: 0},
		{Type: NoteOn, Tick: 1920, String: 1, Fret: 4, Note: 60, Vel: 100},
		{Type: NoteOff, Tick: 2160, String: 1, Fret: 4, Note: 60, Vel: 0},
	}
}

func TestHumanizeEventsPassthroughWhenDisabled(t *testing.T) {
	withHumanize(t, false)
	in := isolatedPairs()
	out := HumanizeEvents(in)
	if !reflect.DeepEqual(out, in) {
		t.Fatalf("disabled humanization must be byte-identical:\n got %+v\nwant %+v", out, in)
	}
	var empty []Event
	if got := HumanizeEvents(empty); got != nil || len(got) != 0 {
		t.Fatalf("empty input: got %v, want nil", got)
	}
}

func TestHumanizeEventsJitterBounds(t *testing.T) {
	withHumanize(t, true)
	in := isolatedPairs()
	out := HumanizeEvents(in)
	if len(out) != len(in) {
		t.Fatalf("humanization changed event count: %d -> %d", len(in), len(out))
	}
	// Every event shifts by at most ±5 ticks and never before tick 0.
	for i := range in {
		delta := out[i].Tick - in[i].Tick
		if delta < -jitterMaxTicks || delta > jitterMaxTicks {
			t.Fatalf("event %d jitter out of range: %d (in %d, out %d)", i, delta, in[i].Tick, out[i].Tick)
		}
		if out[i].Tick < 0 {
			t.Fatalf("event %d moved before tick 0: %d", i, out[i].Tick)
		}
	}
	// Pair-wise jitter preserves each note's written duration.
	for i := 0; i < len(in); i += 2 {
		inDur := in[i+1].Tick - in[i].Tick
		outDur := out[i+1].Tick - out[i].Tick
		if inDur != outDur {
			t.Fatalf("pair %d duration changed: %d -> %d", i/2, inDur, outDur)
		}
	}
}

func TestHumanizeEventsVelocityShaping(t *testing.T) {
	withHumanize(t, true)
	in := []Event{
		{Type: NoteOn, Tick: 0, String: 0, Note: 40, Vel: 100}, // 240 ticks: short -> 94
		{Type: NoteOff, Tick: 240, String: 0, Note: 40, Vel: 0},
		{Type: NoteOn, Tick: 480, String: 0, Note: 45, Vel: 100}, // 960 ticks: long -> 106
		{Type: NoteOff, Tick: 1440, String: 0, Note: 45, Vel: 0},
		{Type: NoteOn, Tick: 1920, String: 0, Note: 50, Vel: 1}, // short, clamp floor
		{Type: NoteOff, Tick: 2160, String: 0, Note: 50, Vel: 0},
		{Type: NoteOn, Tick: 2400, String: 0, Note: 55, Vel: 127}, // long, clamp ceiling
		{Type: NoteOff, Tick: 3360, String: 0, Note: 55, Vel: 0},
	}
	out := HumanizeEvents(in)
	want := []int{94, 0, 106, 0, 1, 0, 127, 0}
	for i, w := range want {
		if out[i].Vel != w {
			t.Fatalf("event %d velocity: got %d, want %d", i, out[i].Vel, w)
		}
	}
}

func TestHumanizeEventsStrumRoll(t *testing.T) {
	withHumanize(t, true)
	in := []Event{
		{Type: NoteOn, Tick: 100, String: 1, Fret: 3, Note: 55, Vel: 100}, // isolated lead-in
		{Type: NoteOff, Tick: 400, String: 1, Fret: 3, Note: 55, Vel: 0},
		{Type: NoteOn, Tick: 1200, String: 0, Fret: 0, Note: 40, Vel: 100}, // 3-string chord
		{Type: NoteOn, Tick: 1200, String: 1, Fret: 2, Note: 45, Vel: 100},
		{Type: NoteOn, Tick: 1200, String: 2, Fret: 2, Note: 50, Vel: 100},
		{Type: NoteOff, Tick: 1680, String: 0, Fret: 0, Note: 40, Vel: 0},
		{Type: NoteOff, Tick: 1680, String: 1, Fret: 2, Note: 45, Vel: 0},
		{Type: NoteOff, Tick: 1680, String: 2, Fret: 2, Note: 50, Vel: 0},
	}
	out := HumanizeEvents(in)
	// Chord note-ons stagger low string first, 6-12 ticks per string.
	on0, on1, on2 := out[2].Tick, out[3].Tick, out[4].Tick
	if !(on0 < on1 && on1 < on2) {
		t.Fatalf("chord not staggered by string: ons %d %d %d", on0, on1, on2)
	}
	if g := on1 - on0; g < strumMinTicks || g > strumMaxTicks {
		t.Fatalf("strum gap string 0->1: %d, want [%d,%d]", g, strumMinTicks, strumMaxTicks)
	}
	if g := on2 - on1; g < strumMinTicks || g > strumMaxTicks {
		t.Fatalf("strum gap string 1->2: %d, want [%d,%d]", g, strumMinTicks, strumMaxTicks)
	}
	// No note-on may pass its own note-off.
	for i := 2; i <= 4; i++ {
		off := out[i+3].Tick // offs are the next three events
		if out[i].Tick >= off {
			t.Fatalf("note-on %d (tick %d) at/after its note-off %d", i, out[i].Tick, off)
		}
	}
	// The isolated lead-in note is only jittered, never rolled.
	if d := out[0].Tick - in[0].Tick; d < -jitterMaxTicks || d > jitterMaxTicks {
		t.Fatalf("isolated note over-affected: delta %d", d)
	}
}

func TestHumanizeEventsSameStringNotRolled(t *testing.T) {
	withHumanize(t, true)
	in := []Event{
		{Type: NoteOn, Tick: 500, String: 0, Note: 40, Vel: 100}, // two notes, one string
		{Type: NoteOn, Tick: 500, String: 0, Note: 45, Vel: 100},
		{Type: NoteOff, Tick: 800, String: 0, Note: 40, Vel: 0},
		{Type: NoteOff, Tick: 800, String: 0, Note: 45, Vel: 0},
	}
	out := HumanizeEvents(in)
	// No roll across a single string: ons differ by jitter only (≤ 2*5).
	if gap := out[1].Tick - out[0].Tick; gap > 2*jitterMaxTicks {
		t.Fatalf("same-string pair staggered by %d ticks, want jitter only", gap)
	}
}

func TestHumanizeEventsDeterministicAndNonMutating(t *testing.T) {
	withHumanize(t, true)
	in := isolatedPairs()
	orig := make([]Event, len(in))
	copy(orig, in)

	a := humanizeEvents(in, humanizeSeed)
	b := humanizeEvents(in, humanizeSeed)
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("seeded humanization not deterministic:\n a %+v\n b %+v", a, b)
	}
	if !reflect.DeepEqual(in, orig) {
		t.Fatalf("humanizeEvents mutated its input:\n got %+v\nwant %+v", in, orig)
	}

	// Public entry point with the fixed seed is deterministic too.
	c := HumanizeEvents(in)
	d := HumanizeEvents(in)
	if !reflect.DeepEqual(c, d) {
		t.Fatal("HumanizeEvents not deterministic with fixed seed")
	}
}
