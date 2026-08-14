package player

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// buildAudioCandidates returns the player subprocess candidates for the given
// file, seek position (file time), playback rate (1 = normal), and volume.
// ffplay and mpv support seeking and pitch-preserving rate control; mpg123 is
// a plain fallback.
// mpvStatusMsg is the --term-status-msg format the engine parses for
// position feedback: the player reports its true output position and the
// file duration, which makes Elapsed() correct including startup latency,
// atempo latency, and seek resumption.
const mpvStatusMsg = `{"pos": ${time-pos}, "dur": ${duration}}`

func buildAudioCandidates(path string, seek time.Duration, rate float64, vol int) []candidate {
	var ffplayArgs, mpvArgs []string
	if seek > 0 {
		ffplayArgs = append(ffplayArgs, "-ss", formatSeek(seek))
		mpvArgs = append(mpvArgs, "--start="+formatSeek(seek))
	}
	if rate != 1 {
		ffplayArgs = append(ffplayArgs, "-af", fmt.Sprintf("atempo=%.3f", rate))
		mpvArgs = append(mpvArgs, fmt.Sprintf("--speed=%.3f", rate))
	}
	ffplayArgs = append(ffplayArgs, "-nodisp", "-autoexit", "-loglevel", "quiet", "-vn", "-volume", fmt.Sprintf("%d", vol), path)
	// mpv reports its real output position via --term-status-msg, which the
	// engine parses for position feedback (and a duration fallback when
	// ffprobe is missing). The status line replaces the default one.
	mpvArgs = append(mpvArgs, "--no-video", "--term-status-msg="+mpvStatusMsg, fmt.Sprintf("--volume=%d", vol), path)
	candidates := []candidate{
		{bin: "ffplay", driver: filepath.Base(path), args: ffplayArgs},
		{bin: "mpv", driver: filepath.Base(path), args: mpvArgs},
	}
	// mpg123 cannot seek or change speed: offering it for those cases would
	// silently play the wrong part of the file at the wrong tempo.
	if rate == 1 && seek == 0 {
		candidates = append(candidates, candidate{bin: "mpg123", driver: filepath.Base(path), args: []string{"-q", path}})
	}
	return candidates
}

func formatSeek(d time.Duration) string {
	return strconv.FormatFloat(d.Seconds(), 'f', 1, 64)
}

// posFeedback is the mpv position feedback channel: the player reports its
// true output position (and duration) via --term-status-msg, so Elapsed()
// includes startup latency, filter latency, and seek resumption instead of
// guessing from wall time.
type posFeedback struct {
	mu   sync.Mutex
	pos  time.Duration
	dur  time.Duration
	seen bool
}

func (e *Engine) playAudio(path string, seek time.Duration) error {
	if err := e.checkShutdown(); err != nil {
		return err
	}
	if err := e.Synth.Stop(); err != nil {
		return fmt.Errorf("stop previous playback: %w", err)
	}
	e.stopAudio()

	vol := e.Volume
	if vol <= 0 {
		vol = 80
	}
	candidates := buildAudioCandidates(path, seek, e.rate, vol)

	var lastErr error
	for _, c := range candidates {
		binPath, err := lookPath(c.bin)
		if err != nil {
			continue
		}
		cmd, err := e.tryAudioCandidate(c, binPath, path)
		if err != nil {
			lastErr = err
			continue
		}
		e.audioCmd = cmd
		e.mode = "audio"
		e.audioPath = path
		e.ActiveDriver = c.driver
		e.LastError = ""
		e.posFB.mu.Lock()
		e.posFB.pos = 0
		e.posFB.dur = 0
		e.posFB.seen = false
		e.posFB.mu.Unlock()
		if e.shutdown {
			killProcessTree(cmd)
			e.audioCmd = nil
			return errors.New("playback shut down")
		}
		return nil
	}
	if lastErr != nil {
		return fmt.Errorf("audio playback failed: %w", lastErr)
	}
	return fmt.Errorf("no audio player found — install ffplay or mpv (mpg123 cannot seek or change speed)")
}

