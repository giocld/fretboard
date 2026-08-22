package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fretboard/internal/config"
	"fretboard/internal/diag"
	"fretboard/internal/player"
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

// writeTestTab writes a minimal parseable tab file.
func writeTestTab(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("Tuning: E Standard\n\ne|0-3-5|\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestRunHelpListsSubcommands pins the -h text: every Wave-2 subcommand and
// render flag is discoverable from usage.
func TestRunHelpListsSubcommands(t *testing.T) {
	code, _, stderr := run("-h")
	if code != 0 {
		t.Fatalf("code=%d, want 0", code)
	}
	for _, want := range []string{"doctor", "scan", "setup gp", "--print", "--html", "export <archive.json>"} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("-h output missing %q:\n%s", want, stderr)
		}
	}
}

// TestRunPrintTab: --print renders a parsed tab as plain text to stdout.
func TestRunPrintTab(t *testing.T) {
	withConfigDir(t, func(dir string) {
		src := filepath.Join(dir, "tab.txt")
		writeTestTab(t, src)
		code, stdout, stderr := run("--print", src, "--width", "100")
		if code != 0 {
			t.Fatalf("code=%d stderr=%q", code, stderr)
		}
		for _, want := range []string{"0", "3", "5"} {
			if !strings.Contains(stdout, want) {
				t.Fatalf("--print output missing fret %q:\n%s", want, stdout)
			}
		}
	})
}

// TestRunHTMLTab: --html renders to stdout, or to -o with a theme.
func TestRunHTMLTab(t *testing.T) {
	withConfigDir(t, func(dir string) {
		src := filepath.Join(dir, "tab.txt")
		writeTestTab(t, src)
		code, stdout, stderr := run("--html", src)
		if code != 0 || !strings.Contains(stdout, "<html") {
			t.Fatalf("stdout mode: code=%d stderr=%q", code, stderr)
		}
		out := filepath.Join(dir, "out.html")
		code, _, stderr = run("--html", src, "--theme", "dark", "-o", out)
		if code != 0 {
			t.Fatalf("-o mode: code=%d stderr=%q", code, stderr)
		}
		data, err := os.ReadFile(out)
		if err != nil || !strings.Contains(string(data), "<html") {
			t.Fatalf("out file: %v", err)
		}
	})
}

// TestParseRenderArgs covers the render flag forms, including flags that
// follow the file path (which the flag package cannot express).
func TestParseRenderArgs(t *testing.T) {
	mode, path, theme, outPath, width, err := parseRenderArgs([]string{"--print", "file.txt", "--width", "100"})
	if err != nil || mode != "print" || path != "file.txt" || width != 100 {
		t.Fatalf("got mode=%q path=%q width=%d err=%v", mode, path, width, err)
	}
	_, _, theme, outPath, _, err = parseRenderArgs([]string{"--html", "a.gp", "--theme=dark", "-o", "out.html"})
	if err != nil || theme != "dark" || outPath != "out.html" {
		t.Fatalf("got theme=%q out=%q err=%v", theme, outPath, err)
	}
	if _, _, _, _, _, err := parseRenderArgs([]string{"--print"}); err == nil {
		t.Fatal("missing file path should error")
	}
	if _, _, _, _, _, err := parseRenderArgs([]string{"--bogus", "x"}); err == nil {
		t.Fatal("unknown flag should error")
	}
	if _, _, _, _, _, err := parseRenderArgs([]string{"--print", "a.txt", "b.txt"}); err == nil {
		t.Fatal("multiple file paths should error")
	}
}

// TestRenderDoctorTable pins the doctor table: ok/WARN/FAIL classification,
// the name filter, and the exit code (1 on any FAIL, not on WARN).
func TestRenderDoctorTable(t *testing.T) {
	results := []diag.CheckResult{
		{Name: "mpv", OK: true, Detail: "found: /usr/bin/mpv"},
		{Name: "yt-dlp", Detail: "WARN: stale version", Fix: "pip install -U yt-dlp"},
		{Name: "soundfont", Detail: "no GM soundfont found", Fix: "install one"},
		{Name: "yt-dlp probe", Detail: "skipped: yt-dlp not installed"},
	}
	var out, errBuf bytes.Buffer
	if code := renderDoctorTable(results, "", &out, &errBuf); code != 1 {
		t.Fatalf("code=%d, want 1 (soundfont FAILs)", code)
	}
	for _, want := range []string{"mpv", "ok", "WARN", "FAIL", "soundfont"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("table missing %q:\n%s", want, out.String())
		}
	}
	// Filter narrows both the output and the exit code.
	out.Reset()
	if code := renderDoctorTable(results, "MPV", &out, &errBuf); code != 0 {
		t.Fatalf("filtered code=%d, want 0", code)
	}
	if !strings.Contains(out.String(), "mpv") || strings.Contains(out.String(), "soundfont") {
		t.Fatalf("filtered table wrong:\n%s", out.String())
	}
	// Unknown filter names the available checks.
	out.Reset()
	if code := renderDoctorTable(results, "nope", &out, &errBuf); code != 1 {
		t.Fatalf("unknown-filter code=%d, want 1", code)
	}
	if !strings.Contains(errBuf.String(), "no check named") {
		t.Fatalf("unknown-filter error=%q", errBuf.String())
	}
}

