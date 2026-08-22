package player

import (
	"testing"
	"time"
)

func TestVerifyStateStrings(t *testing.T) {
	for _, tc := range []struct {
		state VerifyState
		want  string
	}{
		{VerifyUnverified, "uncalibrated"},
		{VerifyPending, "pending"},
		{VerifyKept, "verified"},
		{VerifyRefined, "refined"},
	} {
		if got := tc.state.String(); got != tc.want {
			t.Fatalf("%d String() = %q, want %q", tc.state, got, tc.want)
		}
	}
}

func TestNewVerifySession(t *testing.T) {
	s := NewVerifySession(4, 120*time.Millisecond, 30*time.Second)
	if s.State != VerifyPending {
		t.Fatalf("new session state = %v, want pending", s.State)
	}
	if s.Drift != 120*time.Millisecond || s.AnchorCount != 4 {
		t.Fatalf("new session drift/count = %v/%d, want 120ms/4", s.Drift, s.AnchorCount)
	}
	if s.AutoKeepAt.IsZero() {
		t.Fatal("positive autoKeepAfter must schedule a deadline")
	}
	if got := NewVerifySession(1, 0, 0).AutoKeepAt; !got.IsZero() {
		t.Fatalf("non-positive autoKeepAfter must disable auto-keep, got %v", got)
	}
}

func TestVerifySessionKeepAndRefine(t *testing.T) {
	s := NewVerifySession(3, 0, 30*time.Second)
	s.Keep()
	if s.State != VerifyKept || s.State.String() != "verified" {
		t.Fatalf("Keep: state = %v (%s), want kept/verified", s.State, s.State)
	}
	s.Refine()
	if s.State != VerifyRefined || s.State.String() != "refined" {
		t.Fatalf("Refine from kept: state = %v, want refined", s.State)
	}
	s.Keep() // idempotent from any state
	if s.State != VerifyKept {
		t.Fatalf("Keep after refine: state = %v, want kept", s.State)
	}
}

func TestVerifySessionAutoKeep(t *testing.T) {
	base := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	deadline := base.Add(30 * time.Second)
	s := NewVerifySession(2, 0, 30*time.Second)
	s.AutoKeepAt = deadline // deterministic deadline; the constructor anchors to time.Now
	if got := s.AutoKeepIfElapsed(base); got != VerifyPending {
		t.Fatalf("before deadline: state = %v, want pending", got)
	}
	if got := s.AutoKeepIfElapsed(deadline); got != VerifyKept {
		t.Fatalf("at deadline: state = %v, want kept", got)
	}
	if got := s.AutoKeepIfElapsed(base.Add(time.Minute)); got != VerifyKept {
		t.Fatalf("after deadline: state = %v, want kept (idempotent)", got)
	}
}

func TestVerifySessionAutoKeepDisabledAndNonPending(t *testing.T) {
	base := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	disabled := NewVerifySession(1, 0, 0) // zero AutoKeepAt: never auto-kept
	if got := disabled.AutoKeepIfElapsed(base.Add(time.Hour)); got != VerifyPending {
		t.Fatalf("disabled auto-keep fired: %v", got)
	}
	// Refined sessions are not auto-kept; the caller owns re-derivation.
	refined := NewVerifySession(1, 0, 30*time.Second)
	refined.AutoKeepAt = base.Add(30 * time.Second)
	refined.Refine()
	if got := refined.AutoKeepIfElapsed(base.Add(time.Hour)); got != VerifyRefined {
		t.Fatalf("auto-keep must not fire on a refined session, got %v", got)
	}
}

func TestVerifySessionDowngrade(t *testing.T) {
	s := NewVerifySession(2, 0, 30*time.Second)
	s.Downgrade(false)
	if s.State != VerifyPending {
		t.Fatalf("Downgrade(false) must not change state, got %v", s.State)
	}
	s.Downgrade(true)
	if s.State != VerifyUnverified || s.State.String() != "uncalibrated" {
		t.Fatalf("Downgrade(true) = %v (%s), want unverified/uncalibrated", s.State, s.State)
	}
}

