package diag

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"fretboard/internal/testutil"
)

// writeFakeBins writes executable shims for the given commands into one temp
// dir prepended to PATH, mirroring the player's fakebin_* helpers: a .cmd
// loop on Windows, an executable shell script elsewhere. Every script is
// expected to respond to a "--version"-style probe when given one; extra
// args (the probe) hit the script body.
func writeFakeBins(t *testing.T, scripts map[string]string) {
	t.Helper()
	dir := t.TempDir()
	for name, script := range scripts {
		if runtime.GOOS == "windows" {
			name += ".cmd"
			script = "@echo off\r\n" + script
		}
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// unixYtDlp builds a yt-dlp shim: fast --version, then $body for the probe.
func unixYtDlp(version, body string) string {
	return fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then\n  echo %s\n  exit 0\nfi\n%s", version, body)
}

// winYtDlp is the Windows twin of unixYtDlp.
func winYtDlp(version, body string) string {
	return fmt.Sprintf("if \"%%1\"==\"--version\" (\n  echo %s\n  exit /b 0\n)\n%s", version, body)
}

func fakeYtDlp(version, body string) string {
	if runtime.GOOS == "windows" {
		return winYtDlp(version, body)
	}
	return unixYtDlp(version, body)
}

func recentVersion() string {
	return time.Now().AddDate(0, 0, -10).Format("2006.01.02")
}

func staleVersion() string {
	return time.Now().AddDate(0, -6, 0).Format("2006.01.02")
}

func resultByName(t *testing.T, results []CheckResult, name string) CheckResult {
	t.Helper()
	for _, r := range results {
		if r.Name == name {
			return r
		}
	}
	t.Fatalf("no check result named %q in %v", name, names(results))
	return CheckResult{}
}

func names(results []CheckResult) []string {
	out := make([]string, len(results))
	for i, r := range results {
		out[i] = r.Name
	}
	return out
}

// TestRunChecksAllPresent drives the happy path: every dependency shimmed on
// PATH, a soundfont in the (redirected) config dir, and a fast probe. Every
// check must come back OK and the result set must be complete and stable.
func TestRunChecksAllPresent(t *testing.T) {
	testutil.RedirectConfigDir(t)
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	sf := filepath.Join(cfgDir, "fretboard", "FluidR3_GM.sf2")
	if err := os.MkdirAll(filepath.Dir(sf), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sf, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}

	okProbe := `printf '{"entries":[{"id":"x","title":"test"}]}\n'`
	if runtime.GOOS == "windows" {
		okProbe = "echo {\"entries\":[{\"id\":\"x\",\"title\":\"test\"}]}"
	}
	writeFakeBins(t, map[string]string{
		"yt-dlp":     fakeYtDlp(recentVersion(), okProbe),
		"mpv":        "echo mpv 0.41.0 Copyright",
		"ffplay":     "exit 0",
		"fluidsynth": "echo FluidSynth version 2.5.6",
		"timidity":   "echo TiMidity++ version 2.15.0",
		"ffmpeg":     "echo ffmpeg version 6.1.1",
	})

	results := RunChecks()
	want := []string{
		"yt-dlp", "mpv", "ffplay", "fluidsynth", "timidity", "ffmpeg",
		"soundfont", "disk space (config dir)", "disk space (audio dir)", "yt-dlp probe",
	}
	if len(results) != len(want) {
		t.Fatalf("RunChecks returned %d results, want %d: %v", len(results), len(want), names(results))
	}
	for i, w := range want {
		if results[i].Name != w {
			t.Fatalf("result %d = %q, want %q (full list: %v)", i, results[i].Name, w, names(results))
		}
	}
	for _, r := range results {
		if !r.OK {
			t.Errorf("check %q should pass: Detail=%q Fix=%q", r.Name, r.Detail, r.Fix)
		}
	}
	if r := resultByName(t, results, "yt-dlp"); !strings.Contains(r.Detail, "found:") {
		t.Errorf("yt-dlp detail should mention the path, got %q", r.Detail)
	}
}

// TestRunChecksAllMissing runs with an empty PATH: every dependency must
// still yield a structured FAIL (never a panic), and the probe must degrade
// to a structured skip rather than erroring the run.
func TestRunChecksAllMissing(t *testing.T) {
	testutil.RedirectConfigDir(t)
	t.Setenv("PATH", t.TempDir()) // empty dir: nothing resolves

	results := RunChecks()
	if r := resultByName(t, results, "yt-dlp"); r.OK {
		t.Errorf("yt-dlp should be FAIL with empty PATH, got %+v", r)
	}
	probe := resultByName(t, results, "yt-dlp probe")
	if probe.OK {
		t.Errorf("probe should be FAIL when yt-dlp is absent, got %+v", probe)
	}
	if !strings.Contains(strings.ToLower(probe.Detail), "skipped") {
		t.Errorf("probe detail should say it was skipped, got %q", probe.Detail)
	}
	// Every named dependency must be present even when nothing is installed.
	for _, name := range []string{"mpv", "ffplay", "fluidsynth", "timidity", "ffmpeg", "soundfont"} {
		resultByName(t, results, name)
	}
}

// TestRunChecksProbeTimeout fakes a yt-dlp that hangs on the probe (but
// answers --version quickly) and asserts the probe is classified as a
// timeout instead of hanging the whole run.
func TestRunChecksProbeTimeout(t *testing.T) {
	old := ytProbeTimeout
	ytProbeTimeout = 300 * time.Millisecond
	t.Cleanup(func() { ytProbeTimeout = old })

	var slowProbe string
	if runtime.GOOS == "windows" {
		slowProbe = "ping -n 31 127.0.0.1 >nul"
	} else {
		slowProbe = "exec sleep 30"
	}
	writeFakeBins(t, map[string]string{"yt-dlp": fakeYtDlp(recentVersion(), slowProbe)})

	results := RunChecks()
	probe := resultByName(t, results, "yt-dlp probe")
	if probe.OK {
		t.Fatalf("slow probe should fail, got %+v", probe)
	}
	if !strings.Contains(probe.Detail, "timeout") {
		t.Errorf("probe detail should mention timeout, got %q", probe.Detail)
	}
}

// TestRunChecksProbeBotCheck fakes a yt-dlp that answers --version but fails
// the probe with YouTube's bot-check message.
func TestRunChecksProbeBotCheck(t *testing.T) {
	var botProbe string
	if runtime.GOOS == "windows" {
		botProbe = "echo ERROR: [youtube] abc: Sign in to confirm you're not a bot.\r\nexit /b 1"
	} else {
		botProbe = "echo \"ERROR: [youtube] abc: Sign in to confirm you're not a bot.\"\nexit 1"
	}
	writeFakeBins(t, map[string]string{"yt-dlp": fakeYtDlp(recentVersion(), botProbe)})

	results := RunChecks()
	probe := resultByName(t, results, "yt-dlp probe")
	if probe.OK {
		t.Fatalf("bot-checked probe should fail, got %+v", probe)
	}
	if !strings.Contains(probe.Detail, "bot-check") {
		t.Errorf("probe detail should mention bot-check, got %q", probe.Detail)
	}
	if !strings.Contains(probe.Fix, "pip install -U yt-dlp") {
		t.Errorf("probe fix should suggest updating yt-dlp, got %q", probe.Fix)
	}
}

// TestRunChecksStaleYtDlp installs a yt-dlp whose release date is six months
// old; the check must warn (OK=false) with an update hint.
func TestRunChecksStaleYtDlp(t *testing.T) {
	writeFakeBins(t, map[string]string{"yt-dlp": fakeYtDlp(staleVersion(), "exit 1")})
	results := RunChecks()
	r := resultByName(t, results, "yt-dlp")
	if r.OK {
		t.Fatalf("stale yt-dlp should warn, got %+v", r)
	}
	if !strings.Contains(r.Detail, "WARN") || !strings.Contains(r.Detail, "days old") {
		t.Errorf("stale detail should warn about age, got %q", r.Detail)
	}
	if !strings.Contains(r.Fix, "pip install -U yt-dlp") {
		t.Errorf("stale fix should suggest updating, got %q", r.Fix)
	}
}

// TestRunChecksVersionUnknown installs a yt-dlp whose --version is not a
// date; the check must flag the unknown version as a warning.
func TestRunChecksVersionUnknown(t *testing.T) {
	writeFakeBins(t, map[string]string{"yt-dlp": fakeYtDlp("git HEAD", "exit 1")})
	results := RunChecks()
	r := resultByName(t, results, "yt-dlp")
	if r.OK {
		t.Fatalf("unparseable yt-dlp version should warn, got %+v", r)
	}
	if !strings.Contains(r.Detail, "could not parse") {
		t.Errorf("detail should say the version could not be parsed, got %q", r.Detail)
	}
}

func TestYtDlpVersion(t *testing.T) {
	good := time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		in   string
		want time.Time
		ok   bool
	}{
		{"2026.07.04", good, true},
		{"2026.07.04\n", good, true},
		{" 2026.7.4 ", time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC), true},
		{"2026.07.04.123", good, true}, // dev build suffix
		{"2026.07.04-1", good, true},   // distro suffix
		{"git HEAD", time.Time{}, false},
		{"latest", time.Time{}, false},
		{"", time.Time{}, false},
	} {
		got, ok := ytDlpVersion(tc.in)
		if ok != tc.ok || (ok && !got.Equal(tc.want)) {
			t.Errorf("ytDlpVersion(%q) = %v, %v; want %v, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

type failureCase struct {
	name        string
	err         error
	ok          bool
	class       string
	fixContains string
}

// failureCases exercises every KnownFailure signature: the exact messages
// the tools emit, a wrapped variant, and an unknown error.
func failureCases() []failureCase {
	return []failureCase{
		{"bot-check sign in", errors.New("ERROR: [youtube] abc: Sign in to confirm you're not a bot"), true, "USER_FIXABLE", "pip install -U yt-dlp"},
		{"bot-check recaptcha", errors.New("ERROR: [youtube] abc: reCAPTCHA check required"), true, "USER_FIXABLE", "pip install -U yt-dlp"},
		{"bot-check bot", errors.New("YouTube is blocking this request as a bot"), true, "USER_FIXABLE", "pip install -U yt-dlp"},
		{"http 403", errors.New("ERROR: Unable to download webpage: HTTP Error 403: Forbidden"), true, "TRANSIENT", "rate-limiting"},
		{"http 429", errors.New("HTTP Error 429: Too Many Requests"), true, "TRANSIENT", "rate-limiting"},
		{"unable to extract", errors.New("ERROR: [youtube] xyz: Unable to extract player response"), true, "USER_FIXABLE", "pip install -U yt-dlp"},
		{"nsig", errors.New("ERROR: [youtube] abc: nsig extraction failed"), true, "USER_FIXABLE", "pip install -U yt-dlp"},
		{"player response", errors.New("ERROR: [youtube] abc: player response is not valid JSON"), true, "USER_FIXABLE", "pip install -U yt-dlp"},
		{"fluidsynth no sound font", errors.New("No sound font is set. Please use the command line option"), true, "USER_FIXABLE", "install a GM soundfont (see README)"},
		{"gp parser missing", errors.New("gp-parser: executable not found"), true, "USER_FIXABLE", "fretboard setup gp"},
		{"gp parser env", errors.New("FRETBOARD_GP_PARSER not set"), true, "USER_FIXABLE", "FRETBOARD_GP_PARSER"},
		{"context deadline", context.DeadlineExceeded, true, "TRANSIENT", "timed out"},
		{"context deadline string", errors.New("context deadline exceeded"), true, "TRANSIENT", "timed out"},
		{"search timed out", errors.New("yt-dlp search timed out"), true, "TRANSIENT", "timed out"},
		{"wrapped", fmt.Errorf("search failed: %w", errors.New("Sign in to confirm you're not a bot")), true, "USER_FIXABLE", "pip install -U yt-dlp"},
		{"wrapped deadline", fmt.Errorf("download aborted: %w", context.DeadlineExceeded), true, "TRANSIENT", "timed out"},
		{"unknown", errors.New("something entirely unexpected"), false, "PERMANENT", ""},
	}
}

func TestKnownFailure(t *testing.T) {
	for _, tc := range failureCases() {
		fix, ok := KnownFailure(tc.err)
		if ok != tc.ok {
			t.Errorf("%s: KnownFailure ok = %v, want %v (err %q)", tc.name, ok, tc.ok, tc.err)
			continue
		}
		if ok && !strings.Contains(fix, tc.fixContains) {
			t.Errorf("%s: fix %q should contain %q", tc.name, fix, tc.fixContains)
		}
	}
	if fix, ok := KnownFailure(nil); ok || fix != "" {
		t.Errorf("KnownFailure(nil) = %q, %v; want \"\", false", fix, ok)
	}
}

func TestClassifyError(t *testing.T) {
	for _, tc := range failureCases() {
		class, fix := ClassifyError(tc.err)
		if class != tc.class {
			t.Errorf("%s: class = %q, want %q", tc.name, class, tc.class)
		}
		if tc.ok && !strings.Contains(fix, tc.fixContains) {
			t.Errorf("%s: fix %q should contain %q", tc.name, fix, tc.fixContains)
		}
		if !tc.ok && !strings.Contains(fix, "unexpected error") {
			t.Errorf("%s: unknown errors should get the fallback fix, got %q", tc.name, fix)
		}
	}
	if class, fix := ClassifyError(nil); class != "" || fix != "" {
		t.Errorf("ClassifyError(nil) = %q, %q; want empty", class, fix)
	}
}

// TestClassifyErrorOrdering guards the signature precedence in the table: a
// message carrying two signatures must classify by the first one listed. The
// pinned cases are the ones a reordering would observably change: the 403
// rate-limit (listed before "unable to extract"/"player response") and the
// extraction failure (listed before the generic "player response").
func TestClassifyErrorOrdering(t *testing.T) {
	for _, tc := range []struct{ err, want string }{
		{"ERROR: [youtube] abc: player response fetch failed: HTTP Error 403: Forbidden", "TRANSIENT"},
		{"ERROR: [youtube] xyz: Unable to extract player response", "USER_FIXABLE"},
	} {
		class, _ := ClassifyError(errors.New(tc.err))
		if class != tc.want {
			t.Errorf("ClassifyError(%q) = %q, want %q", tc.err, class, tc.want)
		}
	}
}
