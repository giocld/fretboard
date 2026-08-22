// Subcommand implementations wired in Wave 2: the --print/--html render
// flags, `doctor`, `scan`, `export`/`import` archive round-trip, and
// `setup gp`. Dispatch lives in cli.go; the helpers here stay free of
// library-store dependencies where the command does not need one (doctor,
// setup gp, render) so they run without touching the user's database.
package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"fretboard/internal/config"
	"fretboard/internal/diag"
	"fretboard/internal/export"
	"fretboard/internal/library"
	"fretboard/internal/model"
	"fretboard/internal/parser"
)

// exeSuffix is ".exe" on Windows and "" elsewhere; release binaries follow
// the platform's native naming.
var exeSuffix = func() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}()

// printUsage is the -h text: the subcommand surface plus the shared flags.
func printUsage(w io.Writer, fs *flag.FlagSet) {
	fmt.Fprintln(w, "fretboard — guitar tab manager")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "usage: fretboard [flags] [tab-file]")
	fmt.Fprintln(w, "       fretboard <command> [args]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "commands:")
	fmt.Fprintln(w, "  import <file-or-dir|archive.json>  import tabs, or restore a library archive (.json)")
	fmt.Fprintln(w, "  doctor [check-name]                run environment checks; exit 1 when any check FAILs")
	fmt.Fprintln(w, "  scan [dir]                         relink moved tab files (default: tabs_dir, else the most recent tab's folder)")
	fmt.Fprintln(w, "  export <archive.json>              back up the library to an archive")
	fmt.Fprintln(w, "  setup gp [--version X]             install the Guitar Pro parser binary")
	fmt.Fprintln(w, "  test-audio                         play three test notes")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "render:")
	fmt.Fprintln(w, "  --print file [--width N]           render a tab as paginated plain text to stdout (pipe to lpr)")
	fmt.Fprintln(w, "  --html file [--theme NAME] [-o out.html]   render a tab as a self-contained HTML page")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "flags:")
	fs.PrintDefaults()
}

// ---------- render: --print / --html ----------

// runRender implements `fretboard --print|--html <file> [flags]`: parse the
// tab (ASCII or Guitar Pro) and write PrintTab/HTMLTab to stdout or to -o.
func runRender(args []string, stdout, stderr io.Writer) int {
	mode, path, theme, outPath, width, err := parseRenderArgs(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprintln(stdout, "usage: fretboard --print file [--width N]")
			fmt.Fprintln(stdout, "       fretboard --html file [--theme NAME] [-o out.html]")
			return 0
		}
		fmt.Fprintf(stderr, "%v\n", err)
		return 2
	}
	tab, err := loadRenderTab(path)
	if err != nil {
		fmt.Fprintf(stderr, "parse: %v\n", err)
		return 1
	}
	var text string
	if mode == "print" {
		text = export.PrintTab(tab, width)
	} else {
		text = export.HTMLTab(tab, theme)
	}
	if outPath != "" {
		if err := os.WriteFile(outPath, []byte(text), 0o644); err != nil {
			fmt.Fprintf(stderr, "render: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "wrote %s\n", outPath)
		return 0
	}
	fmt.Fprint(stdout, text)
	return 0
}

// parseRenderArgs walks the render arg list manually because the file path
// may sit between flags (--print file.txt --width 80), which the flag
// package cannot express.
func parseRenderArgs(args []string) (mode, path, theme, outPath string, width int, err error) {
	theme = "default"
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--print":
			mode = "print"
		case a == "--html":
			mode = "html"
		case a == "-h" || a == "--help":
			return "", "", "", "", 0, flag.ErrHelp
		case a == "--width":
			if i+1 >= len(args) {
				return "", "", "", "", 0, errors.New("--width needs a value")
			}
			i++
			width, err = strconv.Atoi(args[i])
			if err != nil || width < 0 {
				return "", "", "", "", 0, fmt.Errorf("invalid --width %q", args[i])
			}
		case strings.HasPrefix(a, "--width="):
			width, err = strconv.Atoi(strings.TrimPrefix(a, "--width="))
			if err != nil || width < 0 {
				return "", "", "", "", 0, fmt.Errorf("invalid --width %q", strings.TrimPrefix(a, "--width="))
			}
		case a == "--theme":
			if i+1 >= len(args) {
				return "", "", "", "", 0, errors.New("--theme needs a value")
			}
			i++
			theme = args[i]
		case strings.HasPrefix(a, "--theme="):
			theme = strings.TrimPrefix(a, "--theme=")
		case a == "-o" || a == "--output":
			if i+1 >= len(args) {
				return "", "", "", "", 0, errors.New("-o needs a value")
			}
			i++
			outPath = args[i]
		case strings.HasPrefix(a, "--output="):
			outPath = strings.TrimPrefix(a, "--output=")
		case strings.HasPrefix(a, "-o="):
			outPath = strings.TrimPrefix(a, "-o=")
		default:
			if strings.HasPrefix(a, "-") {
				return "", "", "", "", 0, fmt.Errorf("render: unknown flag %s", a)
			}
			if path != "" {
				return "", "", "", "", 0, errors.New("render: too many file arguments")
			}
			path = a
		}
	}
	if err != nil {
		return "", "", "", "", 0, err
	}
	if mode == "" {
		return "", "", "", "", 0, errors.New("render: missing --print or --html")
	}
	if path == "" {
		return "", "", "", "", 0, errors.New("render: missing file path")
	}
	return mode, path, theme, outPath, width, nil
}