func TestVerifySessionFlow(t *testing.T) {
	// Full lifecycle: pending -> auto-kept -> refined -> downgraded by a
	// fresh mismatch, ending uncalibrated.
	base := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	s := NewVerifySession(3, 10*time.Millisecond, 30*time.Second)
	s.AutoKeepAt = base.Add(30 * time.Second)
	if got := s.AutoKeepIfElapsed(base.Add(31 * time.Second)); got != VerifyKept {
		t.Fatalf("step 1: %v, want kept", got)
	}
	s.Refine()
	if s.State != VerifyRefined {
		t.Fatalf("step 2: %v, want refined", s.State)
	}
	s.Downgrade(true)
	if s.State != VerifyUnverified {
		t.Fatalf("step 3: %v, want unverified", s.State)
	}
}

func TestAlignmentMismatch(t *testing.T) {
	expected := []time.Duration{1 * time.Second, 2 * time.Second, 3 * time.Second, 4 * time.Second}
	onTarget := []time.Duration{1 * time.Second, 2 * time.Second, 3 * time.Second, 4 * time.Second}
	if AlignmentMismatch(expected, onTarget, 150*time.Millisecond) {
		t.Fatal("perfectly matched onsets must not signal a mismatch")
	}
	jittered := []time.Duration{1050 * time.Millisecond, 2050 * time.Millisecond, 3*time.Second + 20*time.Millisecond, 4 * time.Second}
	if AlignmentMismatch(expected, jittered, 150*time.Millisecond) {
		t.Fatal("jittered but within-tolerance onsets must not signal a mismatch")
	}
	// Two of four matched is exactly 0.5 unmatched — not more than half.
	half := []time.Duration{1 * time.Second, 2 * time.Second}
	if AlignmentMismatch(expected, half, 100*time.Millisecond) {
		t.Fatal("exactly half matched must not signal a mismatch (0.5 is not > 0.5)")
	}
	// One of four matched: 75% unmatched.
	one := []time.Duration{1 * time.Second}
	if !AlignmentMismatch(expected, one, 100*time.Millisecond) {
		t.Fatal("75%% unmatched must signal a mismatch")
	}
	if !AlignmentMismatch(expected, nil, 100*time.Millisecond) {
		t.Fatal("no detections must signal a mismatch")
	}
	if AlignmentMismatch(nil, []time.Duration{1 * time.Second}, 100*time.Millisecond) {
		t.Fatal("empty expected set must never signal a mismatch")
	}
}

func TestAlignmentMismatchToleranceAndOrder(t *testing.T) {
	// A detected onset exactly at the tolerance edge still counts as matched.
	if AlignmentMismatch([]time.Duration{1 * time.Second}, []time.Duration{1200 * time.Millisecond}, 200*time.Millisecond) {
		t.Fatal("onset at exactly tolerance must count as matched")
	}
	if !AlignmentMismatch([]time.Duration{1 * time.Second}, []time.Duration{1200 * time.Millisecond}, 199*time.Millisecond) {
		t.Fatal("onset just past tolerance must count as unmatched")
	}
	// A negative tolerance admits only exact matches: 1s vs 1.01s unmatched.
	if !AlignmentMismatch([]time.Duration{1 * time.Second}, []time.Duration{1010 * time.Millisecond}, -1) {
		t.Fatal("negative tolerance must admit only exact matches")
	}
	// Both sides are sorted internally, so scrambled input behaves the same.
	scrambledExp := []time.Duration{3 * time.Second, 1 * time.Second, 2 * time.Second}
	scrambledDet := []time.Duration{2 * time.Second, 3 * time.Second, 1 * time.Second}
	if AlignmentMismatch(scrambledExp, scrambledDet, 50*time.Millisecond) {
		t.Fatal("scrambled but matched onsets must not signal a mismatch")
	}
}
