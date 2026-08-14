package player

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestClassifyBand guards the two-tier gate: confidence picks the base band,
// partial coverage blocks auto-apply, and an out-of-range identity zone
// downgrades one band but never hard-rejects a presentable result — the gate
// is rank-and-present, never a hard reject.
func TestClassifyBand(t *testing.T) {
	cases := []struct {
		name       string
		conf, cov  float64
		zone       float64
		want       string
		wantReason bool
	}{
		{"confident", 0.65, 1.0, 1.0, BandAuto, false},
		{"weak-present", 0.5, 1.0, 1.0, BandPresent, false},
		{"low-reject", 0.3, 1.0, 1.0, BandReject, false},
		{"partial-blocks-auto", 0.8, 0.4, 1.0, BandPresent, true},
		{"identity-downgrade-capped-present", 0.55, 1.0, 1.5, BandPresent, true},
		{"identity-downgrades-auto", 0.65, 1.0, 1.5, BandPresent, true},
		{"identity-low-downgrades-auto", 0.7, 1.0, 0.5, BandPresent, true},
		{"reject-stays-reject", 0.3, 1.0, 0.5, BandReject, false},
		{"no-identity-bpm-no-downgrade", 0.65, 1.0, 0, BandAuto, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			band, reason := ClassifyBand(tc.conf, tc.cov, tc.zone)
			if band != tc.want {
				t.Fatalf("ClassifyBand(%.2f, %.2f, %.2f) = %q, want %q", tc.conf, tc.cov, tc.zone, band, tc.want)
			}
			if (reason != "") != tc.wantReason {
				t.Fatalf("ClassifyBand(%.2f, %.2f, %.2f) reason = %q, wantReason %v", tc.conf, tc.cov, tc.zone, reason, tc.wantReason)
			}
		})
	}
}

// TestPartialGuardBlocksAutoApply guards the coverage gate: a candidate that
// matches under half of the expected onsets is tagged Partial and never
// auto-applies, no matter how confident the raw score looks.
func TestPartialGuardBlocksAutoApply(t *testing.T) {
	for _, conf := range []float64{0.6, 0.7, 0.8, 0.95} {
		if band, _ := ClassifyBand(conf, 0.4, 1.0); band == BandAuto {
			t.Fatalf("conf %.2f with 40%% coverage must not auto-apply", conf)
		}
	}
	band, reason := ClassifyBand(0.8, 0.4, 1.0)
	if band != BandPresent || !strings.Contains(reason, "partial") {
		t.Fatalf("partial candidate = %q %q, want present with a partial reason", band, reason)
	}
}

// TestCandidateVariants guards the +- half-beat and +- one-bar offset
// variants: half-beat is beat/2 at the candidate BPM and the one-bar length
// falls back to 4 beats when no tick-derived bar length is available.
func TestCandidateVariants(t *testing.T) {
	c := Candidate{Alignment: Alignment{BPM: 120, Offset: 3200 * time.Millisecond}}
	vs := c.variants()
	want := []struct {
		label  string
		offset time.Duration
	}{
		{"half beat early", 2950 * time.Millisecond},
		{"half beat late", 3450 * time.Millisecond},
		{"one bar early", 1200 * time.Millisecond},
		{"one bar late", 5200 * time.Millisecond},
	}
	if len(vs) != len(want) {
		t.Fatalf("variants = %d, want %d", len(vs), len(want))
	}
	for i, w := range want {
		if vs[i].Label != w.label || vs[i].Offset != w.offset {
			t.Fatalf("variant %d = %+v, want %+v", i, vs[i], w)
		}
	}
	// A tick-derived bar length overrides the 4-beat fallback.
	c2 := Candidate{Alignment: Alignment{BPM: 120, Offset: 3200 * time.Millisecond}, barLen: 3 * time.Second}
	vs2 := c2.variants()
	if vs2[2].Offset != 200*time.Millisecond || vs2[3].Offset != 6200*time.Millisecond {
		t.Fatalf("tick-derived bar variants wrong: %+v", vs2)
	}
}

