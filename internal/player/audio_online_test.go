//go:build !noytdlp

package player

import (
	"strings"
	"testing"
	"unicode/utf8"

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

// TestSanitizeAudioFilenameRuneSafe guards byte-index truncation of long
// multibyte titles: the old name[:120] could split a UTF-8 sequence and
// produce an invalid filename for the downloaded file.
func TestSanitizeAudioFilenameRuneSafe(t *testing.T) {
	long := strings.Repeat("あ", 200) // 200 two-byte runes = 400 bytes
	got := sanitizeAudioFilename(long)
	if !utf8.ValidString(got) {
		t.Fatalf("sanitized name is invalid UTF-8: %q", got)
	}
	if got == long {
		t.Fatal("expected truncation")
	}
	if utf8.RuneCountInString(got) != 120 {
		t.Fatalf("expected 120 runes, got %d", utf8.RuneCountInString(got))
	}
}
