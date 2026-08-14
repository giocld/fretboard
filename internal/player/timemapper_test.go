package player

import (
	"testing"
	"time"
)

// quarterBarSchedule builds n bars of one quarter-note step each: 480 ticks
// per bar, so at 120 BPM every bar spans 500ms and bar i starts at i*500ms.
func quarterBarSchedule(bars int) []PlaybackStep {
	schedule := make([]PlaybackStep, 0, bars)
	for b := 0; b < bars; b++ {
		schedule = append(schedule, PlaybackStep{Bar: b, Col: 0, Ticks: ticksPerQuarter})
	}
	return schedule
}

// secs converts a float seconds value to a time.Duration.
func secs(s float64) time.Duration {
	return time.Duration(s * float64(time.Second))
}

// TestAudioAtScoreRoundTripMidSegment maps a score time inside an anchored
// segment to audio and back; the recovered score must match within 2ms.
func TestAudioAtScoreRoundTripMidSegment(t *testing.T) {
	schedule := quarterBarSchedule(8)
	// Bar 1 (500ms of score) -> 10s, bar 3 (1500ms) -> 14s: the segment
	// stretches 1000ms of score across 4s of audio.
	points := []SyncPoint{{Bar: 1, Seconds: 10}, {Bar: 3, Seconds: 14}}
	tm := NewTimeMapper(schedule, points, 120)
	for _, score := range []time.Duration{
		1000 * time.Millisecond, // mid-segment, on a bar boundary
		1250 * time.Millisecond, // mid-segment, mid-bar
	} {
		audio := tm.AudioAtScore(score)
		back, ok := tm.ScoreAtAudio(float64(audio) / float64(time.Second))
		if !ok {
			t.Fatalf("ScoreAtAudio(%v) returned ok=false", audio)
		}
		if d := back - score; d < -2*time.Millisecond || d > 2*time.Millisecond {
			t.Fatalf("round trip %v -> %v -> %v off by %v (want within 2ms)", score, audio, back, d)
		}
	}
	// The forward mapping itself lands on the segment line: 1250ms -> 13s.
	if got := tm.AudioAtScore(1250 * time.Millisecond); got != 13*time.Second {
		t.Fatalf("AudioAtScore(1250ms) = %v, want 13s", got)
	}
}

// TestNoAnchorFallbackMatchesNaiveOffset verifies the no-anchor fallback is
// exactly the naive loopStartTime formula from viewer_practice.go:
// ScheduleTimeAtBar(bar) + audio offset.
func TestNoAnchorFallbackMatchesNaiveOffset(t *testing.T) {
	schedule := quarterBarSchedule(8)
	tm := NewTimeMapper(schedule, nil, 120)
	tm.SetAudioOffset(20 * time.Second)
	for _, bar := range []int{0, 1, 3, 7} {
		score := ScheduleTimeAtBar(schedule, bar, 120)
		want := score + 20*time.Second
		if got := tm.AudioAtScore(score); got != want {
			t.Fatalf("AudioAtScore(%v) = %v, want %v", score, got, want)
		}
	}
}

// TestAudioAtScoreMonotonicPastLastAnchor checks that past the final anchor
// the last segment's rate (4s audio per 1s score) is extended, the map stays
// non-decreasing, and outputs never go negative.
func TestAudioAtScoreMonotonicPastLastAnchor(t *testing.T) {
	schedule := quarterBarSchedule(8)
	points := []SyncPoint{{Bar: 1, Seconds: 10}, {Bar: 3, Seconds: 14}}
	tm := NewTimeMapper(schedule, points, 120)
	for _, tc := range []struct{ score, want time.Duration }{
		{1500 * time.Millisecond, 14 * time.Second},
		{2000 * time.Millisecond, 16 * time.Second},
		{2500 * time.Millisecond, 18 * time.Second},
	} {
		if got := tm.AudioAtScore(tc.score); got != tc.want {
			t.Fatalf("AudioAtScore(%v) = %v, want %v", tc.score, got, tc.want)
		}
	}
	prev := time.Duration(-1)
	for s := time.Duration(0); s <= 6*time.Second; s += 50 * time.Millisecond {
		cur := tm.AudioAtScore(s)
		if cur < prev {
			t.Fatalf("non-monotonic at %v: %v < %v", s, cur, prev)
		}
		if cur < 0 {
			t.Fatalf("negative audio %v at score %v", cur, s)
		}
		prev = cur
	}
	if got := tm.AudioAtScore(-5 * time.Second); got < 0 {
		t.Fatalf("negative score gave negative audio %v", got)
	}
}

