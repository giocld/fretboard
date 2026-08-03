//go:build windows

package player

import (
	"os/exec"
	"testing"
	"time"
)

func TestKillProcessTreeStopsSleep(t *testing.T) {
	cmd := exec.Command("powershell", "-NoProfile", "-Command", "Start-Sleep 60")
	cmd.SysProcAttr = childProcAttr()
	if err := cmd.Start(); err != nil {
		t.Skipf("powershell unavailable: %v", err)
	}
	killProcessTree(cmd)
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("sleep process did not exit after killProcessTree")
	}
}
