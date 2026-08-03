//go:build windows

package player

import (
	"os/exec"
	"syscall"
)

// childProcAttr returns SysProcAttr for detached child processes. Windows has
// no process groups here, so nil.
func childProcAttr() *syscall.SysProcAttr {
	return nil
}

// killProcessTree terminates a child process. Windows has no process groups
// in this implementation, so only the direct child is killed.
func killProcessTree(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
	_, _ = cmd.Process.Wait()
}

// processAlive reports whether the child process is still running by querying
// its exit code. syscall.Signal(0) is unsupported on Windows (only Kill
// works), so it cannot be used for liveness probes there.
const (
	windowsProcessQueryLimitedInformation = 0x1000
	windowsStillActive                    = 259
)

func processAlive(cmd *exec.Cmd) bool {
	if cmd == nil || cmd.Process == nil {
		return false
	}
	handle, err := syscall.OpenProcess(windowsProcessQueryLimitedInformation, false, uint32(cmd.Process.Pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(handle)
	var exitCode uint32
	if err := syscall.GetExitCodeProcess(handle, &exitCode); err != nil {
		return false
	}
	return exitCode == windowsStillActive
}
