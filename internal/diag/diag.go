// Package diag powers the `fretboard doctor` subcommand (wired in Wave 2):
// environment checks plus a shared error taxonomy that turns player/parser
// failures into a remediation class and an exact fix line.
package diag

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"fretboard/internal/config"
)

// CheckResult is one `fretboard doctor` check: what was checked, whether it
// passed, what was observed, and how to fix a failure. OK=false covers both
// hard failures and warnings (e.g. a stale yt-dlp); Detail says which.
type CheckResult struct {
	Name   string
	OK     bool
	Detail string
	Fix    string
}

// versionRE finds the first dotted numeric version in a tool banner line,
// e.g. "0.41.0" in mpv's or "6.1.1" in ffmpeg's.
var versionRE = regexp.MustCompile(`\d+\.\d+(\.\d+)?`)

// ytDlpDateRE matches the date prefix of a stable yt-dlp release. Releases
// are dated YYYY.MM.DD; dev builds append suffixes ("2026.07.04.1",
// "2026.07.04-1") which the anchored regex tolerates.
var ytDlpDateRE = regexp.MustCompile(`^(\d{4})\.(\d{1,2})\.(\d{1,2})`)

// staleAfter is how old a yt-dlp install must be before `fretboard doctor`
// warns. YouTube's HTML changes roughly monthly, so 90 days is generous.
const staleAfter = 90 * 24 * time.Hour

// ytProbeTimeout bounds the live connectivity probe. It is a variable so
// tests can fake a slow yt-dlp without waiting the full duration.
var ytProbeTimeout = 15 * time.Second

// binaryChecks lists the tools checked for PATH presence. versionArg is the
// flag that prints a version banner, or "" when the tool has no cheap probe
// (ffplay). Results are emitted in this order.
var binaryChecks = []struct {
	name, versionArg string
}{
	{"mpv", "--version"},
	{"ffplay", ""},
	{"fluidsynth", "--version"},
	{"timidity", "--version"},
	{"ffmpeg", "-version"},
}

// RunChecks runs every `fretboard doctor` check. Each check stands alone: a
// failure never aborts the rest, absent tools yield structured FAIL results
// instead of errors, and order is stable for rendering.
func RunChecks() []CheckResult {
	var out []CheckResult
	out = append(out, checkYtDlp())
	for _, b := range binaryChecks {
		out = append(out, checkBinary(b.name, b.versionArg))
	}
	out = append(out, checkSoundfont())
	if dir, err := config.Dir(); err == nil {
		out = append(out, checkDisk(dir, "disk space (config dir)"))
	} else {
		out = append(out, CheckResult{Name: "disk space (config dir)", Detail: "config dir unavailable: " + err.Error()})
	}
	if dir, err := config.AudioDir(); err == nil {
		out = append(out, checkDisk(dir, "disk space (audio dir)"))
	} else {
		out = append(out, CheckResult{Name: "disk space (audio dir)", Detail: "audio dir unavailable: " + err.Error()})
	}
	out = append(out, checkYtProbe())
	return out
}

// checkYtDlp verifies yt-dlp presence and, because yt-dlp versions are
// dates, warns when the install is old enough that YouTube changes are
// likely to have broken it. An unparseable version is itself a warning:
// we cannot vouch for staleness either way.
func checkYtDlp() CheckResult {
	r := CheckResult{Name: "yt-dlp"}
	path, err := exec.LookPath("yt-dlp")
	if err != nil {
		r.Detail = "not found on PATH"
		r.Fix = installHint("yt-dlp")
		return r
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "yt-dlp", "--version").Output()
	if err != nil {
		r.Detail = "WARN: found at " + path + " but version could not be determined — cannot tell if it is stale"
		r.Fix = "run `pip install -U yt-dlp`"
		return r
	}
	rel, ok := ytDlpVersion(string(out))
	if !ok {
		r.Detail = fmt.Sprintf("WARN: could not parse yt-dlp version %q — cannot tell if it is stale", strings.TrimSpace(string(out)))
		r.Fix = "run `pip install -U yt-dlp`"
		return r
	}
	if age := time.Since(rel); age > staleAfter {
		r.Detail = fmt.Sprintf("WARN: yt-dlp %s is %d days old — YouTube changes often break older builds",
			rel.Format("2006.01.02"), int(age.Hours()/24))
		r.Fix = "run `pip install -U yt-dlp` (or `yt-dlp -U`)"
		return r
	}
	r.Detail = fmt.Sprintf("found: %s (version %s)", path, rel.Format("2006.01.02"))
	r.OK = true
	return r
}