// tryAudioCandidate spawns one player candidate and verifies it stays alive
// past the startup probe, returning the running command. mpv additionally gets
// a --term-status-msg pipe that feeds e.posFB for position feedback.
func (e *Engine) tryAudioCandidate(c candidate, binPath, path string) (*exec.Cmd, error) {
	cmd := exec.Command(binPath, c.args...)
	cmd.SysProcAttr = childProcAttr()
	var stderr stderrCollector
	cmd.Stderr = &stderr
	cmd.Stdout = io.Discard
	if c.bin == "mpv" {
		// Position feedback: pipe the status line into a scanner that
		// feeds e.posFB (tee'd into the collector for error summaries).
		pr, pw := io.Pipe()
		cmd.Stderr = io.MultiWriter(&stderr, pw)
		cmd.Stdout = io.MultiWriter(&stderr, pw)
		go func() {
			defer pw.Close()
			defer pr.Close()
			sc := bufio.NewScanner(pr)
			sc.Buffer(make([]byte, 1024*1024), 4*1024*1024)
			for sc.Scan() {
				e.feedMPVStatus(sc.Text())
			}
		}()
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("%s %v: %w", binPath, c.args, err)
	}
	startReaper(cmd)
	time.Sleep(150 * time.Millisecond)
	if !processAlive(cmd) {
		_ = cmd.Wait()
		msg := stderr.String()
		if msg == "" {
			msg = "audio player exited immediately"
		}
		return nil, fmt.Errorf("%s: %s", binPath, summarizeStderr(msg))
	}
	return cmd, nil
}

// feedMPVStatus parses one --term-status-msg line into the position feedback.
func (e *Engine) feedMPVStatus(line string) {
	pos, dur, ok := parseMPVStatus(line)
	if !ok {
		return
	}
	e.posFB.mu.Lock()
	e.posFB.pos = pos
	if dur > 0 {
		e.posFB.dur = dur
	}
	e.posFB.seen = true
	e.posFB.mu.Unlock()
	// Duration fallback: without ffprobe the audio stays synced thanks to
	// the player's own duration report.
	if dur > 0 && e.audioDuration == 0 {
		e.audioDuration = dur
	}
}

// parseMPVStatus extracts pos and dur from a status line like
// {"pos": 12.345, "dur": 253.2}.
func parseMPVStatus(line string) (pos, dur time.Duration, ok bool) {
	p := parseFloatAfter(line, `"pos":`)
	d := parseFloatAfter(line, `"dur":`)
	if p < 0 {
		return 0, 0, false
	}
	return time.Duration(p * float64(time.Second)), time.Duration(d * float64(time.Second)), true
}

// parseFloatAfter parses the first float after marker, or -1 when absent.
func parseFloatAfter(line, marker string) float64 {
	_, rest, found := strings.Cut(line, marker)
	if !found {
		return -1
	}
	j := 0
	for j < len(rest) && (rest[j] == ' ' || rest[j] == '\t') {
		j++
	}
	start := j
	for j < len(rest) && ((rest[j] >= '0' && rest[j] <= '9') || rest[j] == '.' || rest[j] == '-') {
		j++
	}
	if j == start {
		return -1
	}
	v, err := strconv.ParseFloat(rest[start:j], 64)
	if err != nil {
		return -1
	}
	return v
}

func (e *Engine) stopAudio() {
	e.posFB.mu.Lock()
	e.posFB.pos = 0
	e.posFB.dur = 0
	e.posFB.seen = false
	e.posFB.mu.Unlock()
	if e.audioCmd == nil || e.audioCmd.Process == nil {
		e.audioCmd = nil
		return
	}
	killProcessTree(e.audioCmd)
	e.audioCmd = nil
	e.ActiveDriver = ""
}

func (e *Engine) audioRunning() bool {
	if e.audioCmd == nil || e.audioCmd.Process == nil {
		return false
	}
	if !processAlive(e.audioCmd) {
		if e.LastError == "" {
			e.LastError = "audio stopped unexpectedly"
		}
		e.audioCmd = nil
		e.mode = ""
		e.audioPath = ""
		e.playbackStart = time.Time{}
		e.audioDuration = 0
		return false
	}
	return true
}
