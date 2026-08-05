//go:build !windows

package player

import (
	"os/exec"
	"sync"
	"syscall"
	"time"
)

var (
	childMu   sync.Mutex
	childDone = map[*exec.Cmd]chan struct{}{} // closed once the reaper has Wait()ed the child
	childExit = map[*exec.Cmd]bool{}          // children whose Wait() has already returned
)

// childProcAttr returns SysProcAttr for detached child processes that can be
// killed as a group when the app exits.
func childProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}

// startReaper spawns a goroutine that reaps the child when it exits. A child
// that exits naturally stays a zombie until Wait()ed; without a reaper,
// kill(pid, 0) keeps reporting it alive forever, so playback is never
// detected as ended and zombies pile up.
func startReaper(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	childMu.Lock()
	if _, ok := childDone[cmd]; ok {
		childMu.Unlock()
		return
	}
	done := make(chan struct{})
	childDone[cmd] = done
	childMu.Unlock()
	go func() {
		_ = cmd.Wait()
		childMu.Lock()
		delete(childDone, cmd)
		childExit[cmd] = true
		childMu.Unlock()
		close(done)
	}()
}

// killProcessTree terminates a child process and its process group, waiting
// for the reaper (or a local Wait) so the child never lingers as a zombie.
func killProcessTree(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	pid := cmd.Process.Pid
	childMu.Lock()
	done := childDone[cmd]
	childMu.Unlock()
	_ = syscall.Kill(pid, syscall.SIGTERM)
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	if done == nil {
		// Child never went through startReaper: reap it here.
		done = make(chan struct{})
		go func() {
			_, _ = cmd.Process.Wait()
			close(done)
		}()
	}
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		_ = syscall.Kill(pid, syscall.SIGKILL)
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		_ = cmd.Process.Kill()
		<-done
	}
}

// processAlive reports whether the child process is still running. A child is
// considered dead once its reaper has Wait()ed it, which is the only reliable
// way to tell a zombie from a live process on Unix.
func processAlive(cmd *exec.Cmd) bool {
	if cmd == nil || cmd.Process == nil {
		return false
	}
	childMu.Lock()
	exited := childExit[cmd]
	childMu.Unlock()
	if exited {
		return false
	}
	return cmd.Process.Signal(syscall.Signal(0)) == nil
}
