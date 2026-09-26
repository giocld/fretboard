package viewer

import (
	"testing"
	"time"
)

// An audio A-B loop must play through to B. Reading LoopRegion's first
// value (the start) as the end restarted it as soon as playback passed A.
func TestAudioLoopRunsToItsEnd(t *testing.T) {
	m := NewViewerModel()
	m.engine.SetLoop(10*time.Second, 20*time.Second)
	if m.loopPassed(15 * time.Second) {
		t.Fatal("15s is inside the 10s-20s loop; it must not restart yet")
	}
	if !m.loopPassed(20 * time.Second) {
		t.Fatal("20s is the loop end; it must restart")
	}
}
