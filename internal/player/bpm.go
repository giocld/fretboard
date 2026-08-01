package player

import (
	"strconv"
	"strings"

	"github.com/YOUR_USERNAME/fretboard/internal/model"
)

// DefaultBPM is the tempo used when none can be determined.
const DefaultBPM = model.DefaultBPM

// TabBPM returns the best-known tempo for a tab in beats per minute.
func TabBPM(tab *model.Tab) int {
	if tab == nil || tab.Metadata == nil {
		return DefaultBPM
	}
	for _, key := range []string{model.MetaKeyBPM, model.MetaKeyTempo} {
		if s := strings.TrimSpace(tab.Metadata[key]); s != "" {
			if n := parseBPMValue(s); n > 0 {
				return ClampBPM(n)
			}
		}
	}
	return DefaultBPM
}

func parseBPMValue(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if n, err := strconv.Atoi(strings.Fields(s)[0]); err == nil {
		return n
	}
	return model.ParseBPMFromText(s)
}

// ClampBPM keeps tempo in a sensible guitar-tab range.
func ClampBPM(n int) int {
	return model.ClampBPM(n)
}
