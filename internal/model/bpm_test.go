package model

import "testing"

func TestParseBPMFromText(t *testing.T) {
	if got := ParseBPMFromText("Based on the video. 107 bpm."); got != 107 {
		t.Fatalf("ParseBPMFromText = %d, want 107", got)
	}
}
