package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fretboard/internal/config"
)

// TestRunCorruptConfigIsNotFatal pins the corrupt-config contract: a broken
// config file is warned about on stderr but does not abort the CLI -- the
// import subcommand still runs afterward.
func TestRunCorruptConfigIsNotFatal(t *testing.T) {
	withConfigDir(t, func(dir string) {
		if err := os.MkdirAll(filepath.Join(dir, "fretboard"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "fretboard", "config.json"), []byte("{ not json"), 0o644); err != nil {
			t.Fatal(err)
		}
		code, _, stderr := run("import", filepath.Join(dir, "does-not-exist.gp5"))
		if code != 1 {
			t.Fatalf("code=%d, want 1", code)
		}
		if !strings.Contains(stderr, "config:") || !strings.Contains(stderr, "import:") {
			t.Fatalf("stderr=%q, want both a config warning and an import error", stderr)
		}
	})
}

// TestResolveSoundfontPrecedence pins the soundfont fallback chain: the
// config value beats the FRETBOARD_SOUNDFONT override, which beats the
// auto-discovered default.
func TestResolveSoundfontPrecedence(t *testing.T) {
	t.Setenv("FRETBOARD_SOUNDFONT", "env.sf2")
	if got := resolveSoundfont(config.Config{Soundfont: "cfg.sf2"}); got != "cfg.sf2" {
		t.Fatalf("config soundfont=%q, want cfg.sf2", got)
	}
	if got := resolveSoundfont(config.Config{}); got != "env.sf2" {
		t.Fatalf("env soundfont=%q, want env.sf2", got)
	}
}
