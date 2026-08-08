package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fretboard/internal/config"
	"fretboard/internal/testutil"
)

// withConfigDir runs fn with the user config dir pointed at a temp dir so
// config/store writes never touch the real user configuration.
func withConfigDir(t *testing.T, fn func(dir string)) {
	t.Helper()
	testutil.WithConfigDir(t, fn)
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
		cfgDir, err := config.Dir()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(cfgDir, "fretboard.db")); err != nil {
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

// TestWriteCrashLog guards S6.4: a panic writes a stack report into the
// config dir and returns its path.
func TestWriteCrashLog(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir) // Windows config dir; other OSes ignore it
	path := writeCrashLog("boom", []byte("goroutine 1 [running]:\nmain.fail()\n"))
	if path == "" {
		t.Skip("config dir not resolvable in this environment")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("crash log not written: %v", err)
	}
	content := string(data)
	for _, want := range []string{"boom", "main.fail"} {
		if !strings.Contains(content, want) {
			t.Fatalf("crash log missing %q:\n%s", want, content)
		}
	}
}