// loadRenderTab parses a tab for rendering: ASCII tabs via the text parser,
// Guitar Pro files via the track parser (the first track carries the full
// tab).
func loadRenderTab(path string) (*model.Tab, error) {
	if parser.IsGpFile(path) {
		tracks, err := parser.ParseGuitarProTracks(path)
		if err != nil {
			return nil, err
		}
		if len(tracks) == 0 || tracks[0].Tab == nil {
			return nil, fmt.Errorf("%s: first track has no tab data", path)
		}
		return tracks[0].Tab, nil
	}
	return parser.ParseFile(path)
}

// ---------- doctor ----------

// runDoctor runs the environment checks and renders the results table.
func runDoctor(filter string, stdout, stderr io.Writer) int {
	return renderDoctorTable(diag.RunChecks(), filter, stdout, stderr)
}

// renderDoctorTable renders check results as a table — check / status /
// detail / fix — honoring an optional name filter. The exit code is 1 when
// any shown check FAILed; warnings do not fail the run. diag marks warnings
// ("WARN:" prefix) and structured skips ("skipped:" prefix) in the Detail
// text with OK=false, so the status is derived from the detail.
func renderDoctorTable(results []diag.CheckResult, filter string, stdout, stderr io.Writer) int {
	shown := make([]diag.CheckResult, 0, len(results))
	for _, r := range results {
		if filter != "" && !strings.EqualFold(r.Name, filter) {
			continue
		}
		shown = append(shown, r)
	}
	if filter != "" && len(shown) == 0 {
		names := make([]string, 0, len(results))
		for _, r := range results {
			names = append(names, r.Name)
		}
		fmt.Fprintf(stderr, "doctor: no check named %q (available: %s)\n", filter, strings.Join(names, ", "))
		return 1
	}
	status := func(r diag.CheckResult) (string, bool) {
		if r.OK {
			return "ok", false
		}
		if strings.HasPrefix(r.Detail, "WARN:") || strings.HasPrefix(r.Detail, "skipped:") {
			return "WARN", false
		}
		return "FAIL", true
	}
	nameW, detailW := 0, 0
	for _, r := range shown {
		if len(r.Name) > nameW {
			nameW = len(r.Name)
		}
		if len(r.Detail) > detailW {
			detailW = len(r.Detail)
		}
	}
	code := 0
	fmt.Fprintf(stdout, "%-*s  %-5s  %-*s  %s\n", nameW, "check", "status", detailW, "detail", "fix")
	for _, r := range shown {
		st, fail := status(r)
		if fail {
			code = 1
		}
		fmt.Fprintf(stdout, "%-*s  %-5s  %-*s  %s\n", nameW, r.Name, st, detailW, r.Detail, r.Fix)
	}
	return code
}

// ---------- scan ----------

