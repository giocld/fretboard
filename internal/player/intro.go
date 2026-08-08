package player

import (
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// LeadingSilence estimates the duration of leading silence in an audio file
// using ffmpeg's silencedetect filter — a quick assist for recordings that
// open with silence, applause, or a count-in. It returns 0 with no error when
// ffmpeg is unavailable, no leading silence is found, or the result is
// implausible (only 0.4–30 s lead-ins are treated as intros).
//
// Count-ins and claps are handled: the offset is the end of the LAST silence
// segment before a sustained sound (≥ 1.5 s of audio), so a
// "silence -> clicks -> music" opening anchors at the music, not at the end of
// the first silent gap.
func LeadingSilence(path string) (time.Duration, error) {
	if path == "" {
		return 0, nil
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return 0, nil // optional assist: no ffmpeg, no detection
	}
	cmd := exec.Command("ffmpeg", "-hide_banner", "-i", path,
		"-af", "silencedetect=noise=-35dB:d=0.4",
		"-f", "null", "-")
	out, _ := cmd.CombinedOutput()
	return introOffsetFromSilenceLog(string(out))
}

// introOffsetFromSilenceLog extracts the intro offset from a silencedetect
// log: the end of the last silence segment whose following sound lasts at
// least 1.5 s (sustained music), falling back to the first silence end when
// no segment qualifies (e.g. an intro that fades in directly).
func introOffsetFromSilenceLog(log string) (time.Duration, error) {
	type seg struct{ start, end float64 }
	var segs []seg
	var cur *seg
	flush := func() {
		if cur != nil && cur.end >= 0 {
			segs = append(segs, *cur)
		}
		cur = nil
	}
	for _, line := range strings.Split(log, "\n") {
		if i := strings.Index(line, "silence_start:"); i >= 0 {
			flush()
			if v, err := parseFirstFloat(line[i+len("silence_start:"):]); err == nil {
				cur = &seg{start: v, end: -1}
			}
			continue
		}
		if i := strings.Index(line, "silence_end:"); i >= 0 {
			if v, err := parseFirstFloat(line[i+len("silence_end:"):]); err == nil {
				if cur != nil {
					cur.end = v
				} else {
					cur = &seg{start: 0, end: v}
				}
			}
		}
	}
	flush()
	if len(segs) == 0 {
		return 0, nil
	}
	// Prefer the end of the last silence before sustained sound (≥1.5 s of
	// audio until the next silence, or to the end of the file).
	for i := range segs {
		nextStart := float64(1 << 30)
		if i+1 < len(segs) {
			nextStart = segs[i+1].start
		}
		sound := nextStart - segs[i].end
		if sound >= 1.5 {
			sec := segs[i].end
			if sec < 0.4 || sec > 30 {
				return 0, nil
			}
			return time.Duration(sec * float64(time.Second)), nil
		}
	}
	// Fallback: the very first silence end.
	sec := segs[0].end
	if sec < 0.4 || sec > 30 {
		return 0, nil
	}
	return time.Duration(sec * float64(time.Second)), nil
}

func parseFirstFloat(s string) (float64, error) {
	s = strings.TrimSpace(s)
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return 0, strconv.ErrSyntax
	}
	return strconv.ParseFloat(fields[0], 64)
}