// TestTempoDeltaWarningNotGate guards the demoted tempo gate: a large
// tab-vs-audio BPM gap produces a warning string but never blocks the band
// classification.
func TestTempoDeltaWarningNotGate(t *testing.T) {
	if got := tempoDeltaString(120, 96); got == "" || !strings.Contains(got, "120") || !strings.Contains(got, "96") {
		t.Fatalf("large delta must warn, got %q", got)
	}
	if got := tempoDeltaString(120, 118); got != "" {
		t.Fatalf("2%% delta must not warn, got %q", got)
	}
	if got := tempoDeltaString(120, 120); got != "" {
		t.Fatalf("equal tempo must not warn, got %q", got)
	}
	if got := tempoDeltaString(0, 96); got != "" {
		t.Fatalf("unknown tab tempo must not warn, got %q", got)
	}
	// The warning is informational: a confident candidate still auto-applies.
	if band, _ := ClassifyBand(0.7, 1.0, 1.0); band != BandAuto {
		t.Fatalf("tempo delta must never be a gate, band = %q", band)
	}
}

// TestRankAlignmentsTop3 guards the ranked output on the standard synthetic
// recording: the top candidate recovers ~118 BPM, covers the expected span
// well, carries the 4 offset variants, and the list is capped at top-3
// distinct (bpm, offset) hypotheses ordered best-first.
func TestRankAlignmentsTop3(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	path := filepath.Join(t.TempDir(), "song.wav")
	bpm := 118
	beat := time.Duration(60000/bpm/2) * time.Millisecond
	intro := 3200 * time.Millisecond
	var clicks []time.Duration
	for i := 0; i < 480; i++ {
		clicks = append(clicks, intro+time.Duration(i)*beat)
	}
	if err := writeSyntheticWAVAlt(path, 8000, clicks, 30*time.Millisecond, 2); err != nil {
		t.Fatal(err)
	}

	cands, err := RankAlignments(testTab(), path, intro)
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) == 0 {
		t.Fatal("RankAlignments returned no candidates")
	}
	if len(cands) > 3 {
		t.Fatalf("RankAlignments must cap at top-3, got %d", len(cands))
	}
	top := cands[0]
	if top.Alignment.BPM < 117 || top.Alignment.BPM > 119 {
		t.Fatalf("top BPM = %d, want ~118", top.Alignment.BPM)
	}
	if top.Coverage < 0.5 {
		t.Fatalf("top coverage = %.2f, want >= 0.5", top.Coverage)
	}
	if top.Partial {
		t.Fatal("a well-covered alignment must not be tagged partial")
	}
	if len(top.Variants) != 4 {
		t.Fatalf("top variants = %d, want 4", len(top.Variants))
	}
	// Ranked best-first.
	for i := 1; i < len(cands); i++ {
		if cands[i].Alignment.Score > cands[i-1].Alignment.Score {
			t.Fatalf("candidates out of order at %d: %f > %f", i, cands[i].Alignment.Score, cands[i-1].Alignment.Score)
		}
	}
	// Distinct (bpm, offset) pairs.
	seen := map[[2]interface{}]bool{}
	for _, c := range cands {
		k := [2]interface{}{c.Alignment.BPM, c.Alignment.Offset}
		if seen[k] {
			t.Fatalf("duplicate candidate: %+v", c.Alignment)
		}
		seen[k] = true
	}
	// When the duration-derived identity BPM disagrees, the candidate must
	// be downgraded away from auto — never silently auto-applied.
	if top.IdentityZone != 0 && (top.IdentityZone < 0.7 || top.IdentityZone > 1.3) {
		if band, _ := ClassifyBand(top.Alignment.Confidence, top.Coverage, top.IdentityZone); band == BandAuto {
			t.Fatalf("out-of-range identity zone %.2f must not auto-apply", top.IdentityZone)
		}
	}
}
