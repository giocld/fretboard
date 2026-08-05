//go:build !windows

package player

import (
	"os/exec"
	"testing"
	"time"
)

// waitFor reports true once cond() returns true within the deadline.
func waitFor(t *testing.T, timeout time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return cond()
}

// TestReaperMarksNaturallyExitedProcessAsDead guards against the Unix zombie
// trap: a child that exits on its own stays in the process table until
// Wait()ed, and kill(pid, 0) keeps reporting it alive. Without the reaper,
// audio/MIDI playback would never be detected as ended.
func TestReaperMarksNaturallyExitedProcessAsDead(t *testing.T) {
	cmd := exec.Command("sh", "-c", "sleep 0.3")
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start shell child: %v", err)
	}
	startReaper(cmd)
	if !processAlive(cmd) {
		t.Fatal("process should be alive right after start")
	}
	if !waitFor(t, 3*time.Second, func() bool { return !processAlive(cmd) }) {
		t.Fatal("process still reported alive 3s after natural exit; zombie not reaped")
	}
}

// TestReaperReportsRunningProcessAlive ensures the reaper does not kill
// liveness detection for children that are still running.
func TestReaperReportsRunningProcessAlive(t *testing.T) {
	cmd := exec.Command("sh", "-c", "sleep 30")
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start shell child: %v", err)
	}
	startReaper(cmd)
	if !processAlive(cmd) {
		t.Fatal("running process must be reported alive")
	}
	killProcessTree(cmd)
	if processAlive(cmd) {
		t.Fatal("process must be reported dead after killProcessTree")
	}
}

// TestKillProcessTreeWithReaper verifies the kill path waits on the reaper and
// escalates to SIGKILL when the child ignores SIGTERM.
func TestKillProcessTreeWithReaper(t *testing.T) {
	cmd := exec.Command("sh", "-c", "trap '' TERM; sleep 30")
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start shell child: %v", err)
	}
	startReaper(cmd)
	start := time.Now()
	killProcessTree(cmd)
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("killProcessTree took %v", elapsed)
	}
	if processAlive(cmd) {
		t.Fatal("process must be dead after killProcessTree")
	}
}
