package player

import (
	"strings"
	"testing"

	"fretboard/internal/model"
)

func TestAudioSearchQuery(t *testing.T) {
	tab := &model.Tab{Title: "Layla", Artist: "Eric Clapton"}
	if got := AudioSearchQuery(tab); got != "Eric Clapton Layla" {
		t.Fatalf("AudioSearchQuery = %q", got)
	}
}

func TestSanitizeAudioFilename(t *testing.T) {
	got := sanitizeAudioFilename(`AC/DC: Back In Black?`)
	if got == "" || strings.Contains(got, "/") || strings.Contains(got, "?") {
		t.Fatalf("sanitizeAudioFilename = %q", got)
	}
}
