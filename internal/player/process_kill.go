package player

import (
	"os/exec"
	"runtime"
	"syscall"
	"time"
)

// childProcAttr returns SysProcAttr for detached child processes that can be
// killed as a group when the app exits.
func childProcAttr() *syscall.SysProcAttr {
	if runtime.GOOS == "windows" {
		return nil
	}
	return &syscall.SysProcAttr{Setsid: true}
}

// killProcessTree terminates a child process and its process group.
func killProcessTree(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	pid := cmd.Process.Pid
	if runtime.GOOS == "windows" {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		return
	}
	_ = syscall.Kill(pid, syscall.SIGTERM)
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		_, _ = cmd.Process.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		_ = syscall.Kill(pid, syscall.SIGKILL)
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		_ = cmd.Process.Kill()
		<-done
	}
}