// TestWarpedLoopTimesSingleAnchorMatchesNaive ties WarpedLoopTimes to the
// naive loopStartTime regression in viewer_playback_test.go:123-127. That
// test uses two 480-tick eighth-note steps per bar at 120 BPM (0-based bar 1
// at 1s) with a 20s audio offset and expects 21s; here the same schedule with
// a single anchor at bar 1 = 21s implies the same 20s offset, so the warped
// loop start must equal the naive formula ScheduleTimeAtBar(bar 1) + 20s.
func TestWarpedLoopTimesSingleAnchorMatchesNaive(t *testing.T) {
	schedule := []PlaybackStep{
		{Bar: 0, Ticks: 480}, {Bar: 0, Ticks: 480},
		{Bar: 1, Ticks: 480}, {Bar: 1, Ticks: 480},
		{Bar: 2, Ticks: 480}, {Bar: 2, Ticks: 480},
	}
	tm := NewTimeMapper(schedule, []SyncPoint{{Bar: 1, Seconds: 21}}, 120)
	start, end := tm.WarpedLoopTimes(1, 2)
	naive := ScheduleTimeAtBar(schedule, 1, 120) + 20*time.Second
	if start != naive {
		t.Fatalf("WarpedLoopTimes start = %v, want %v (naive formula)", start, naive)
	}
	if start != 21*time.Second {
		t.Fatalf("WarpedLoopTimes start = %v, want 21s (viewer regression)", start)
	}
	if end != 22*time.Second {
		t.Fatalf("WarpedLoopTimes end = %v, want 22s", end)
	}
}

// TestSetAnchorsInvalidatesCache changes an anchor after a lookup; the next
// lookup must reflect the new anchor, not a stale memoized bar value.
func TestSetAnchorsInvalidatesCache(t *testing.T) {
	schedule := quarterBarSchedule(8)
	tm := NewTimeMapper(schedule, []SyncPoint{{Bar: 1, Seconds: 10}}, 120)
	first := tm.AudioAtScore(800 * time.Millisecond) // offset 9.5s -> 10.3s
	tm.SetAnchors([]SyncPoint{{Bar: 1, Seconds: 12}})
	second := tm.AudioAtScore(800 * time.Millisecond) // offset 11.5s -> 12.3s
	if second == first {
		t.Fatalf("SetAnchors did not invalidate the cache: %v before and after", first)
	}
	if want := secs(12.3); second != want {
		t.Fatalf("after SetAnchors AudioAtScore(800ms) = %v, want %v", second, want)
	}
}

// TestTimeMapperCacheSeededFromPos guards the Pos wiring: when an anchor
// carries a cached audio Pos, the anchored bar's audio start must be the Pos
// value (the memoized hint) even when warp would compute a different one
// from Seconds. The cache is a hint — correctness never depends on it — but
// when present it must win.
func TestTimeMapperCacheSeededFromPos(t *testing.T) {
	schedule := quarterBarSchedule(8)
	// Single anchor at bar 1 (500ms of score): warp would compute 10s
	// (Seconds); the cached Pos says 7s and must win at the bar start.
	tm := NewTimeMapper(schedule, []SyncPoint{{Bar: 1, Seconds: 10, Pos: 7}}, 120)
	if got := tm.AudioAtScore(500 * time.Millisecond); got != 7*time.Second {
		t.Fatalf("AudioAtScore at anchored bar start = %v, want 7s (seeded Pos)", got)
	}
	// Without Pos the same anchor warps to Seconds (10s): the seed is what
	// differs, not the anchor math.
	tm2 := NewTimeMapper(schedule, []SyncPoint{{Bar: 1, Seconds: 10}}, 120)
	if got := tm2.AudioAtScore(500 * time.Millisecond); got != 10*time.Second {
		t.Fatalf("AudioAtScore without Pos = %v, want 10s (warp)", got)
	}
}

// TestTimeMapperSetAnchorsClearsAndReseeds guards the SetAnchors contract:
// replacing anchors clears the old memoized entries and re-seeds the cache
// from the new anchors' Pos values.
func TestTimeMapperSetAnchorsClearsAndReseeds(t *testing.T) {
	schedule := quarterBarSchedule(8)
	tm := NewTimeMapper(schedule, []SyncPoint{{Bar: 1, Seconds: 10, Pos: 7}}, 120)
	if got := tm.AudioAtScore(500 * time.Millisecond); got != 7*time.Second {
		t.Fatalf("seeded Pos before SetAnchors: got %v, want 7s", got)
	}
	// New anchors with a new Pos must replace the seeded value.
	tm.SetAnchors([]SyncPoint{{Bar: 1, Seconds: 12, Pos: 9}})
	if got := tm.AudioAtScore(500 * time.Millisecond); got != 9*time.Second {
		t.Fatalf("after SetAnchors with new Pos: got %v, want 9s", got)
	}
	// An anchor without Pos (Pos=0) must not seed: warp computes from Seconds.
	tm.SetAnchors([]SyncPoint{{Bar: 1, Seconds: 12}})
	if got := tm.AudioAtScore(500 * time.Millisecond); got != 12*time.Second {
		t.Fatalf("after SetAnchors without Pos: got %v, want 12s (warp)", got)
	}
}

