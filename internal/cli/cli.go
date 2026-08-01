// Package cli implements the fretboard command-line entrypoint: flag
// parsing, config loading, the non-interactive import subcommand, the
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
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/YOUR_USERNAME/fretboard/internal/config"
	"github.com/YOUR_USERNAME/fretboard/internal/library"
	"github.com/YOUR_USERNAME/fretboard/internal/parser"
	"github.com/YOUR_USERNAME/fretboard/internal/player"
	"github.com/YOUR_USERNAME/fretboard/internal/scraper"
	apppkg "github.com/YOUR_USERNAME/fretboard/internal/ui/app"
	"github.com/YOUR_USERNAME/fretboard/internal/ui/kit"
)

// Run executes the CLI and returns the process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("fretboard", flag.ContinueOnError)
	fs.SetOutput(stderr)
	ugDelay := fs.Duration("ug-delay", 0, "delay between Ultimate Guitar requests")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(stderr, "config: %v\n", err)
		return 1
	}
	kit.SetTheme(cfg.ThemeName)

	if *ugDelay == 0 {
		*ugDelay = time.Duration(cfg.UGDelayMs) * time.Millisecond
	}

	rest := fs.Args()

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

	// Handle the import subcommand non-interactively.
	if len(rest) >= 1 && rest[0] == "import" {
		if len(rest) != 2 {
			fmt.Fprintln(stderr, "usage: fretboard import <file-or-directory>")
			return 1
		}
		if err := importPath(store, rest[1]); err != nil {
			fmt.Fprintf(stderr, "import: %v\n", err)
			return 1
		}
		return 0
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

	var app apppkg.AppModel
	if filePath != "" {
		tab, err := parser.ParsePath(filePath)
		if err != nil {
			fmt.Fprintf(stderr, "parse: %v\n", err)
			return 1
		}
		app = apppkg.NewAppWithOptions(store, client, cfg.AutoImportPath, cfg.AudioSearchPaths)
		app.LoadViewerTab(tab, filePath)
	} else {
		app = apppkg.NewAppWithOptions(store, client, cfg.AutoImportPath, cfg.AudioSearchPaths)
	}
	app.SetVolume(cfg.VolumePercent)
	sf := cfg.Soundfont
	if sf == "" {
		sf = os.Getenv("FRETBOARD_SOUNDFONT")
	}
	if sf == "" {
		sf = player.ResolveSoundfont()
	}
	if sf != "" {
		app.SetSoundfont(sf)
	}

	m := tea.Model(app)

	p := tea.NewProgram(m, tea.WithAltScreen())

	// NOTE: bubbletea installs its own SIGINT/SIGTERM handler and delivers a
	// QuitMsg (or InterruptMsg for SIGINT outside raw mode) to the event loop,
	// so we must not register a second signal handler here — two concurrent
	// senders on bubbletea's unbuffered message channel deadlock its shutdown.
	// Cleanup runs against the live model returned by p.Run() below.

	if mFinal, err := p.Run(); err != nil {
		if appModel, ok := mFinal.(apppkg.AppModel); ok {
			appModel.Shutdown()
		}
		if !errors.Is(err, tea.ErrInterrupted) {
			fmt.Fprintf(stderr, "tui: %v\n", err)
			return 1
		}
	} else if appModel, ok := mFinal.(apppkg.AppModel); ok {
		appModel.Shutdown()
	}
	return 0
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

func runTestAudio(cfg config.Config, stdout, stderr io.Writer) error {
	if !player.SynthAvailable() {
		return fmt.Errorf("fluidsynth/timidity not found — install: sudo pacman -S fluidsynth soundfont-fluid")
	}
	sf := cfg.Soundfont
	if sf == "" {
		sf = os.Getenv("FRETBOARD_SOUNDFONT")
	}
	if sf == "" {
		sf = player.ResolveSoundfont()
	}
	if sf == "" {
		return fmt.Errorf("no soundfont found — install: sudo pacman -S soundfont-fluid")
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
	fmt.Fprintf(stdout, "playing via %s …\n", s.ActiveDriver)
	time.Sleep(2 * time.Second)
	if !s.Running() {
		if s.LastError != "" {
			return fmt.Errorf("%s", s.LastError)
		}
		return fmt.Errorf("synth exited immediately")
	}
	return s.Stop()
}
