package player

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// startMPVIPCServer listens on a unix socket, records the first command it
// receives, and replies with mpv's success acknowledgment so the engine can
// confirm the rate change landed. It mimics the real mpv JSON IPC contract.
func startMPVIPCServer(t *testing.T) (sockPath string, received chan string) {
	t.Helper()
	sockPath = filepath.Join(t.TempDir(), "mpv.sock")
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	received = make(chan string, 1)
	go func() {
		defer ln.Close()
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		buf := make([]byte, 4096)
		n, err := conn.Read(buf)
		if err != nil {
			return
		}
		received <- string(buf[:n])
		_, _ = conn.Write([]byte(`{"error": "success"}` + "\n"))
	}()
	t.Cleanup(func() {
		ln.Close()
		os.Remove(sockPath)
	})
	return sockPath, received
}

// TestCapabilitiesMatrix guards the backend capability report: only mpv
// audio can change speed live; everything else (ffplay, mpg123, MIDI,
// idle) gets an all-false struct.
func TestCapabilitiesMatrix(t *testing.T) {
	e := NewEngine()
	if got := e.Capabilities(); got != (BackendCapabilities{}) {
		t.Fatalf("idle capabilities = %+v, want none", got)
	}
	cases := []struct {
		name    string
		backend string
		mode    string
		want    BackendCapabilities
	}{
		{"mpv audio", "mpv", "audio", BackendCapabilities{CanSeekWithoutRestart: true, CanChangeSpeedLive: true}},
		{"ffplay audio", "ffplay", "audio", BackendCapabilities{}},
		{"mpg123 audio", "mpg123", "audio", BackendCapabilities{}},
		{"midi", "", "midi", BackendCapabilities{}},
	}
	for _, tc := range cases {
		e.mode = tc.mode
		e.audioBackend = tc.backend
		if got := e.Capabilities(); got != tc.want {
			t.Fatalf("%s: Capabilities() = %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

// TestSetRateOverMPVIPC guards the live speed path: with mpv active and a
// reachable IPC socket, SetRate sends the mpv JSON command and must NOT
// restart the player. A fake mpv on PATH makes the restart fallback
// observable (it would spawn a new process), so a nil audioCmd proves the
// IPC fast path was taken.
func TestSetRateOverMPVIPC(t *testing.T) {
	writeFakeMPV(t, `{"pos": 1.0, "dur": 100}`)
	sock, received := startMPVIPCServer(t)

	e := NewEngine()
	e.mode = "audio"
	e.audioBackend = "mpv"
	e.audioPath = filepath.Join(t.TempDir(), "song.mp3")
	e.mpvSocket = sock
	e.rate = 1

	if err := e.SetRate(1.5); err != nil {
		t.Fatalf("SetRate: %v", err)
	}
	if e.rate != 1.5 {
		t.Fatalf("rate = %v, want 1.5", e.rate)
	}
	if e.audioCmd != nil {
		t.Fatal("SetRate restarted the player instead of using IPC")
	}
	select {
	case msg := <-received:
		want := `{"command": ["set_property", "speed", 1.5]}` + "\n"
		if msg != want {
			t.Fatalf("IPC command = %q, want %q", msg, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no IPC command received")
	}
}

// TestSetRateFallsBackWithoutSocket guards the fallback path: the fake mpv
// ignores --input-ipc-server and never binds a socket, so SetRate must
// restart the player at the current position instead of failing or
// stalling.
func TestSetRateFallsBackWithoutSocket(t *testing.T) {
	writeFakeMPV(t, `{"pos": 3.0, "dur": 100}`)
	dir := t.TempDir()
	audio := filepath.Join(dir, "song.mp3")
	if err := os.WriteFile(audio, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	e := NewEngine()
	e.Volume = 80
	if err := e.playAudioFile(audio); err != nil {
		t.Fatalf("playAudioFile: %v", err)
	}
	defer e.Stop()

	oldCmd := e.audioCmd
	if oldCmd == nil {
		t.Fatal("audio playback did not start")
	}
	if err := e.SetRate(2); err != nil {
		t.Fatalf("SetRate: %v", err)
	}
	if e.audioCmd == nil || e.audioCmd == oldCmd {
		t.Fatal("SetRate did not restart the player (expected fallback)")
	}
	if got := e.Rate(); got != 2 {
		t.Fatalf("rate = %v, want 2", got)
	}
	// The restarted fake mpv keeps reporting pos 3.0, so the playhead
	// resumes near the pre-change position.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if e.Elapsed() >= 3*time.Second {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if e.Mode() != "audio" {
		t.Fatalf("mode = %q, want audio", e.Mode())
	}
	if el := e.Elapsed(); el < 3*time.Second || el > 6*time.Second {
		t.Fatalf("Elapsed after restart = %v, want ~3s", el)
	}
}

// TestMPVSocketArgAndCleanup guards the socket lifecycle: launching mpv
// allocates a socket path and temp dir, and Stop removes both.
func TestMPVSocketArgAndCleanup(t *testing.T) {
	writeFakeMPV(t, `{"pos": 0.5, "dur": 10}`)
	dir := t.TempDir()
	audio := filepath.Join(dir, "song.mp3")
	if err := os.WriteFile(audio, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	e := NewEngine()
	e.Volume = 80
	if err := e.playAudioFile(audio); err != nil {
		t.Fatalf("playAudioFile: %v", err)
	}
	if e.mpvSocket == "" {
		t.Fatal("mpv socket path not set after launch")
	}
	sockDir := filepath.Dir(e.mpvSocket)
	if _, err := os.Stat(sockDir); err != nil {
		t.Fatalf("socket dir missing while playing: %v", err)
	}
	if err := e.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if e.mpvSocket != "" {
		t.Fatal("mpv socket path not cleared on Stop")
	}
	if _, err := os.Stat(sockDir); !os.IsNotExist(err) {
		t.Fatalf("socket dir not cleaned up on Stop (stat err: %v)", err)
	}
}
