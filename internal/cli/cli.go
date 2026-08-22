// Package cli implements the fretboard command-line entrypoint: flag
// parsing, config loading, the non-interactive subcommands (import,
// doctor, scan, export, setup gp, and --print/--html rendering), the
// test-audio subcommand, and the interactive TUI. It is separated from
// cmd/fretboard so the whole surface is testable without a main package.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"fretboard/internal/config"
	"fretboard/internal/library"
	"fretboard/internal/parser"
	"fretboard/internal/player"
	"fretboard/internal/scraper"
	apppkg "fretboard/internal/ui/app"
	"fretboard/internal/ui/kit"
)

// Run executes the CLI and returns the process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	// --print/--html render a file to stdout. They take flags of their own
	// (--width, --theme, -o) that may follow the file path, which the flag
	// package cannot express, so the whole arg list is handled separately.
	for _, a := range args {
		if a == "--print" || a == "--html" {
			return runRender(args, stdout, stderr)
		}
	}

	fs := flag.NewFlagSet("fretboard", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { printUsage(stderr, fs) }
	ugDelay := fs.Duration("ug-delay", 0, "delay between Ultimate Guitar requests")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	cfg, err := config.Load()
	if err != nil {
		// Corrupt config must not lock the user out: warn and continue with
		// defaults; any other load error is fatal.
		fmt.Fprintf(stderr, "config: %v\n", err)
		if !errors.Is(err, config.ErrCorruptConfig) {
			return 1
		}
	}
	kit.SetTheme(cfg.ThemeName)

	// Wire player behavior from config: MIDI humanization and the audio
	// cache cap. The cap is a raw GB count; the player converts to bytes
	// internally (AudioCacheMaxGB << 30), so pass it through unchanged.
	player.HumanizeMIDI = cfg.HumanizeMIDI
	player.AudioCacheMaxGB = int64(cfg.AudioCacheMaxGB)

	if *ugDelay == 0 {
		*ugDelay = time.Duration(cfg.UGDelayMs) * time.Millisecond
	}

	rest := fs.Args()

	// Subcommands that need no library store.
	if len(rest) >= 1 && rest[0] == "doctor" {
		if len(rest) > 2 {
			fmt.Fprintln(stderr, "usage: fretboard doctor [check-name]")
			return 1
		}
		filter := ""
		if len(rest) == 2 {
			filter = rest[1]
		}
		return runDoctor(filter, stdout, stderr)
	}

	if len(rest) >= 1 && rest[0] == "setup" {
		if len(rest) < 2 || rest[1] != "gp" {
			fmt.Fprintln(stderr, "usage: fretboard setup gp [--version X]")
			return 1
		}
		version, err := parseSetupGPArgs(rest[2:])
		if err != nil {
			fmt.Fprintf(stderr, "setup gp: %v\n", err)
			return 1
		}
		return runSetupGP(version, stdout, stderr)
	}

	if len(rest) >= 1 && rest[0] == "test-audio" {
		if err := runTestAudio(cfg, stdout, stderr); err != nil {
			fmt.Fprintf(stderr, "audio test failed: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, "audio test ok — you should have heard three notes (on this machine's speakers)")
		return 0
	}

	store, err := openStore()
	if err != nil {
		fmt.Fprintf(stderr, "library: %v\n", err)
		return 1
	}
	defer store.Close()

	if len(rest) >= 1 && rest[0] == "import" {
		if len(rest) != 2 {
			fmt.Fprintln(stderr, "usage: fretboard import <file-or-directory|archive.json>")
			return 1
		}
		arg := rest[1]
		// A .json path is a library archive (ExportArchive output); anything
		// else is a tab file or a directory of tabs.
		if strings.EqualFold(filepath.Ext(arg), ".json") {
			return runImportArchive(store, arg, stdout, stderr)
		}
		if err := importPath(store, arg); err != nil {
			fmt.Fprintf(stderr, "import: %v\n", err)
			return 1
		}
		return 0
	}

	if len(rest) >= 1 && rest[0] == "scan" {
		return runScan(cfg, store, rest[1:], stdout, stderr)
	}

	if len(rest) >= 1 && rest[0] == "export" {
		return runExport(store, rest[1:], stdout, stderr)
	}

	if len(rest) > 1 {
		fmt.Fprintln(stderr, "usage: fretboard [tab-file]")
		return 1
	}
	filePath := ""
	if len(rest) == 1 {
		filePath = rest[0]
	}

	client := scraper.NewClient(*ugDelay)

	app := apppkg.NewAppWithOptions(store, client, cfg.AutoImportPath, cfg.AudioSearchPaths)
	if filePath != "" {
		tab, err := parser.ParsePath(filePath)
		if err != nil {
			fmt.Fprintf(stderr, "parse: %v\n", err)
			return 1
		}
		app.LoadViewerTab(tab, filePath)
	} else {
		// Resume the last session (tab, cursor, settings) when no file is
		// given; the startup command runs on the first Init.
		if cmd := app.RestoreSession(); cmd != nil {
			app.SetStartupCmd(cmd)
		}
	}
	app.SetVolume(cfg.VolumePercent)
	app.SetStrictAudio(cfg.StrictAudioSelection)
	if sf := resolveSoundfont(cfg); sf != "" {
		app.SetSoundfont(sf)
	}

	m := tea.Model(app)

	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())

	// NOTE: bubbletea installs its own SIGINT/SIGTERM handler and delivers a
	// QuitMsg (or InterruptMsg for SIGINT outside raw mode) to the event loop,
	// so we must not register a second signal handler here — two concurrent
	// senders on bubbletea's unbuffered message channel deadlock its shutdown.
	// Cleanup runs against the live model returned by p.Run() below.

	return runProgram(app, p)
}

// runProgram runs the bubbletea program and converts a panic into a crash
// log plus a friendly message instead of a raw stack on the terminal.
func runProgram(app apppkg.AppModel, p *tea.Program) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			app.Shutdown()
			path := writeCrashLog(r, debug.Stack())
			fmt.Fprintf(os.Stderr, "fretboard crashed: %v\ncrash details: %s\n", r, path)
			exitCode = 1
		}
	}()
	mFinal, err := p.Run()
	if appModel, ok := mFinal.(apppkg.AppModel); ok {
		appModel.Shutdown()
	}
	if err != nil && !errors.Is(err, tea.ErrInterrupted) {
		fmt.Fprintf(os.Stderr, "tui: %v\n", err)
		return 1
	}
	return 0
}

