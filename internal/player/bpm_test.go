package player

import (
	"testing"

	"fretboard/internal/model"
)

func TestTabBPMFromMetadata(t *testing.T) {
	tab := &model.Tab{Metadata: map[string]string{model.MetaKeyBPM: "112"}}
	if got := TabBPM(tab); got != 112 {
		t.Fatalf("TabBPM = %d, want 112", got)
	}
}

func TestTabBPMFromTempo(t *testing.T) {
	tab := &model.Tab{Metadata: map[string]string{model.MetaKeyTempo: "96"}}
	if got := TabBPM(tab); got != 96 {
		t.Fatalf("TabBPM = %d, want 96", got)
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
