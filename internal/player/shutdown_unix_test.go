//go:build !windows

package player

import (
	"os/exec"
	"testing"
	"time"
)

func TestKillProcessTreeStopsSleep(t *testing.T) {
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
