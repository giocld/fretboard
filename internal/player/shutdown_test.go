package player

import (
	"os/exec"
	"runtime"
	"testing"
	"time"
)

func TestEngineShutdownStopsPlayback(t *testing.T) {
	e := NewEngine()
	e.shutdown = true
	if !e.ShutdownRequested() {
		t.Fatal("expected shutdown flag")
	}
	if err := e.Play(nil, 120, PlayContext{}); err == nil {
		t.Fatal("expected play to fail after shutdown")
	}
}

func TestKillProcessTreeStopsSleep(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process groups differ on windows")
	}
	cmd := exec.Command("sleep", "60")
	cmd.SysProcAttr = childProcAttr()
	if err := cmd.Start(); err != nil {
		t.Skipf("sleep unavailable: %v", err)
	}
	killProcessTree(cmd)
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("sleep process did not exit after killProcessTree")
	}
}
