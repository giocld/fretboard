package player

import (
	"fmt"
	"math"
	"sort"
	"time"

	"fretboard/internal/model"
)

// Ranked alignment: the same analysis AlignAudio runs, but keeping the
// top-N distinct (bpm, offset) hypotheses instead of a single best, each
// decorated for the two-tier gate. The gate is rank-and-present, never a
// hard reject: candidates land in one of three bands (auto / present /
// reject) and the viewer applies the auto band, presents the top-3 for the
// present band, and only ever hints at the reject band.

// Alignment bands returned by ClassifyBand.
const (
	BandAuto    = "auto"    // confident + well covered: apply without asking
	BandPresent = "present" // plausible: present the top-3 for confirmation
	BandReject  = "reject"  // too weak: never auto-apply, never silent
)

// rankTopN is how many distinct alignment hypotheses RankAlignments keeps.
const rankTopN = 3

// maxRefineScan bounds how many grid hypotheses run through the offset
// refinement passes while collecting the distinct top-N.
const maxRefineScan = 200

// OffsetVariant is an alternative tab-start offset for a candidate, labelled
// for the confirm overlay (half a beat or one bar early/late).
type OffsetVariant struct {
	Label  string
	Offset time.Duration
}

// Candidate is one ranked alignment hypothesis plus the metadata the
// two-tier gate needs.
type Candidate struct {
	Alignment    Alignment
	Coverage     float64 // fraction of expected onsets matched at the strict 60ms tolerance
	IdentityZone float64 // candidate BPM / duration-derived BPM (0 when unknown)
	Variants     []OffsetVariant
	Partial      bool          // coverage < 0.5: never auto-applied
	TempoDelta   string        // warning when the tab BPM and the audio BPM differ
	barLen       time.Duration // tick-derived bar length at the candidate BPM
}

// variants returns the +- half-beat and +- one-bar offset alternatives of
// the candidate's offset. Half-beat is beat/2 at the candidate BPM; the
// one-bar length is the schedule's own bar duration when available, else the
// 4/4 fallback of 4 beats.
func (c Candidate) variants() []OffsetVariant {
	if c.Alignment.BPM <= 0 {
		return nil
	}
	beat := time.Duration(60000/c.Alignment.BPM) * time.Millisecond
	bar := c.barLen
	if bar <= 0 {
		bar = 4 * beat
	}
	half := beat / 2
	return []OffsetVariant{
		{Label: "half beat early", Offset: maxDur(0, c.Alignment.Offset-half)},
		{Label: "half beat late", Offset: c.Alignment.Offset + half},
		{Label: "one bar early", Offset: maxDur(0, c.Alignment.Offset-bar)},
		{Label: "one bar late", Offset: c.Alignment.Offset + bar},
	}
}

// RankAlignments runs the same analysis as AlignAudio but keeps the top-3
// distinct (bpm, offset) hypotheses by score, each with its own alignment
// (onsets, strengths, strict confidence) and the gate metadata: coverage,
// identity zone, partial flag, tempo-delta warning, and offset variants. An
// analysis failure is returned as an error; a usable-but-weak analysis
// returns no candidates (nil, nil).
func RankAlignments(tab *model.Tab, path string, hint time.Duration) ([]Candidate, error) {
	in, ok, err := prepareAlignment(tab, path)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	finals := in.rankSearch(hint, rankTopN)
	if len(finals) == 0 {
		return nil, nil
	}
	schedule := BuildSchedule(tab)
	// Identity BPM: the tempo that maps the whole tab schedule onto the
	// recording's duration, grounded in the download gate's file probe.
	// Zero when the duration cannot be probed (no ffprobe) — no downgrade.
	identityBPM := 0
	if dur, perr := ProbeDuration(path); perr == nil && dur > 0 {
		identityBPM = DeriveBPMFromAudio(schedule, dur, hint)
	}
	barLen := barLengthAt(schedule, finals[0].BPM)
	out := make([]Candidate, 0, len(finals))
	for _, a := range finals {
		scale := float64(in.baseBPM) / float64(a.BPM)
		cov := verifyStrict(in.expected, in.onsets, scale, a.Offset)
		c := Candidate{Alignment: a, Coverage: cov, Partial: cov < 0.5}
		if identityBPM > 0 {
			c.IdentityZone = float64(a.BPM) / float64(identityBPM)
		}
		c.TempoDelta = tempoDeltaString(in.baseBPM, a.BPM)
		c.barLen = barLen
		c.Variants = c.variants()
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Alignment.Score > out[j].Alignment.Score })
	return out, nil
}

// rankSearch returns the top n distinct (bpm, offset) alignment hypotheses
// by grid score, each run through the near-tie and refinement passes.
func (in alignInput) rankSearch(hint time.Duration, n int) []Alignment {
	hyps := in.gridSearch(hint)
	if len(hyps) == 0 {
		return nil
	}
	sort.SliceStable(hyps, func(i, j int) bool { return hyps[i].score > hyps[j].score })
	if len(hyps) > maxRefineScan {
		hyps = hyps[:maxRefineScan]
	}
	seen := map[alignKey]bool{}
	var out []Alignment
	for _, h := range hyps {
		cand := in.refineHypothesis(h, hint)
		k := alignKey{bpm: cand.BPM, offset: cand.Offset}
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, cand)
		if len(out) >= n {
			break
		}
	}
	return out
}

// ClassifyBand maps a candidate's confidence, coverage, and identity zone to
// an alignment band and a short reason. Confidence alone picks the base band;
// low coverage (partial match) blocks auto-apply; an out-of-range identity
// zone downgrades one band but never pushes a presentable result into reject
// — the gate ranks and presents, it never hard-rejects.
func ClassifyBand(conf, cov, identityZone float64) (band, reason string) {
	switch {
	case conf >= 0.6:
		band = BandAuto
	case conf >= 0.4:
		band = BandPresent
	default:
		band = BandReject
	}
	if cov < 0.5 {
		if band == BandAuto {
			band = BandPresent
			reason = "partial coverage blocks auto-apply"
		}
	}
	if identityZone != 0 && (identityZone < 0.7 || identityZone > 1.3) {
		switch band {
		case BandAuto:
			band = BandPresent
			reason = "identity-zone mismatch downgrades to present"
		case BandPresent:
			reason = "identity-zone mismatch (downgrade capped at present)"
		}
	}
	return band, reason
}

// tempoDeltaString returns a warning describing the tab-vs-audio BPM gap, or
// "" when the tempos agree (within 2%) or cannot be compared. The warning is
// informational only — it never gates the alignment.
func tempoDeltaString(tabBPM, audioBPM int) string {
	if tabBPM <= 0 || audioBPM <= 0 {
		return ""
	}
	if math.Abs(float64(audioBPM-tabBPM))/float64(tabBPM) <= 0.02 {
		return ""
	}
	return fmt.Sprintf("tab %d BPM vs audio %d BPM", tabBPM, audioBPM)
}

// barLengthAt returns the duration of the first schedule bar at the given
// BPM, from the schedule's own tick content (0 when it cannot be derived).
func barLengthAt(schedule []PlaybackStep, bpm int) time.Duration {
	if len(schedule) == 0 || bpm <= 0 {
		return 0
	}
	bar := schedule[0].Bar
	var ticks int64
	for _, s := range schedule {
		if s.Bar == bar {
			ticks += int64(s.Ticks)
		}
	}
	if ticks <= 0 {
		return 0
	}
	return time.Duration(TicksToSeconds(ticks, bpm) * float64(time.Second))
}

func maxDur(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}
