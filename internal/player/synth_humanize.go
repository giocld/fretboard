package player

import (
	"math/rand/v2"
	"sort"
)

// HumanizeMIDI enables subtle humanization of generated MIDI: timing jitter,
// velocity shaping by note length, and strummed chords. Off by default so
// playback stays deterministic and existing behavior is unchanged. Wave 2
// wires config.HumanizeMIDI into this flag.
var HumanizeMIDI bool = false

// humanizeSeed fixes the RNG so humanization is fully deterministic for a
// given input: re-rendering the same tab yields the same MIDI, and tests
// can pin exact expectations.
const humanizeSeed = 42

// Humanization constants in ticks. The SMF writer uses 480 ticks per
// quarter; at the default 120 BPM a quarter is 500 ms, so 1 tick ≈ 1.04 ms.
// Ticks keep the math independent of any actual playback tempo.
const (
	jitterMaxTicks    = 5   // ≈ ±5 ms
	shortNoteMaxTicks = 240 // ≈ 250 ms
	shortVelDelta     = -6
	longVelDelta      = +6
	strumWindowTicks  = 29 // ≈ 30 ms
	strumMinTicks     = 6  // ≈ 6 ms
	strumMaxTicks     = 12 // ≈ 12 ms
)

// HumanizeEvents applies the HumanizeMIDI effects to a copy of evs and
// returns the result. When HumanizeMIDI is false it returns evs untouched,
// so callers pay no cost and get byte-identical output.
func HumanizeEvents(evs []Event) []Event {
	if !HumanizeMIDI {
		return evs
	}
	return humanizeEvents(evs, humanizeSeed)
}

// humanizeEvents is the seeded internal variant (tests pin the RNG with a
// custom seed). It never mutates the caller's slice.
func humanizeEvents(evs []Event, seed uint64) []Event {
	out := make([]Event, len(evs))
	copy(out, evs)
	rng := rand.New(rand.NewPCG(seed, seed))

	// Velocity needs the written duration, which pair-wise jitter preserves,
	// so shaping may run on the copy in any order; do it first anyway.
	shapeVelocities(out)
	jitterPairs(out, rng)
	strumChords(out, rng)
	return out
}

// shapeVelocities nudges each note's velocity by its duration: short notes
// (≤ ~250 ms) play slightly softer, longer notes slightly harder, clamped to
// 1..127. Note-off events keep velocity 0 (their Vel field is never a real
// velocity byte).
func shapeVelocities(evs []Event) {
	for i := range evs {
		if evs[i].Type != NoteOn {
			continue
		}
		dur := int64(0)
		for j := i + 1; j < len(evs); j++ {
			if evs[j].Type == NoteOff && evs[j].Note == evs[i].Note && evs[j].Tick >= evs[i].Tick {
				dur = evs[j].Tick - evs[i].Tick
				break
			}
		}
		delta := longVelDelta
		if dur <= shortNoteMaxTicks {
			delta = shortVelDelta
		}
		v := evs[i].Vel + delta
		if v < 1 {
			v = 1
		}
		if v > 127 {
			v = 127
		}
		evs[i].Vel = v
	}
}

// jitterPairs shifts each note-on and its matching note-off by the same
// ±5-tick offset: attacks become uneven while every note keeps its written
// length. Events emits each column's on/off pairs consecutively, so a
// pitch-keyed map reliably tracks the pending offset. A pair is clamped so
// a note-on never moves before tick 0.
func jitterPairs(evs []Event, rng *rand.Rand) {
	offByNote := make(map[int]int64)
	for i := range evs {
		switch evs[i].Type {
		case NoteOn:
			off := int64(rng.IntN(2*jitterMaxTicks+1) - jitterMaxTicks) // [-5, +5]
			if evs[i].Tick+off < 0 {
				off = -evs[i].Tick // a note cannot start before the song
			}
			evs[i].Tick += off
			offByNote[evs[i].Note] = off
		case NoteOff:
			if off, ok := offByNote[evs[i].Note]; ok {
				evs[i].Tick += off
				delete(offByNote, evs[i].Note)
			}
		}
	}
}

// strumChords staggers simultaneous note-ons on different strings: the low
// string fires on the chord's earliest attack, each following string 6-12
// ticks (≈6-12 ms) later, like a down-strum. Note-offs stay at their
// jittered tick so the roll never lengthens the chord. Notes attacking
// within the 30 ms window count as one chord; single notes are never rolled.
func strumChords(evs []Event, rng *rand.Rand) {
	type chordNote struct {
		idx  int
		str  int
		tick int64
	}

	var groups [][]chordNote
	var cur []chordNote
	for i := range evs {
		if evs[i].Type != NoteOn {
			continue
		}
		if len(cur) > 0 && evs[i].Tick-cur[0].tick > strumWindowTicks {
			groups = append(groups, cur)
			cur = nil
		}
		cur = append(cur, chordNote{idx: i, str: evs[i].String, tick: evs[i].Tick})
	}
	if len(cur) > 0 {
		groups = append(groups, cur)
	}

	// Find each note-on's off index once, to cap strum delays so a delayed
	// note-on never passes its own note-off.
	offs := make(map[int]int)
	for i := range evs {
		if evs[i].Type != NoteOn {
			continue
		}
		for j := i + 1; j < len(evs); j++ {
			if evs[j].Type == NoteOff && evs[j].Note == evs[i].Note {
				offs[i] = j
				break
			}
		}
	}

	for _, g := range groups {
		if len(g) < 2 {
			continue
		}
		distinct := make(map[int]bool)
		for _, n := range g {
			distinct[n.str] = true
		}
		if len(distinct) < 2 {
			continue // not a chord across strings
		}
		// Low string (index 0) first; stable so equal strings keep slice order.
		sort.SliceStable(g, func(a, b int) bool { return g[a].str < g[b].str })
		// Align the whole chord to its earliest (jittered) attack so the
		// per-string gaps are exactly the strum increments — otherwise the
		// ±5 jitter noise between strings would swamp the roll.
		base := g[0].tick
		for _, n := range g[1:] {
			if n.tick < base {
				base = n.tick
			}
		}
		delay := int64(0)
		for i, n := range g {
			if i > 0 {
				delay += int64(strumMinTicks + rng.IntN(strumMaxTicks-strumMinTicks+1)) // 6..12
			}
			if offIdx, ok := offs[n.idx]; ok {
				if cap := evs[offIdx].Tick - base - 1; delay > cap {
					delay = max(cap, 0) // never push a note-on past its own note-off
				}
			}
			evs[n.idx].Tick = base + delay
		}
	}
}
