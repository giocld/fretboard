// Package clipboard copies text to the system clipboard through the
// platform's CLI tool (clip / wl-copy / xclip / pbcopy), with a clear error
// when none is available so callers can degrade gracefully.
package clipboard

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// Copy writes text to the system clipboard. It returns an error naming the
// missing tool when no clipboard program is installed.
func Copy(text string) error {
	var candidates []string
	switch runtime.GOOS {
	case "windows":
		candidates = []string{"clip"}
	case "darwin":
		candidates = []string{"pbcopy"}
	default:
		candidates = []string{"wl-copy", "xclip", "xsel"}
	}
	var lastErr error
	for _, bin := range candidates {
		path, err := exec.LookPath(bin)
		if err != nil {
			lastErr = err
			continue
		}
		cmd := exec.Command(path)
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no clipboard tool found")
	}
	return fmt.Errorf("clipboard unavailable (%v)", lastErr)
}
