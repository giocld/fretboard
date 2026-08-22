package player

import (
	"sort"
	"sync"
	"time"
)

// VerifyState is the calibration verification state of a playback session:
// whether the sync anchors have been accepted, are still being checked, or
// were found wanting.
type VerifyState int

const (
	// VerifyUnverified means no usable calibration is in force — either no
	// anchors exist yet or a mismatch downgraded the session.
	VerifyUnverified VerifyState = iota
	// VerifyPending means verification is in progress and awaiting either a
	// user decision or the passive auto-keep timeout.
	VerifyPending
	// VerifyKept means the anchors were accepted (user or timeout) and stay
	// in force.
	VerifyKept
	// VerifyRefined means the anchors were suspect and re-derivation of the
	// alignment is in progress.
	VerifyRefined
)

// String returns the short label for a state, suitable for status display.
func (s VerifyState) String() string {
	switch s {
	case VerifyPending:
		return "pending"
	case VerifyKept:
		return "verified"
	case VerifyRefined:
		return "refined"
	default:
		return "uncalibrated"
	}
}

// VerifySession tracks the non-blocking verification of sync anchors. State
// transitions are pure — no timers, no I/O; the caller drives time by passing
// `now` to AutoKeepIfElapsed — and guarded by a mutex so the audio callback
// and the UI loop may both touch the same session. The passive auto-keep
// timeout exists so playback never blocks waiting for a decision.
type VerifySession struct {
	mu          sync.Mutex
	State       VerifyState
	Drift       time.Duration // measured drift since the last calibration
	AnchorCount int           // how many sync anchors the session covers
	AutoKeepAt  time.Time     // zero => auto-keep disabled
}

// NewVerifySession starts verification in the pending state for a playback
// session with the given number of anchors and measured drift. autoKeepAfter
// is the passive timeout before the anchors are auto-accepted; non-positive
// disables the timeout. The deadline is anchored to creation time; callers
// that need deterministic deadlines can set AutoKeepAt afterwards.
func NewVerifySession(anchors int, drift time.Duration, autoKeepAfter time.Duration) *VerifySession {
	s := &VerifySession{
		State:       VerifyPending,
		Drift:       drift,
		AnchorCount: anchors,
	}
	if autoKeepAfter > 0 {
		s.AutoKeepAt = time.Now().Add(autoKeepAfter)
	}
	return s
}

// Keep accepts the current anchors as verified.
func (v *VerifySession) Keep() {
	v.mu.Lock()
	v.State = VerifyKept
	v.mu.Unlock()
}

// Refine marks the anchors suspect and moves to the refined state, signalling
// that the alignment should be re-derived; the caller completes the
// re-derivation and decides whether to keep the result or downgrade.
func (v *VerifySession) Refine() {
	v.mu.Lock()
	v.State = VerifyRefined
	v.mu.Unlock()
}

// AutoKeepIfElapsed returns the state and, once now has passed AutoKeepAt,
// transitions a pending session to kept — the passive timeout so playback
// never blocks on a verification dialog. Non-pending sessions (already kept,
// refined, or downgraded) and sessions without a scheduled deadline are left
// untouched.
func (v *VerifySession) AutoKeepIfElapsed(now time.Time) VerifyState {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.State == VerifyPending && !v.AutoKeepAt.IsZero() && !now.Before(v.AutoKeepAt) {
		v.State = VerifyKept
	}
	return v.State
}

// Downgrade returns the session to the unverified state when a mismatch
// between expected and detected onsets is found; mismatch=false leaves the
// state unchanged.
func (v *VerifySession) Downgrade(mismatch bool) {
	if !mismatch {
		return
	}
	v.mu.Lock()
	v.State = VerifyUnverified
	v.mu.Unlock()
}

// AlignmentMismatch reports whether more than half of the expected onsets
// have no detected onset within tolerance — the signal that the current
// calibration is drifting and verification should downgrade. Both slices are
// matched in time order, so they need not be sorted beforehand. An empty
// expected set never reports a mismatch; a tolerance <= 0 admits only exact
// matches. A detected onset may satisfy several expected onsets (the spec
// asks per expected onset whether any detection covers it).
func AlignmentMismatch(expectedOnsets, detected []time.Duration, tolerance time.Duration) bool {
	if len(expectedOnsets) == 0 {
		return false
	}
	exp := append([]time.Duration(nil), expectedOnsets...)
	det := append([]time.Duration(nil), detected...)
	sort.Slice(exp, func(i, j int) bool { return exp[i] < exp[j] })
	sort.Slice(det, func(i, j int) bool { return det[i] < det[j] })
	matched := 0
	j := 0
	for _, e := range exp {
		for j < len(det) && det[j] < e-tolerance {
			j++
		}
		if j < len(det) && det[j] <= e+tolerance {
			matched++
		}
	}
	return float64(len(exp)-matched)/float64(len(exp)) > 0.5
}
