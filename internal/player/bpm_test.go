package player

import (
	"testing"

	"github.com/YOUR_USERNAME/fretboard/internal/model"
)

func TestTabBPMFromMetadata(t *testing.T) {
	tab := &model.Tab{Metadata: map[string]string{"bpm": "112"}}
	if got := TabBPM(tab); got != 112 {
		t.Fatalf("TabBPM = %d, want 112", got)
	}
}

func TestTabBPMFromTempo(t *testing.T) {
	tab := &model.Tab{Metadata: map[string]string{"tempo": "96"}}
	if got := TabBPM(tab); got != 96 {
		t.Fatalf("TabBPM = %d, want 96", got)
	}
}

func TestParseBPMFromText(t *testing.T) {
	if got := ParseBPMFromText("Based on the video. 107 bpm."); got != 107 {
		t.Fatalf("ParseBPMFromText = %d, want 107", got)
	}
}

func TestClampBPM(t *testing.T) {
	if ClampBPM(10) != 40 {
		t.Fatalf("ClampBPM(10) = %d", ClampBPM(10))
	}
	if ClampBPM(400) != 300 {
		t.Fatalf("ClampBPM(400) = %d", ClampBPM(400))
	}
}
