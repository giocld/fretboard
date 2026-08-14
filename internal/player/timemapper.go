package player

import (
	"sort"
	"time"
)

// TimeMapper is a bidirectional, tick-weighted warp between score time
// (cumulative tab time at a fixed BPM) and audio time (seconds since playback
// start). Sync anchors pin bars to audio positions; between anchors the map
// is linear in accumulated MIDI ticks, so dense bars stretch and sparse bars
// compress the same way the audio cursor does. The map is always monotonic
// non-decreasing and never teleports to 0.
type TimeMapper struct {
	schedule []PlaybackStep
	bpm      int
	anchors  []SyncPoint // sorted by Seconds (stable)
	audioOff time.Duration
	cache    map[int]time.Duration // bar -> audio time at that bar's start
}

// NewTimeMapper builds a TimeMapper over a playback schedule. points are
// 0-based bar anchors; an empty schedule or nil points are handled
// gracefully (the map degenerates to the naive schedule-at-BPM line).
func NewTimeMapper(schedule []PlaybackStep, points []SyncPoint, bpm int) *TimeMapper {
	return &TimeMapper{
		schedule: schedule,
		bpm:      bpm,
		anchors:  sortAnchors(points),
		cache:    make(map[int]time.Duration),
	}
}

// SetAnchors replaces the sync anchors (0-based bars) and invalidates the
// memoized per-bar audio cache.
func (tm *TimeMapper) SetAnchors(points []SyncPoint) {
	tm.anchors = sortAnchors(points)
	clear(tm.cache)
}

// SetAudioOffset sets the audio offset used by the naive (unanchored) region:
// audio = score + offset. Changing it invalidates the cache because cached
// bar starts may lie inside that region.
func (tm *TimeMapper) SetAudioOffset(d time.Duration) {
	tm.audioOff = d
	clear(tm.cache)
}

// AudioAtScore maps cumulative score time to audio time. With two or more
// anchors the score positions of the anchor bars are interpolated tick-
// proportionally between their audio seconds; past the last anchor the final
// segment's rate is extended. Before the first anchor (or with no anchors)
// the naive schedule-at-BPM plus audio offset applies, capped at the first
// anchor's audio so the map cannot dip back down. Single anchors offset the
// whole schedule uniformly.
func (tm *TimeMapper) AudioAtScore(scoreTime time.Duration) time.Duration {
	if len(tm.schedule) == 0 {
		return 0
	}
	step := StepIndexAtScheduleTime(tm.schedule, scoreTime, tm.bpm)
	bar := tm.schedule[step].Bar
	barStart := ScheduleTimeAtBar(tm.schedule, bar, tm.bpm)
	audioStart := tm.barAudio(bar, barStart)
	if scoreTime <= barStart {
		return audioStart
	}
	return tm.warp(scoreTime)
}

// ScoreAtAudio maps audio seconds to continuous cumulative score time,
// wrapping the audio-to-step semantics (segment selection and the final
// segment's extended rate) but returning time. The segment interpolation runs
// over the accumulated ticks the anchors span, so tick density is honored.
// It reports ok=false only for an empty schedule.
func (tm *TimeMapper) ScoreAtAudio(audioSeconds float64) (time.Duration, bool) {
	if len(tm.schedule) == 0 {
		return 0, false
	}
	total := tm.totalScore()
	anchors := tm.anchors
	if len(anchors) == 0 {
		return tm.clampScore(fromSeconds(audioSeconds)-tm.audioOff, total), true
	}
	first := anchors[0]
	if audioSeconds <= first.Seconds {
		// Before the first anchor: the naive schedule-at-BPM region.
		return tm.clampScore(fromSeconds(audioSeconds)-tm.audioOff, total), true
	}
	if len(anchors) >= 2 {
		for i := 0; i+1 < len(anchors); i++ {
			a, b := anchors[i], anchors[i+1]
			if audioSeconds < b.Seconds {
				return tm.segmentScore(a, b, audioSeconds), true
			}
		}
		a, b := anchors[len(anchors)-2], anchors[len(anchors)-1]
		return tm.clampScore(tm.extendScore(a, b, audioSeconds), total), true
	}
	// Single anchor: uniform offset for the whole schedule.
	anchor := anchors[0]
	off := fromSeconds(anchor.Seconds) - ScheduleTimeAtBar(tm.schedule, anchor.Bar, tm.bpm)
	return tm.clampScore(fromSeconds(audioSeconds)-off, total), true
}

// WarpedLoopTimes returns the audio positions of the start and end bars of a
// 0-based loop range.
func (tm *TimeMapper) WarpedLoopTimes(startBar, endBar int) (start, end time.Duration) {
	return tm.AudioAtScore(ScheduleTimeAtBar(tm.schedule, startBar, tm.bpm)),
		tm.AudioAtScore(ScheduleTimeAtBar(tm.schedule, endBar, tm.bpm))
}

// ResumePos returns the audio position to resume playback from a cursor at
// the given 0-based bar/col: the accumulated schedule time through that
// boundary, mapped through AudioAtScore.
func (tm *TimeMapper) ResumePos(bar, col int) time.Duration {
	idx := StepIndexAtPosition(tm.schedule, bar, col)
	var score time.Duration
	for i := 0; i < idx && i < len(tm.schedule); i++ {
		score += time.Duration(StepDuration(tm.schedule[i].Ticks, tm.bpm)) * time.Millisecond
	}
	return tm.AudioAtScore(score)
}

