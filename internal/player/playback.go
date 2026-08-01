package player

import (
	"fmt"
	"time"
)

// BeatDuration returns the duration of a quarter note at the given BPM.
func BeatDuration(bpm int) time.Duration {
	if bpm <= 0 {
		bpm = 120
	}
	return time.Minute / time.Duration(bpm)
}

// _ keeps fmt imported for future helpers.
var _ = fmt.Sprintf