// TestRunScan wires scan end to end: import, move the file, scan relinks it,
// a default scan (most recent tab's folder) finds nothing left to do, and a
// deleted file surfaces under "missing".
func TestRunScan(t *testing.T) {
	withConfigDir(t, func(dir string) {
		oldDir := filepath.Join(dir, "old")
		newDir := filepath.Join(dir, "new")
		if err := os.MkdirAll(oldDir, 0o755); err != nil {
			t.Fatal(err)
		}
		src := filepath.Join(oldDir, "tab.txt")
		writeTestTab(t, src)
		if code, _, stderr := run("import", src); code != 0 {
			t.Fatalf("import: %s", stderr)
		}
		if err := os.MkdirAll(newDir, 0o755); err != nil {
			t.Fatal(err)
		}
		dst := filepath.Join(newDir, "tab.txt")
		if err := os.Rename(src, dst); err != nil {
			t.Fatal(err)
		}
		code, stdout, stderr := run("scan", newDir)
		if code != 0 || !strings.Contains(stdout, "relinked 1") || !strings.Contains(stdout, "missing 0") {
			t.Fatalf("scan: code=%d stdout=%q stderr=%q", code, stdout, stderr)
		}
		// No dir argument: falls back to the most recent tab's folder.
		code, stdout, stderr = run("scan")
		if code != 0 || !strings.Contains(stdout, "relinked 0") {
			t.Fatalf("default scan: code=%d stdout=%q stderr=%q", code, stdout, stderr)
		}
		// Deleting the file surfaces the row as missing.
		if err := os.Remove(dst); err != nil {
			t.Fatal(err)
		}
		code, stdout, _ = run("scan")
		if code != 0 || !strings.Contains(stdout, "missing 1") {
			t.Fatalf("missing scan: code=%d stdout=%q", code, stdout)
		}
	})
}

// TestRunExportImportArchive: export writes an archive and importing the
// .json path restores it through the archive importer.
func TestRunExportImportArchive(t *testing.T) {
	withConfigDir(t, func(dir string) {
		src := filepath.Join(dir, "tab.txt")
		writeTestTab(t, src)
		if code, _, stderr := run("import", src); code != 0 {
			t.Fatalf("import: %s", stderr)
		}
		archive := filepath.Join(dir, "lib.json")
		code, stdout, stderr := run("export", archive)
		if code != 0 || !strings.Contains(stdout, "exported library") {
			t.Fatalf("export: code=%d stdout=%q stderr=%q", code, stdout, stderr)
		}
		if _, err := os.Stat(archive); err != nil {
			t.Fatalf("archive not written: %v", err)
		}
		code, stdout, stderr = run("import", archive)
		if code != 0 || !strings.Contains(stdout, "imported library archive") {
			t.Fatalf("import archive: code=%d stdout=%q stderr=%q", code, stdout, stderr)
		}
	})
}

// TestRunSetupUsage: setup rejects bad invocations without touching the
// network.
func TestRunSetupUsage(t *testing.T) {
	withConfigDir(t, func(dir string) {
		code, _, stderr := run("setup")
		if code != 1 || !strings.Contains(stderr, "usage: fretboard setup gp") {
			t.Fatalf("code=%d stderr=%q", code, stderr)
		}
		code, _, stderr = run("setup", "bogus")
		if code != 1 || !strings.Contains(stderr, "usage: fretboard setup gp") {
			t.Fatalf("code=%d stderr=%q", code, stderr)
		}
		code, _, stderr = run("setup", "gp", "--version")
		if code != 1 || !strings.Contains(stderr, "--version needs a value") {
			t.Fatalf("code=%d stderr=%q", code, stderr)
		}
	})
}

// TestRunWiresPlayerConfig pins the startup wiring: player.HumanizeMIDI and
// player.AudioCacheMaxGB come from config (GB count passed through raw; the
// player multiplies by 1<<30 itself).
func TestRunWiresPlayerConfig(t *testing.T) {
	withConfigDir(t, func(dir string) {
		cfg, err := config.Load()
		if err != nil {
			t.Fatal(err)
		}
		cfg.HumanizeMIDI = true
		cfg.AudioCacheMaxGB = 3
		if err := config.Save(cfg); err != nil {
			t.Fatal(err)
		}
		// Any subcommand that passes through Run's config wiring; the import
		// itself fails but the wiring has already happened by then.
		run("import", filepath.Join(dir, "does-not-exist.gp5"))
		if !player.HumanizeMIDI {
			t.Fatal("player.HumanizeMIDI should be wired from config")
		}
		if player.AudioCacheMaxGB != 3 {
			t.Fatalf("player.AudioCacheMaxGB = %d, want 3", player.AudioCacheMaxGB)
		}
	})
}