// warp maps a score position to audio using the full anchor logic. It is
// piecewise linear; every piece boundary is an anchor bar start.
func (tm *TimeMapper) warp(score time.Duration) time.Duration {
	anchors := tm.anchors
	if len(anchors) == 0 {
		return tm.naive(score, nil)
	}
	first := anchors[0]
	sFirst := ScheduleTimeAtBar(tm.schedule, first.Bar, tm.bpm)
	if len(anchors) == 1 {
		// Single anchor: schedule time at BPM plus the anchor's implied
		// offset, applied linearly to the whole schedule.
		anchor := anchors[0]
		off := fromSeconds(anchor.Seconds) - sFirst
		return clampDur(score + off)
	}
	if score < sFirst {
		return tm.naive(score, &first)
	}
	for i := 0; i+1 < len(anchors); i++ {
		a, b := anchors[i], anchors[i+1]
		sA := ScheduleTimeAtBar(tm.schedule, a.Bar, tm.bpm)
		sB := ScheduleTimeAtBar(tm.schedule, b.Bar, tm.bpm)
		if score < sB {
			return segmentAudio(a, b, sA, sB, score)
		}
	}
	a, b := anchors[len(anchors)-2], anchors[len(anchors)-1]
	sA := ScheduleTimeAtBar(tm.schedule, a.Bar, tm.bpm)
	sB := ScheduleTimeAtBar(tm.schedule, b.Bar, tm.bpm)
	return extendAudio(a, b, sA, sB, score)
}

// barAudio returns the audio time at a bar's start, memoized per bar index.
func (tm *TimeMapper) barAudio(bar int, barStart time.Duration) time.Duration {
	if v, ok := tm.cache[bar]; ok {
		return v
	}
	v := tm.warp(barStart)
	tm.cache[bar] = v
	return v
}

// naive is the unanchored mapping: score at BPM plus the audio offset. When
// a first anchor exists the result is capped at its audio time so the map
// stays non-decreasing across the boundary.
func (tm *TimeMapper) naive(score time.Duration, first *SyncPoint) time.Duration {
	d := clampDur(tm.audioOff + score)
	if first != nil {
		if cap := fromSeconds(first.Seconds); d > cap {
			d = cap
		}
	}
	return d
}

// segmentAudio interpolates score tick-proportionally inside a segment: the
// fraction of score elapsed between the anchors is applied to their audio
// span, mirroring segmentStep.
func segmentAudio(a, b SyncPoint, sA, sB, score time.Duration) time.Duration {
	spanSec := (b.Seconds - a.Seconds) * 1e9
	spanScore := float64(sB - sA)
	if spanSec <= 0 {
		return fromSeconds(a.Seconds)
	}
	if spanScore <= 0 {
		return fromSeconds(b.Seconds)
	}
	f := float64(score-sA) / spanScore
	return fromSeconds(a.Seconds) + time.Duration(f*spanSec)
}

// extendAudio extends the final segment's audio-per-score rate past the last
// anchor, the inverse spirit of extendLastSegment.
func extendAudio(a, b SyncPoint, sA, sB, score time.Duration) time.Duration {
	spanSec := (b.Seconds - a.Seconds) * 1e9
	spanScore := float64(sB - sA)
	if spanSec <= 0 || spanScore <= 0 {
		return fromSeconds(b.Seconds)
	}
	rate := spanSec / spanScore
	return fromSeconds(b.Seconds) + time.Duration(float64(score-sB)*rate)
}

// segmentScore inverts segmentAudio: audio back to score at the same rate.
func (tm *TimeMapper) segmentScore(a, b SyncPoint, audioSeconds float64) time.Duration {
	sA := ScheduleTimeAtBar(tm.schedule, a.Bar, tm.bpm)
	sB := ScheduleTimeAtBar(tm.schedule, b.Bar, tm.bpm)
	spanSec := (b.Seconds - a.Seconds) * 1e9
	spanScore := float64(sB - sA)
	if spanSec <= 0 {
		return sA
	}
	if spanScore <= 0 {
		return sB
	}
	f := (audioSeconds - a.Seconds) / (b.Seconds - a.Seconds)
	return sA + time.Duration(f*spanScore)
}

// extendScore inverts extendAudio: audio past the last anchor back to score.
func (tm *TimeMapper) extendScore(a, b SyncPoint, audioSeconds float64) time.Duration {
	sA := ScheduleTimeAtBar(tm.schedule, a.Bar, tm.bpm)
	sB := ScheduleTimeAtBar(tm.schedule, b.Bar, tm.bpm)
	spanSec := (b.Seconds - a.Seconds) * 1e9
	spanScore := float64(sB - sA)
	if spanSec <= 0 || spanScore <= 0 {
		return sB
	}
	rate := spanScore / spanSec
	score := sB + time.Duration((audioSeconds-b.Seconds)*1e9*rate)
	if score < sB {
		score = sB
	}
	return score
}

// totalScore is the whole schedule's duration at the mapping BPM.
func (tm *TimeMapper) totalScore() time.Duration {
	var total time.Duration
	for _, s := range tm.schedule {
		total += time.Duration(StepDuration(s.Ticks, tm.bpm)) * time.Millisecond
	}
	return total
}

// clampScore clamps a score time to [0, total].
func (tm *TimeMapper) clampScore(d, total time.Duration) time.Duration {
	if d < 0 {
		return 0
	}
	if d > total {
		return total
	}
	return d
}

// sortAnchors returns anchors sorted by Seconds, keeping bar order for ties.
func sortAnchors(points []SyncPoint) []SyncPoint {
	if len(points) == 0 {
		return nil
	}
	out := append([]SyncPoint(nil), points...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Seconds < out[j].Seconds })
	return out
}

// fromSeconds converts float seconds to a time.Duration.
func fromSeconds(s float64) time.Duration {
	return time.Duration(s * float64(time.Second))
}

// clampDur clamps a duration to be non-negative.
func clampDur(d time.Duration) time.Duration {
	if d < 0 {
		return 0
	}
	return d
}