// runScan implements `fretboard scan [dir]`: relink moved tab files and
// report ambiguous and missing rows. The exit code is 0 unless the scan
// itself fails.
func runScan(cfg config.Config, store *library.Store, args []string, stdout, stderr io.Writer) int {
	if len(args) > 1 {
		fmt.Fprintln(stderr, "usage: fretboard scan [dir]")
		return 1
	}
	dir := ""
	if len(args) == 1 {
		dir = args[0]
	}
	dir, err := resolveScanDir(cfg, store, dir)
	if err != nil {
		fmt.Fprintf(stderr, "scan: %v\n", err)
		return 1
	}
	relocs, err := store.ScanForRelocations(dir)
	if err != nil {
		fmt.Fprintf(stderr, "scan: %v\n", err)
		return 1
	}
	missing, err := store.MissingRows()
	if err != nil {
		fmt.Fprintf(stderr, "scan: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "scanning %s\n", dir)
	relinked, ambiguous := 0, 0
	for _, r := range relocs {
		if r.Ambiguous {
			ambiguous++
		} else {
			relinked++
		}
	}
	// Group the ambiguous candidates by row for a readable listing.
	seen := map[int64]bool{}
	for _, r := range relocs {
		if !r.Ambiguous {
			continue
		}
		if !seen[r.RowID] {
			seen[r.RowID] = true
			title := fmt.Sprintf("row %d", r.RowID)
			if row, err := store.GetRow(r.RowID); err == nil && row.Title != "" {
				title = row.Title
			}
			fmt.Fprintf(stdout, "  ambiguous: %s (was %s)\n", title, r.Path)
		}
		fmt.Fprintf(stdout, "    candidate: %s\n", r.FoundAt)
	}
	fmt.Fprintf(stdout, "relinked %d\n", relinked)
	fmt.Fprintf(stdout, "ambiguous %d\n", ambiguous)
	fmt.Fprintf(stdout, "missing %d\n", len(missing))
	return 0
}

// resolveScanDir picks the directory to scan: the explicit argument, then
// config.TabsDir, then the folder of the most recently added local tab, then
// an error explaining how to point the scan somewhere.
func resolveScanDir(cfg config.Config, store *library.Store, arg string) (string, error) {
	if arg != "" {
		return arg, nil
	}
	if cfg.TabsDir != "" {
		return cfg.TabsDir, nil
	}
	rows, err := store.List()
	if err != nil {
		return "", err
	}
	var latest *library.TabRow
	for i := range rows {
		r := &rows[i]
		if r.Filepath == "" || strings.Contains(r.Filepath, "://") {
			continue
		}
		if latest == nil || r.ID > latest.ID {
			latest = r
		}
	}
	if latest == nil {
		return "", errors.New("no local tabs to scan — pass a directory, or set tabs_dir in the config")
	}
	return filepath.Dir(latest.Filepath), nil
}

// ---------- export / import archive ----------

// runExport implements `fretboard export <archive.json>`.
func runExport(store *library.Store, args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: fretboard export <archive.json>")
		return 1
	}
	if err := store.ExportArchive(args[0]); err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "exported library to %s\n", args[0])
	return 0
}

// runImportArchive implements `fretboard import <archive.json>`: restore the
// library and report which manifest files resolved locally and which still
// need copying into the tabs dir.
func runImportArchive(store *library.Store, path string, stdout, stderr io.Writer) int {
	unresolved, err := store.ImportArchive(path)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "imported library archive %s\n", path)
	if len(unresolved) == 0 {
		fmt.Fprintln(stdout, "all archive files resolved")
		return 0
	}
	fmt.Fprintf(stdout, "unresolved files (%d) — copy them into the tabs dir and re-import to relink:\n", len(unresolved))
	for _, p := range unresolved {
		fmt.Fprintf(stdout, "  %s\n", p)
	}
	return 0
}

// ---------- setup gp ----------

// gpReleaseBase is the GitHub latest-release download root for gp-parser.
// The release workflow (.github/workflows/release.yml) stages one binary per
// platform as "gp-parser-<os>-<arch>[.exe]" with os in {linux, macos,
// windows} and arch in {x86_64, aarch64}, plus a combined "sha256sums.txt"
// checksum file.
const gpReleaseBase = "https://github.com/giocld/fretboard/releases/latest/download"

// errChecksumMismatch marks a downloaded binary whose hash does not match
// the release checksum; the install is refused rather than executed.
var errChecksumMismatch = errors.New("gp-parser checksum mismatch")

// maxGPParserBytes bounds the binary download; the release builds are a few
// megabytes.
const maxGPParserBytes = 64 << 20

// parseSetupGPArgs parses `setup gp` arguments; only --version X is
// accepted.
func parseSetupGPArgs(args []string) (string, error) {
	version := ""
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--version":
			if i+1 >= len(args) {
				return "", errors.New("--version needs a value")
			}
			i++
			version = args[i]
		case strings.HasPrefix(args[i], "--version="):
			version = strings.TrimPrefix(args[i], "--version=")
		default:
			return "", fmt.Errorf("unknown argument %q", args[i])
		}
	}
	return version, nil
}