// writeCrashLog writes a panic report to the config dir so crashes are
// debuggable even when the terminal is gone. Returns the log path.
func writeCrashLog(recovered any, stack []byte) string {
	dir, err := config.Dir()
	if err != nil {
		return ""
	}
	path := filepath.Join(dir, "crash.log")
	data := fmt.Sprintf("fretboard crash at %s\npanic: %v\n\n%s\n",
		time.Now().Format(time.RFC3339), recovered, stack)
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		return ""
	}
	return path
}

func importPath(store *library.Store, path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return store.ImportDirectory(path)
	}
	_, err = store.ImportFile(path)
	return err
}

// openStore opens (or creates) the SQLite library in the user's config dir.
func openStore() (*library.Store, error) {
	dir, err := config.Dir()
	if err != nil {
		return nil, err
	}
	dbPath := filepath.Join(dir, "fretboard.db")
	return library.NewStore(dbPath)
}

// resolveSoundfont returns the soundfont path for playback: the config value,
// then the FRETBOARD_SOUNDFONT override, then the auto-discovered default.
func resolveSoundfont(cfg config.Config) string {
	sf := cfg.Soundfont
	if sf == "" {
		sf = os.Getenv("FRETBOARD_SOUNDFONT")
	}
	if sf == "" {
		sf = player.ResolveSoundfont()
	}
	return sf
}

func runTestAudio(cfg config.Config, stdout, stderr io.Writer) error {
	if !player.SynthAvailable() {
		return fmt.Errorf("fluidsynth/timidity not found — install fluidsynth (e.g. choco install fluidsynth, apt install fluidsynth)")
	}
	sf := resolveSoundfont(cfg)
	if sf == "" {
		return fmt.Errorf("no soundfont found — install a GM soundfont (e.g. soundfont-fluid) or set FRETBOARD_SOUNDFONT")
	}
	fmt.Fprintf(stdout, "soundfont: %s\n", sf)
	s := player.NewSynth()
	s.Soundfont = sf
	s.Volume = cfg.VolumePercent
	if s.Volume <= 0 {
		s.Volume = 80
	}
	tab, err := parser.Parse(strings.NewReader("Tuning: E Standard\n\ne|0-3-5|\n"))
	if err != nil {
		return err
	}
	if err := s.Play(tab, 120); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "playing via %s ...\n", s.ActiveDriver)
	time.Sleep(2 * time.Second)
	if !s.Running() {
		if s.LastError != "" {
			return fmt.Errorf("%s", s.LastError)
		}
		return fmt.Errorf("synth exited immediately")
	}
	return s.Stop()
}
