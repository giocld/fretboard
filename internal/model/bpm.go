package model

import (
	"regexp"
	"strconv"
	"strings"
)

var bpmInTextRegex = regexp.MustCompile(`(?i)(?:^|[^\d])(\d{2,3})\s*bpm`)

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

// DefaultBPM is the tempo used when none can be determined.
const DefaultBPM = 120

// bestBPM returns the best-known tempo for a tab in beats per minute.
func bestBPM(tab *Tab) int {
	if tab == nil || tab.Metadata == nil {
		return DefaultBPM
	}
	for _, key := range []string{MetaKeyBPM, MetaKeyTempo} {
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
	return ParseBPMFromText(s)
}

// NormalizeTabBPM copies tempo metadata into the canonical "bpm" key.
func NormalizeTabBPM(tab *Tab) {
	if tab == nil {
		return
	}
	if tab.Metadata == nil {
		tab.Metadata = map[string]string{}
	}
	if strings.TrimSpace(tab.Metadata[MetaKeyBPM]) != "" {
		tab.Metadata[MetaKeyBPM] = strconv.Itoa(bestBPM(tab))
		return
	}
	if tempo := strings.TrimSpace(tab.Metadata[MetaKeyTempo]); tempo != "" {
		if n := parseBPMValue(tempo); n > 0 {
			tab.Metadata[MetaKeyBPM] = strconv.Itoa(n)
		}
	}
}
