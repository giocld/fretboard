package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withConfigDir runs fn with XDG_CONFIG_HOME pointed at a temp dir so
// config/store writes never touch the real user configuration.
func withConfigDir(t *testing.T, fn func(dir string)) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	fn(dir)
}

func run(args ...string) (code int, stdout, stderr string) {
	var out, errBuf bytes.Buffer
	code = Run(args, &out, &errBuf)
	return code, out.String(), errBuf.String()
}

func TestRunImportUsage(t *testing.T) {
	withConfigDir(t, func(dir string) {
		code, _, stderr := run("import")
		if code != 1 || !strings.Contains(stderr, "usage: fretboard import") {
			t.Fatalf("code=%d stderr=%q", code, stderr)
		}
	})
}

func TestRunTooManyArgs(t *testing.T) {
	withConfigDir(t, func(dir string) {
		code, _, stderr := run("a", "b", "c")
		if code != 1 || !strings.Contains(stderr, "usage: fretboard [tab-file]") {
			t.Fatalf("code=%d stderr=%q", code, stderr)
		}
	})
}

func TestRunImportMissingFile(t *testing.T) {
	withConfigDir(t, func(dir string) {
		code, _, stderr := run("import", filepath.Join(dir, "does-not-exist.gp5"))
		if code != 1 || !strings.Contains(stderr, "import:") {
			t.Fatalf("code=%d stderr=%q", code, stderr)
		}
	})
}

func TestRunImportFile(t *testing.T) {
	withConfigDir(t, func(dir string) {
		src := filepath.Join(dir, "tab.txt")
		if err := os.WriteFile(src, []byte("Tuning: E Standard\n\ne|0-3-5|\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		code, _, stderr := run("import", src)
		if code != 0 {
			t.Fatalf("code=%d stderr=%q", code, stderr)
		}
		if _, err := os.Stat(filepath.Join(dir, "fretboard", "fretboard.db")); err != nil {
			t.Fatalf("expected library db to be created: %v", err)
		}
	})
}

func TestRunHelpExitsZero(t *testing.T) {
	code, _, _ := run("-h")
	if code != 0 {
		t.Fatalf("code=%d, want 0 for -h", code)
	}
}
