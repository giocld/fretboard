package player

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// BackendCapabilities describes what the active audio backend can do
// without tearing down and restarting playback.
type BackendCapabilities struct {
	CanSeekWithoutRestart bool
	CanChangeSpeedLive    bool
}

// Capabilities reports the active backend's capabilities. Only mpv can
// change speed live (over its JSON IPC socket); ffplay and mpg123 fall
// back to restart-based changes, and MIDI mode has no audio backend.
func (e *Engine) Capabilities() BackendCapabilities {
	if e.mode != "audio" || e.audioBackend != "mpv" {
		return BackendCapabilities{}
	}
	return BackendCapabilities{CanSeekWithoutRestart: true, CanChangeSpeedLive: true}
}

// mpvSocketPath returns the mpv IPC socket path, creating a per-engine
// temp dir on first use. Returns "" when the dir cannot be created, in
// which case callers fall back to restart-based rate changes.
func (e *Engine) mpvSocketPath() string {
	if e.mpvSocket != "" {
		return e.mpvSocket
	}
	dir, err := os.MkdirTemp("", "fretboard-mpv-")
	if err != nil {
		return ""
	}
	e.mpvSocket = filepath.Join(dir, "fretboard-mpv.sock")
	return e.mpvSocket
}

// cleanupMPVSocket removes the IPC socket file and its temp dir. mpv
// itself unlinks the socket on clean exit, but Stop hard-kills the
// process tree, so the file may linger.
func (e *Engine) cleanupMPVSocket() {
	if e.mpvSocket == "" {
		return
	}
	os.Remove(e.mpvSocket)
	os.RemoveAll(filepath.Dir(e.mpvSocket))
	e.mpvSocket = ""
}

// setMPVSpeedLive pushes a new playback rate to the running mpv over its
// JSON IPC socket and waits for the acknowledgment. Returns false when
// mpv is not the active backend or the socket is missing/unresponsive
// (e.g. a fake mpv that never bound one), so callers fall back to the
// restart path. The whole attempt is bounded by a short deadline so a
// dead socket never stalls playback.
func (e *Engine) setMPVSpeedLive(rate float64) bool {
	if e.audioBackend != "mpv" || e.mpvSocket == "" {
		return false
	}
	conn, err := net.DialTimeout("unix", e.mpvSocket, 150*time.Millisecond)
	if err != nil {
		return false
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(150 * time.Millisecond))
	msg := fmt.Sprintf(`{"command": ["set_property", "speed", %s]}`+"\n", strconv.FormatFloat(rate, 'f', -1, 64))
	if _, err := conn.Write([]byte(msg)); err != nil {
		return false
	}
	// mpv replies {"error":"success",...} per command; confirm the rate
	// change landed before claiming success.
	buf := make([]byte, 256)
	n, err := conn.Read(buf)
	if err != nil {
		return false
	}
	return strings.Contains(string(buf[:n]), `"success"`)
}