// TestScoreAtAudioRoundTrip maps audio inside a segment to score time and back.
func TestScoreAtAudioRoundTrip(t *testing.T) {
	schedule := quarterBarSchedule(8)
	points := []SyncPoint{{Bar: 1, Seconds: 10}, {Bar: 3, Seconds: 14}}
	tm := NewTimeMapper(schedule, points, 120)
	for _, tc := range []struct {
		audio float64
		want  time.Duration
	}{
		{11, 750 * time.Millisecond},  // quarter of the segment
		{13, 1250 * time.Millisecond}, // three quarters of the segment
	} {
		score, ok := tm.ScoreAtAudio(tc.audio)
		if !ok {
			t.Fatalf("ScoreAtAudio(%v) returned ok=false", tc.audio)
		}
		if d := score - tc.want; d < -2*time.Millisecond || d > 2*time.Millisecond {
			t.Fatalf("ScoreAtAudio(%v) = %v, want %v", tc.audio, score, tc.want)
		}
		got := tm.AudioAtScore(score)
		if d := got - secs(tc.audio); d < -2*time.Millisecond || d > 2*time.Millisecond {
			t.Fatalf("round trip audio %v -> score %v -> %v off by %v", tc.audio, score, got, d)
		}
	}
}

// TestResumePos returns a sane audio position for a mid-tab bar/col cursor.
func TestResumePos(t *testing.T) {
	schedule := quarterBarSchedule(8)
	// No anchors: resume is schedule@bpm + offset.
	tm := NewTimeMapper(schedule, nil, 120)
	tm.SetAudioOffset(20 * time.Second)
	if got := tm.ResumePos(2, 0); got != 21*time.Second {
		t.Fatalf("ResumePos(2,0) no anchors = %v, want 21s", got)
	}
	// Anchored: bar 2 starts at 1000ms of score, inside the 10s..14s segment.
	tm2 := NewTimeMapper(schedule, []SyncPoint{{Bar: 1, Seconds: 10}, {Bar: 3, Seconds: 14}}, 120)
	if got := tm2.ResumePos(2, 0); got != 12*time.Second {
		t.Fatalf("ResumePos(2,0) anchored = %v, want 12s", got)
	}
}

// TestTimeMapperEmptySchedule guards the degenerate cases.
func TestTimeMapperEmptySchedule(t *testing.T) {
	tm := NewTimeMapper(nil, []SyncPoint{{Bar: 0, Seconds: 5}}, 120)
	if got := tm.AudioAtScore(3 * time.Second); got != 0 {
		t.Fatalf("AudioAtScore on empty schedule = %v, want 0", got)
	}
	if score, ok := tm.ScoreAtAudio(30); ok || score != 0 {
		t.Fatalf("ScoreAtAudio on empty schedule = (%v, %v), want (0, false)", score, ok)
	}
}

// TestAudioAtScoreBeforeFirstAnchor uses the naive schedule@bpm line ahead of
// the first anchor and never emits negative audio.
func TestAudioAtScoreBeforeFirstAnchor(t *testing.T) {
	schedule := quarterBarSchedule(8)
	tm := NewTimeMapper(schedule, []SyncPoint{{Bar: 1, Seconds: 10}, {Bar: 3, Seconds: 14}}, 120)
	if got := tm.AudioAtScore(200 * time.Millisecond); got != 200*time.Millisecond {
		t.Fatalf("AudioAtScore(200ms) before first anchor = %v, want 200ms", got)
	}
	if got, ok := tm.ScoreAtAudio(2); !ok || got != 2*time.Second {
		t.Fatalf("ScoreAtAudio(2) before first anchor = (%v, %v), want (2s, true)", got, ok)
	}
	tm.SetAudioOffset(-30 * time.Second)
	if got := tm.AudioAtScore(0); got != 0 {
		t.Fatalf("negative offset must clamp to 0, got %v", got)
	}
}

// TestSingleAnchorOffsetAppliedLinearly checks that one anchor offsets the
// whole schedule uniformly.
func TestSingleAnchorOffsetAppliedLinearly(t *testing.T) {
	schedule := quarterBarSchedule(8)
	// Anchor bar 2 (1000ms of score) at 25s: offset 24s.
	tm := NewTimeMapper(schedule, []SyncPoint{{Bar: 2, Seconds: 25}}, 120)
	a0 := tm.AudioAtScore(0)
	a1 := tm.AudioAtScore(1000 * time.Millisecond)
	a2 := tm.AudioAtScore(2000 * time.Millisecond)
	if a1-a0 != 1000*time.Millisecond || a2-a1 != 1000*time.Millisecond {
		t.Fatalf("single anchor not linear: %v -> %v -> %v", a0, a1, a2)
	}
	if a1 != 25*time.Second {
		t.Fatalf("AudioAtScore(1000ms) = %v, want 25s", a1)
	}
}