// runSetupGP implements `fretboard setup gp [--version X]`: download the
// gp-parser release binary for the current platform, verify its sha256, and
// install it under the config dir, recording the version in the config.
func runSetupGP(version string, stdout, stderr io.Writer) int {
	if env := os.Getenv("FRETBOARD_GP_PARSER"); env != "" {
		fmt.Fprintf(stdout, "FRETBOARD_GP_PARSER is set (%s) — skipping install; unset it to let `setup gp` manage the binary\n", env)
		return 0
	}
	cfgDir, err := config.Dir()
	if err != nil {
		fmt.Fprintf(stderr, "setup gp: %v\n", err)
		return 1
	}
	client := &http.Client{Timeout: 60 * time.Second}
	dest, err := installGPParser(client, gpReleaseBase, filepath.Join(cfgDir, "bin"))
	if err != nil {
		if errors.Is(err, errChecksumMismatch) {
			fmt.Fprintf(stderr, "setup gp: %v — refusing to install\n", err)
			return 1
		}
		fmt.Fprintf(stderr, "setup gp: %v\n", err)
		if asset, err := gpParserAssetName(); err == nil {
			fmt.Fprintf(stderr, "download %s from the GitHub releases page and set FRETBOARD_GP_PARSER to its path\n", asset)
		}
		return 1
	}
	if version == "" {
		version = "latest"
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(stderr, "setup gp: read config: %v\n", err)
		return 1
	}
	cfg.GPParserVersion = version
	if err := config.Save(cfg); err != nil {
		fmt.Fprintf(stderr, "setup gp: save config: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "installed gp-parser %s -> %s\n", version, dest)
	fmt.Fprintln(stdout, "add the bin dir to PATH (or point FRETBOARD_GP_PARSER at the binary) so the app finds it")
	return 0
}

// gpParserAssetName returns the release asset name for the current platform,
// e.g. "gp-parser-linux-x86_64" or "gp-parser-windows-x86_64.exe", or an
// error when the platform is not published by the release workflow.
func gpParserAssetName() (string, error) {
	var osName, archName string
	switch runtime.GOOS {
	case "linux":
		osName = "linux"
	case "darwin":
		osName = "macos"
	case "windows":
		osName = "windows"
	default:
		return "", fmt.Errorf("unsupported platform %s/%s (the release workflow publishes linux, macos, windows)", runtime.GOOS, runtime.GOARCH)
	}
	switch runtime.GOARCH {
	case "amd64":
		archName = "x86_64"
	case "arm64":
		archName = "aarch64"
	default:
		return "", fmt.Errorf("unsupported platform %s/%s (the release workflow publishes x86_64 and aarch64)", runtime.GOOS, runtime.GOARCH)
	}
	asset := fmt.Sprintf("gp-parser-%s-%s", osName, archName)
	if runtime.GOOS == "windows" {
		asset += ".exe"
	}
	return asset, nil
}

// installGPParser downloads the gp-parser binary for the current platform
// from base (a release download root), verifies its sha256 against the
// checksum published with the release, and installs it into destDir with the
// executable bit set. client is injectable so tests can serve a fake release
// via httptest.
func installGPParser(client *http.Client, base, destDir string) (string, error) {
	asset, err := gpParserAssetName()
	if err != nil {
		return "", err
	}
	bin, err := fetchURL(client, base+"/"+asset)
	if err != nil {
		return "", err
	}
	want, err := fetchChecksum(client, base, asset)
	if err != nil {
		return "", err
	}
	got := sha256.Sum256(bin)
	if !bytes.Equal(got[:], want) {
		return "", fmt.Errorf("%w: %s", errChecksumMismatch, asset)
	}
	dest := filepath.Join(destDir, "gp-parser"+exeSuffix)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", fmt.Errorf("install gp-parser: %w", err)
	}
	if err := os.WriteFile(dest, bin, 0o755); err != nil {
		return "", fmt.Errorf("install gp-parser: %w", err)
	}
	return dest, nil
}

// fetchChecksum returns the expected sha256 for asset. The release workflow
// publishes a combined "sha256sums.txt"; a per-asset "<asset>.sha256"
// sidecar is accepted as a fallback for manually staged releases.
func fetchChecksum(client *http.Client, base, asset string) ([]byte, error) {
	if sums, err := fetchURL(client, base+"/sha256sums.txt"); err == nil {
		if want, ok := sha256sumEntry(sums, asset); ok {
			return want, nil
		}
	}
	sidecar, err := fetchURL(client, base+"/"+asset+".sha256")
	if err != nil {
		return nil, fmt.Errorf("no checksum for %s: tried sha256sums.txt and %s.sha256 (%v)", asset, asset, err)
	}
	if want, ok := sha256sumEntry(sidecar, asset); ok {
		return want, nil
	}
	// A bare-hex sidecar ("<64 hex chars>") is also accepted.
	if hexStr := strings.TrimSpace(string(sidecar)); len(hexStr) == hex.EncodedLen(sha256.Size) {
		if want, err := hex.DecodeString(hexStr); err == nil {
			return want, nil
		}
	}
	return nil, fmt.Errorf("unparseable checksum sidecar for %s", asset)
}

// sha256sumEntry extracts the expected hash for asset from sha256sum output
// ("<hex>  <name>", optionally "*" for binary mode and a ./ path prefix).
func sha256sumEntry(data []byte, asset string) ([]byte, bool) {
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		if filepath.Base(strings.TrimPrefix(fields[1], "*")) != asset {
			continue
		}
		want, err := hex.DecodeString(fields[0])
		if err == nil && len(want) == sha256.Size {
			return want, true
		}
	}
	return nil, false
}

// fetchURL GETs url and returns the body, failing on non-2xx responses.
func fetchURL(client *http.Client, url string) ([]byte, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxGPParserBytes))
}