// ytDlpVersion parses a stable yt-dlp release date from `--version` output,
// returning ok=false when the line is not a date.
func ytDlpVersion(out string) (time.Time, bool) {
	m := ytDlpDateRE.FindStringSubmatch(strings.TrimSpace(out))
	if m == nil {
		return time.Time{}, false
	}
	y, _ := strconv.Atoi(m[1])
	mo, _ := strconv.Atoi(m[2])
	d, _ := strconv.Atoi(m[3])
	if y < 2000 || y > 2100 || mo < 1 || mo > 12 || d < 1 || d > 31 {
		return time.Time{}, false
	}
	return time.Date(y, time.Month(mo), d, 0, 0, 0, 0, time.UTC), true
}

// checkBinary reports a tool's presence on PATH and, when it has a cheap
// version probe, the first dotted version found in its banner. A failed
// version probe is noted but does not fail the check: presence is what the
// player needs.
func checkBinary(name, versionArg string) CheckResult {
	r := CheckResult{Name: name}
	path, err := exec.LookPath(name)
	if err != nil {
		r.Detail = "not found on PATH"
		r.Fix = installHint(name)
		return r
	}
	r.Detail = "found: " + path
	if versionArg == "" {
		r.OK = true
		return r
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, versionArg).Output()
	if err != nil {
		r.Detail += " (version probe failed: " + err.Error() + ")"
		r.OK = true
		return r
	}
	if v := versionRE.FindString(string(out)); v != "" {
		r.Detail += " (" + v + ")"
	}
	r.OK = true
	return r
}

// checkSoundfont verifies a GM soundfont is discoverable, mirroring the
// player's resolution order: config path, FRETBOARD_SOUNDFONT, then platform
// and user directories with the well-known GM names and a *.sf2/*.sf3 glob
// fallback. The lookup is re-implemented here (rather than importing player)
// so diag stays independent of that package's in-flight changes.
func checkSoundfont() CheckResult {
	r := CheckResult{Name: "soundfont"}
	if cfg, err := config.Load(); err == nil && cfg.Soundfont != "" {
		if fileExists(cfg.Soundfont) {
			r.OK = true
			r.Detail = "found: " + cfg.Soundfont + " (from config)"
			return r
		}
		r.Detail = "config points at missing file: " + cfg.Soundfont
		r.Fix = "point soundfont in config at an existing file, or install a GM soundfont (see README)"
		return r
	}
	if sf := os.Getenv("FRETBOARD_SOUNDFONT"); sf != "" {
		if fileExists(sf) {
			r.OK = true
			r.Detail = "found: " + sf + " (from FRETBOARD_SOUNDFONT)"
			return r
		}
		r.Detail = "FRETBOARD_SOUNDFONT points at missing file: " + sf
		r.Fix = "set FRETBOARD_SOUNDFONT to an existing file, or install a GM soundfont (see README)"
		return r
	}
	dirs := soundfontSearchDirs()
	names := []string{"FluidR3_GM.sf2", "GeneralUser_GS.sf2", "default.sf2", "default_gs.sf2"}
	for _, dir := range dirs {
		for _, name := range names {
			if p := filepath.Join(dir, name); fileExists(p) {
				r.OK = true
				r.Detail = "found: " + p
				return r
			}
		}
	}
	for _, dir := range dirs {
		for _, pattern := range []string{"*.sf2", "*.sf3"} {
			if matches, _ := filepath.Glob(filepath.Join(dir, pattern)); len(matches) > 0 {
				r.OK = true
				r.Detail = "found: " + matches[0]
				return r
			}
		}
	}
	r.Detail = "no GM soundfont found; searched: " + strings.Join(dirs, ", ")
	r.Fix = "install a GM soundfont (e.g. soundfont-fluid) or set FRETBOARD_SOUNDFONT"
	return r
}

