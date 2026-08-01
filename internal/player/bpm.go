package player

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/YOUR_USERNAME/fretboard/internal/model"
)

var bpmInTextRegex = regexp.MustCompile(`(?i)(?:^|[^\d])(\d{2,3})\s*bpm`)

// TabBPM returns the best-known tempo for a tab in beats per minute.
func TabBPM(tab *model.Tab) int {
	if tab == nil || tab.Metadata == nil {
		return DefaultBPM
	}
	for _, key := range []string{"bpm", "tempo"} {
		if s := strings.TrimSpace(tab.Metadata[key]); s != "" {
			if n := parseBPMValue(s); n > 0 {
				return ClampBPM(n)
			}
		}
	}
	return DefaultBPM
}

// ParseBPMFromText extracts a tempo from free text such as UG version notes.
func ParseBPMFromText(s string) int {
	m := bpmInTextRegex.FindStringSubmatch(s)
	if len(m) < 2 {
		return 0
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0
	}
	return ClampBPM(n)
}

func parseBPMValue(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if n, err := strconv.Atoi(strings.Fields(s)[0]); err == nil {
		return n
	}
	return ParseBPMFromText(s)
}

const DefaultBPM = 120

// ClampBPM keeps tempo in a sensible guitar-tab range.
func ClampBPM(n int) int {
	if n < 40 {
		return 40
	}
	if n > 300 {
		return 300
	}
	return n
}

// NormalizeTabBPM copies tempo metadata into the canonical "bpm" key.
func NormalizeTabBPM(tab *model.Tab) {
	if tab == nil {
		return
	}
	if tab.Metadata == nil {
		tab.Metadata = map[string]string{}
	}
	if strings.TrimSpace(tab.Metadata["bpm"]) != "" {
		tab.Metadata["bpm"] = strconv.Itoa(TabBPM(tab))
		return
	}
	if tempo := strings.TrimSpace(tab.Metadata["tempo"]); tempo != "" {
		if n := parseBPMValue(tempo); n > 0 {
			tab.Metadata["bpm"] = strconv.Itoa(n)
		}
	}
}
