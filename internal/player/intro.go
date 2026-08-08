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
	s := string(out)
	idx := strings.Index(s, "silence_end:")
	if idx < 0 {
		return 0, nil
	}
	rest := strings.TrimSpace(s[idx+len("silence_end:"):])
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return 0, nil
	}
	sec, err := strconv.ParseFloat(fields[0], 64)
	if err != nil || sec < 0.4 || sec > 30 {
		return 0, nil
	}
	return time.Duration(sec * float64(time.Second)), nil
}
