package player

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"fretboard/internal/config"
	"fretboard/internal/model"
)

var audioExtensions = []string{".mp3", ".flac", ".ogg", ".wav", ".m4a", ".opus", ".aac"}

// FindAudio locates a backing-track file for tab.
func FindAudio(tab *model.Tab, tabPath string, extraDirs []string) string {
	if tab == nil {
		return ""
	}

	var dirs []string
	if tabPath != "" && !strings.HasPrefix(tabPath, "online://") {
		if dir := filepath.Dir(tabPath); dir != "" && dir != "." {
			dirs = append(dirs, dir)
		}
		base := strings.TrimSuffix(filepath.Base(tabPath), filepath.Ext(tabPath))
		if path := findAudioByBasename(dirs, base); path != "" {
			return path
		}
	}

	for _, d := range extraDirs {
		if d = strings.TrimSpace(d); d != "" {
			dirs = append(dirs, expandHome(d))
		}
	}
	if cfgDir, err := config.AudioDir(); err == nil {
		dirs = append(dirs, cfgDir)
	}

	names := audioNameCandidates(tab)
	for _, dir := range uniqueDirs(dirs) {
		if path := findAudioByNames(dir, names); path != "" {
			return path
		}
	}
	return ""
}

func audioNameCandidates(tab *model.Tab) []string {
	title := strings.TrimSpace(tab.Title)
	artist := strings.TrimSpace(tab.Artist)
	var names []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		for _, n := range names {
			if strings.EqualFold(n, s) {
				return
			}
		}
		names = append(names, s)
	}
	add(title)
	if artist != "" && title != "" {
		add(fmt.Sprintf("%s - %s", artist, title))
		add(fmt.Sprintf("%s_%s", artist, title))
	}
	return names
}

func findAudioByBasename(dirs []string, base string) string {
	for _, dir := range dirs {
		if path := findAudioByNames(dir, []string{base}); path != "" {
			return path
		}
	}
	return ""
}

func findAudioByNames(dir string, names []string) string {
	if dir == "" {
		return ""
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return ""
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	// Pass 1: exact normalized match.
	for _, name := range names {
		want := normalizeAudioName(name)
		for _, ent := range entries {
			if ent.IsDir() {
				continue
			}
			ext := strings.ToLower(filepath.Ext(ent.Name()))
			if !isAudioExt(ext) {
				continue
			}
			stem := normalizeAudioName(strings.TrimSuffix(ent.Name(), ext))
			if stem == want {
				return filepath.Join(dir, ent.Name())
			}
		}
	}
	// Pass 2 (relaxed): a file whose normalized name *contains* a candidate
	// — "Sultans of Swing (Live 1984).mp3" pairs with the tab even though
	// the extra words break exact matching. The closest candidate (shortest
	// stem) wins so "Sultans of Swing" beats "Sultans of Swing 2".
	best := ""
	bestLen := 0
	for _, name := range names {
		want := normalizeAudioName(name)
		if len(want) < 4 {
			continue
		}
		for _, ent := range entries {
			if ent.IsDir() {
				continue
			}
			ext := strings.ToLower(filepath.Ext(ent.Name()))
			if !isAudioExt(ext) {
				continue
			}
			stem := normalizeAudioName(strings.TrimSuffix(ent.Name(), ext))
			if strings.Contains(stem, want) && (best == "" || len(stem) < bestLen) {
				best = filepath.Join(dir, ent.Name())
				bestLen = len(stem)
			}
		}
	}
	return best
}

func normalizeAudioName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func isAudioExt(ext string) bool {
	for _, e := range audioExtensions {
		if ext == e {
			return true
		}
	}
	return false
}

func uniqueDirs(dirs []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, d := range dirs {
		d = filepath.Clean(expandHome(d))
		if d == "" || d == "." {
			continue
		}
		if _, ok := seen[d]; ok {
			continue
		}
		seen[d] = struct{}{}
		out = append(out, d)
	}
	return out
}

// buildAudioCandidates returns the player subprocess candidates for the given
// file, seek position (file time), playback rate (1 = normal), and volume.
// ffplay and mpv support seeking and pitch-preserving rate control; mpg123 is
// a plain fallback.
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
	mpvArgs = append(mpvArgs, "--no-video", "--really-quiet", "--no-terminal", fmt.Sprintf("--volume=%d", vol), path)
	return []candidate{
		{bin: "ffplay", driver: filepath.Base(path), args: ffplayArgs},
		{bin: "mpv", driver: filepath.Base(path), args: mpvArgs},
		{bin: "mpg123", driver: filepath.Base(path), args: []string{"-q", path}},
	}
}

func formatSeek(d time.Duration) string {
	return strconv.FormatFloat(d.Seconds(), 'f', 1, 64)
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
		cmd := exec.Command(binPath, c.args...)
		cmd.SysProcAttr = childProcAttr()
		var stderr stderrCollector
		cmd.Stderr = &stderr
		cmd.Stdout = io.Discard
		if err := cmd.Start(); err != nil {
			lastErr = fmt.Errorf("%s %v: %w", binPath, c.args, err)
			continue
		}
		startReaper(cmd)
		time.Sleep(150 * time.Millisecond)
		if !processAlive(cmd) {
			_ = cmd.Wait()
			msg := stderr.String()
			if msg == "" {
				msg = "audio player exited immediately"
			}
			lastErr = fmt.Errorf("%s: %s", binPath, summarizeStderr(msg))
			continue
		}
		e.audioCmd = cmd
		e.mode = "audio"
		e.audioPath = path
		e.ActiveDriver = c.driver
		e.LastError = ""
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
	return fmt.Errorf("no audio player found — install ffplay, mpv, or mpg123")
}

func (e *Engine) stopAudio() {
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
