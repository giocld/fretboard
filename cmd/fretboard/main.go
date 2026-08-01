package main

import (
	"errors"
	"flag"
	"fmt"
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

func main() {
	ugDelay := flag.Duration("ug-delay", 0, "delay between Ultimate Guitar requests")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}
	kit.SetTheme(cfg.ThemeName)

	if *ugDelay == 0 {
		*ugDelay = time.Duration(cfg.UGDelayMs) * time.Millisecond
	}

	args := flag.Args()

	if len(args) >= 1 && args[0] == "test-audio" {
		if err := runTestAudio(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "audio test failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("audio test ok — you should have heard three notes (on this machine's speakers)")
		return
	}

	store, err := openStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "library: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	// Handle import subcommand non-interactively.
	if len(args) >= 1 && args[0] == "import" {
		if len(args) != 2 {
			fmt.Fprintln(os.Stderr, "usage: fretboard import <file-or-directory>")
			os.Exit(1)
		}
		if err := importPath(store, args[1]); err != nil {
			fmt.Fprintf(os.Stderr, "import: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if len(args) > 1 {
		fmt.Fprintln(os.Stderr, "usage: fretboard [tab-file]")
		os.Exit(1)
	}
	filePath := ""
	if len(args) == 1 {
		filePath = args[0]
	}

	client := scraper.NewClient(*ugDelay)

	var app apppkg.AppModel
	if filePath != "" {
		tab, err := parser.ParsePath(filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "parse: %v\n", err)
			os.Exit(1)
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
			fmt.Fprintf(os.Stderr, "tui: %v\n", err)
			os.Exit(1)
		}
	} else if appModel, ok := mFinal.(apppkg.AppModel); ok {
		appModel.Shutdown()
	}
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

func runTestAudio(cfg config.Config) error {
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
	fmt.Printf("soundfont: %s\n", sf)
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
	fmt.Printf("playing via %s …\n", s.ActiveDriver)
	time.Sleep(2 * time.Second)
	if !s.Running() {
		if s.LastError != "" {
			return fmt.Errorf("%s", s.LastError)
		}
		return fmt.Errorf("synth exited immediately")
	}
	return s.Stop()
}