// soundfontSearchDirs returns the directories the player would search for GM
// soundfonts, most-specific first.
func soundfontSearchDirs() []string {
	var dirs []string
	switch runtime.GOOS {
	case "darwin":
		dirs = append(dirs, "/opt/homebrew/share/soundfonts", "/usr/local/share/soundfonts")
	case "linux":
		dirs = append(dirs, "/usr/share/soundfonts", "/usr/share/sounds/sf2")
	}
	if cfg, err := os.UserConfigDir(); err == nil {
		dirs = append(dirs, filepath.Join(cfg, "fretboard"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs,
			filepath.Join(home, ".local", "share", "soundfonts"),
			filepath.Join(home, ".config", "fretboard"),
		)
	}
	return dirs
}

// checkDisk warns when the filesystem holding dir has under 500MB free —
// the audio cache and tab store can grow quickly.
func checkDisk(dir, name string) CheckResult {
	r := CheckResult{Name: name}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		r.Detail = "cannot access dir " + dir + ": " + err.Error()
		return r
	}
	free, ok := diskFree(dir)
	if !ok {
		r.Detail = dir + ": free space unknown on this platform"
		r.OK = true
		return r
	}
	const warnBelow = int64(500) << 20 // 500 MiB
	if free < warnBelow {
		r.Detail = fmt.Sprintf("%s: only %s free", dir, humanBytes(free))
		r.Fix = "free up disk space (downloaded audio and the tab store live here)"
		return r
	}
	r.Detail = fmt.Sprintf("%s: %s free", dir, humanBytes(free))
	r.OK = true
	return r
}

// checkYtProbe runs a live yt-dlp search to confirm YouTube is reachable and
// not bot-checking us. The result is classified ok / bot-check / timeout /
// network-down; when yt-dlp is absent it degrades to a structured skip
// instead of failing the whole doctor run.
func checkYtProbe() CheckResult {
	r := CheckResult{Name: "yt-dlp probe"}
	if _, err := exec.LookPath("yt-dlp"); err != nil {
		r.Detail = "skipped: yt-dlp not installed"
		r.Fix = installHint("yt-dlp")
		return r
	}
	args := []string{"ytsearch1:test", "--dump-single-json", "--no-warnings", "--flat-playlist", "--quiet"}
	ctx, cancel := context.WithTimeout(context.Background(), ytProbeTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "yt-dlp", args...).Output()
	if err == nil {
		var playlist struct {
			Entries []json.RawMessage `json:"entries"`
		}
		// Even an unparseable or empty reply means yt-dlp ran and reached
		// YouTube; connectivity is what the probe measures.
		if json.Unmarshal(out, &playlist) == nil && len(playlist.Entries) > 0 {
			r.Detail = "ok: reached YouTube search"
		} else {
			r.Detail = "ok: yt-dlp responded (no search hits)"
		}
		r.OK = true
		return r
	}
	// yt-dlp writes its diagnosis to stdout/stderr before exiting non-zero;
	// fold both into the error so the signature matcher sees the real
	// message (CommandContext's own error is just "exit status N").
	probeErr := err
	if msg := strings.TrimSpace(string(out)); msg != "" {
		probeErr = fmt.Errorf("%v: %s", err, msg)
	} else if ee, ok := err.(*exec.ExitError); ok {
		if msg := strings.TrimSpace(string(ee.Stderr)); msg != "" {
			probeErr = fmt.Errorf("%v: %s", err, msg)
		}
	}
	switch probeClassify(ctx, probeErr) {
	case "timeout":
		r.Detail = "timeout: yt-dlp did not respond within " + ytProbeTimeout.String()
		r.Fix = "check your network connectivity and retry"
	case "bot-check":
		// Rate limits and bot checks are both "YouTube is blocking the
		// probe"; reuse the taxonomy for the exact remediation text.
		class, fix := ClassifyError(probeErr)
		if class == "TRANSIENT" {
			r.Detail = "rate-limited: YouTube is blocking the search probe"
		} else {
			r.Detail = "bot-check: YouTube is blocking the search or yt-dlp is outdated"
		}
		r.Fix = fix
	default:
		r.Detail = "network-down: " + err.Error()
		r.Fix = "check your network / YouTube reachability and retry"
	}
	return r
}

// probeClassify buckets a probe failure into timeout / bot-check /
// network-down. The deadline is read from the probe context because
// CommandContext reports the kill (e.g. "signal: killed"), not the
// DeadlineExceeded error itself.
func probeClassify(ctx context.Context, err error) string {
	if ctx.Err() != nil || errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	if _, ok := KnownFailure(err); ok {
		return "bot-check"
	}
	return "network-down"
}

