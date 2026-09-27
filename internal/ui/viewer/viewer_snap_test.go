package viewer

import "testing"

// boundedAudioSnap keeps live onset correction from showing a future note
// before the player reaches it or rewinding a note already displayed.
func TestBoundedAudioSnap(t *testing.T) {
	for _, tc := range []struct {
		current, mapped, snapped, want int
	}{
		{0, 0, 1, 0}, // future onset: don't skip ahead
		{1, 2, 0, 2}, // stale past onset: don't rewind
		{1, 2, 1, 1}, // a modest, already-heard correction is allowed
		{3, 0, 1, 0}, // seek/loop wrap follows the audio clock
	} {
		if got := boundedAudioSnap(tc.current, tc.mapped, tc.snapped); got != tc.want {
			t.Errorf("current=%d mapped=%d snapped=%d: got %d, want %d", tc.current, tc.mapped, tc.snapped, got, tc.want)
		}
	}
}