// TestInstallGPParser: installGPParser downloads the platform asset, verifies
// its sha256 against the release checksum file, and installs it executable.
func TestInstallGPParser(t *testing.T) {
	asset, err := gpParserAssetName()
	if err != nil {
		t.Skipf("platform not published by the release workflow: %v", err)
	}
	bin := []byte("#!/bin/sh\necho fake gp-parser\n")
	sum := sha256.Sum256(bin)
	mux := http.NewServeMux()
	mux.HandleFunc("/"+asset, func(w http.ResponseWriter, r *http.Request) { w.Write(bin) })
	mux.HandleFunc("/sha256sums.txt", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%x  %s\n", sum, asset)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	destDir := t.TempDir()
	dest, err := installGPParser(srv.Client(), srv.URL, destDir)
	if err != nil {
		t.Fatal(err)
	}
	wantDest := filepath.Join(destDir, "gp-parser"+exeSuffix)
	if dest != wantDest {
		t.Fatalf("installed to %q, want %q", dest, wantDest)
	}
	got, err := os.ReadFile(wantDest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, bin) {
		t.Fatal("installed bytes differ from the downloaded binary")
	}
	if info, _ := os.Stat(wantDest); info.Mode()&0o111 == 0 {
		t.Fatal("installed binary is not executable")
	}
}

// TestInstallGPParserRejectsChecksumMismatch: a wrong checksum refuses the
// install and leaves nothing behind.
func TestInstallGPParserRejectsChecksumMismatch(t *testing.T) {
	asset, err := gpParserAssetName()
	if err != nil {
		t.Skipf("platform not published by the release workflow: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/"+asset, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("fake binary")) })
	mux.HandleFunc("/sha256sums.txt", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%x  %s\n", sha256.Sum256([]byte("other bytes")), asset)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	destDir := t.TempDir()
	if _, err := installGPParser(srv.Client(), srv.URL, destDir); !errors.Is(err, errChecksumMismatch) {
		t.Fatalf("err = %v, want errChecksumMismatch", err)
	}
	if _, err := os.Stat(filepath.Join(destDir, "gp-parser"+exeSuffix)); !os.IsNotExist(err) {
		t.Fatal("binary must not be installed on checksum mismatch")
	}
}

// TestInstallGPParserSidecarFallback: without sha256sums.txt, a per-asset
// bare-hex .sha256 sidecar verifies the download.
func TestInstallGPParserSidecarFallback(t *testing.T) {
	asset, err := gpParserAssetName()
	if err != nil {
		t.Skipf("platform not published by the release workflow: %v", err)
	}
	bin := []byte("sidecar binary")
	sum := sha256.Sum256(bin)
	mux := http.NewServeMux()
	mux.HandleFunc("/"+asset, func(w http.ResponseWriter, r *http.Request) { w.Write(bin) })
	mux.HandleFunc("/"+asset+".sha256", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(hex.EncodeToString(sum[:])))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	if _, err := installGPParser(srv.Client(), srv.URL, t.TempDir()); err != nil {
		t.Fatal(err)
	}
}

// TestSha256sumEntry: sha256sum parsing accepts the two-space separator,
// the "*" binary-mode marker, and "./" path prefixes.
func TestSha256sumEntry(t *testing.T) {
	want := sha256.Sum256([]byte("x"))
	asset := "gp-parser-linux-x86_64"
	data := []byte(fmt.Sprintf("%x  %s\n%x *./dist/%s\n", want, asset, want, "other"))
	got, ok := sha256sumEntry(data, asset)
	if !ok || !bytes.Equal(got, want[:]) {
		t.Fatalf("entry not found: ok=%v", ok)
	}
	data2 := []byte(fmt.Sprintf("%x *./dist/%s\n", want, asset))
	if got, ok := sha256sumEntry(data2, asset); !ok || !bytes.Equal(got, want[:]) {
		t.Fatalf("binary-mode entry not found: ok=%v", ok)
	}
	if _, ok := sha256sumEntry(data, "gp-parser-macos-arm64"); ok {
		t.Fatal("absent asset must not match")
	}
}

func TestRunHelpExitsZero(t *testing.T) {
	code, _, _ := run("-h")
	if code != 0 {
		t.Fatalf("code=%d, want 0 for -h", code)
	}
}

func TestWriteCrashLog(t *testing.T) {
	testutil.RedirectConfigDir(t)
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