// installHint returns the platform-appropriate install command for a missing
// tool, matching the hints the player already surfaces elsewhere.
func installHint(name string) string {
	switch name {
	case "yt-dlp":
		return "install yt-dlp (pip install -U yt-dlp, apt install yt-dlp, or choco install yt-dlp)"
	case "mpv":
		return "install mpv (apt install mpv, brew install mpv, or choco install mpv)"
	case "ffplay":
		return "install ffmpeg — ffplay ships with it (apt install ffmpeg, brew install ffmpeg)"
	case "fluidsynth":
		return "install fluidsynth (apt install fluidsynth, brew install fluid-synth)"
	case "timidity":
		return "install timidity (apt install timidity)"
	case "ffmpeg":
		return "install ffmpeg (apt install ffmpeg, brew install ffmpeg)"
	}
	return "install " + name
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func humanBytes(n int64) string {
	const unit = int64(1024)
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := unit, 0
	for x := n / unit; x >= unit && exp < len("KMGTPE")-1; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// failureSpec maps one error signature to its remediation class and exact
// fix line. matches are case-insensitive substrings of err.Error(); is
// covers signatures that need errors.Is/As (e.g. context.DeadlineExceeded).
type failureSpec struct {
	name    string
	matches []string
	is      func(error) bool
	class   string
	fix     string
}

// failureSpecs is ordered: the first matching signature wins, so specific
// YouTube signatures are tried before the generic ones.
var failureSpecs = []failureSpec{
	{
		name:    "yt-dlp bot-check",
		matches: []string{"sign in to confirm", "recaptcha", "bot"},
		class:   "USER_FIXABLE",
		fix:     "yt-dlp is likely outdated or YouTube changed — run `pip install -U yt-dlp`",
	},
	{
		name:    "http rate-limit",
		matches: []string{"http error 403", "http error 429", "403: forbidden", "429: too many requests"},
		class:   "TRANSIENT",
		fix:     "YouTube is rate-limiting — wait a few minutes and retry",
	},
	{
		name:    "yt-dlp extraction failure",
		matches: []string{"unable to extract"},
		class:   "USER_FIXABLE",
		fix:     "yt-dlp is outdated for the current YouTube page — run `pip install -U yt-dlp`",
	},
	{
		name:    "outdated player response",
		matches: []string{"nsig", "player response"},
		class:   "USER_FIXABLE",
		fix:     "yt-dlp is outdated — run `pip install -U yt-dlp`",
	},
	{
		name:    "fluidsynth no soundfont",
		matches: []string{"no sound font", "no soundfont", "soundfont not found"},
		class:   "USER_FIXABLE",
		fix:     "install a GM soundfont (see README)",
	},
	{
		name:    "missing gp parser helper",
		matches: []string{"gp-parser", "gp parser", "fretboard setup gp", "fretboard_gp_parser"},
		class:   "USER_FIXABLE",
		fix:     "run `fretboard setup gp` or set FRETBOARD_GP_PARSER",
	},
	{
		name:    "context deadline",
		matches: []string{"context deadline exceeded", "timed out"},
		is:      func(err error) bool { return errors.Is(err, context.DeadlineExceeded) },
		class:   "TRANSIENT",
		fix:     "the operation timed out — check your network and retry",
	},
}

// matchSpec walks failureSpecs and returns the first spec whose signature
// matches err. Wrapped errors classify because err.Error() carries the whole
// chain and errors.Is/As unwraps it.
func matchSpec(err error) (failureSpec, bool) {
	msg := strings.ToLower(err.Error())
	for _, spec := range failureSpecs {
		if spec.is != nil && spec.is(err) {
			return spec, true
		}
		for _, m := range spec.matches {
			if strings.Contains(msg, m) {
				return spec, true
			}
		}
	}
	return failureSpec{}, false
}

// KnownFailure returns the remediation text for a recognized error signature
// and whether one matched. Errors may be wrapped; matching uses errors.Is/As
// plus a case-insensitive substring scan as fallback.
func KnownFailure(err error) (fix string, ok bool) {
	if err == nil {
		return "", false
	}
	spec, ok := matchSpec(err)
	return spec.fix, ok
}

// ClassifyError buckets any error into a remediation class (USER_FIXABLE,
// TRANSIENT, or PERMANENT) plus the exact remediation text. Unknown errors
// fall back to PERMANENT with the raw message so callers can always render a
// fix line.
func ClassifyError(err error) (class, fix string) {
	if err == nil {
		return "", ""
	}
	spec, ok := matchSpec(err)
	if ok {
		return spec.class, spec.fix
	}
	return "PERMANENT", "unexpected error — check the logs and report it: " + err.Error()
}
