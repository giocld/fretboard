package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestUnmarshalDefaultsAutoFetchAudio(t *testing.T) {
	var c Config
	if err := json.Unmarshal([]byte(`{"theme":"dark"}`), &c); err != nil {
		t.Fatal(err)
	}
	if !c.AutoFetchAudio {
		t.Fatal("expected AutoFetchAudio to default to true when the key is absent")
	}
}

func TestUnmarshalAutoFetchAudioFalse(t *testing.T) {
	var c Config
	if err := json.Unmarshal([]byte(`{"auto_fetch_audio":false}`), &c); err != nil {
		t.Fatal(err)
	}
	if c.AutoFetchAudio {
		t.Fatal("expected AutoFetchAudio to be false")
	}
}

func TestDefaultsAutoFetchAudio(t *testing.T) {
	if !Defaults().AutoFetchAudio {
		t.Fatal("expected Defaults().AutoFetchAudio to be true")
	}
}

func TestDefaultsStrictAudioSelection(t *testing.T) {
	var c Config
	if err := json.Unmarshal([]byte(`{"theme":"dark"}`), &c); err != nil {
		t.Fatal(err)
	}
	if !c.StrictAudioSelection {
		t.Fatal("expected StrictAudioSelection to default to true when the key is absent")
	}
	var c2 Config
	if err := json.Unmarshal([]byte(`{"strict_audio_selection":false}`), &c2); err != nil {
		t.Fatal(err)
	}
	if c2.StrictAudioSelection {
		t.Fatal("expected StrictAudioSelection to be false when explicitly set")
	}
	if !Defaults().StrictAudioSelection {
		t.Fatal("expected Defaults().StrictAudioSelection to be true")
	}
}

// withTempConfigDir points the platform config dir at a temp dir, like
// internal/testutil but without the import (config can't import testutil).
func withTempConfigDir(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	switch runtime.GOOS {
	case "windows":
		t.Setenv("APPDATA", base)
	case "darwin":
		t.Setenv("HOME", base)
	default:
		t.Setenv("XDG_CONFIG_HOME", base)
	}
	return base
}

// TestLoadCorruptConfigReturnsDefaultsAndSoftError guards the "corrupt config
// locks the app out" failure: a truncated or invalid config.json must yield
// defaults plus ErrCorruptConfig so the CLI can warn and keep running.
func TestLoadCorruptConfigReturnsDefaultsAndSoftError(t *testing.T) {
	base := withTempConfigDir(t)
	dir := filepath.Join(base, "fretboard")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.json")
	cases := []string{"{ not json", "", "{\"theme\":"}
	for _, content := range cases {
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		c, err := Load()
		if err == nil || !errors.Is(err, ErrCorruptConfig) {
			t.Fatalf("content %q: want ErrCorruptConfig, got %v", content, err)
		}
		if c.UGDelayMs != 500 || c.VolumePercent != 80 || !c.AutoFetchAudio || c.ThemeName != "default" {
			t.Fatalf("content %q: expected defaults, got %+v", content, c)
		}
	}
}

// TestSaveIsAtomicAndRoundTrips verifies Save writes via temp+rename (no
// .tmp leftover) and the result Loads back unchanged.
func TestSaveIsAtomicAndRoundTrips(t *testing.T) {
	base := withTempConfigDir(t)
	c := Defaults()
	c.ThemeName = "dracula"
	c.UGDelayMs = 750
	if err := Save(c); err != nil {
		t.Fatalf("Save: %v", err)
	}
	dir := filepath.Join(base, "fretboard")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "config.json" {
			t.Fatalf("leftover file after atomic save: %s", e.Name())
		}
	}
	got, err := Load()
	if err != nil {
		t.Fatalf("Load after Save: %v", err)
	}
	if got.ThemeName != "dracula" || got.UGDelayMs != 750 {
		t.Fatalf("round trip mismatch: %+v", got)
	}
}

// TestSessionRoundTrip guards G4.1: the session persists through the config
// dir and restores with a zero session when absent.
func TestSessionRoundTrip(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	if s := LoadSession(); s.TabID != 0 {
		t.Fatalf("no session file should load a zero session, got %+v", s)
	}
	if err := SaveSession(Session{TabID: 7, Bar: 12, BPM: 96, Linear: true}); err != nil {
		t.Fatal(err)
	}
	got := LoadSession()
	if got.TabID != 7 || got.Bar != 12 || got.BPM != 96 || !got.Linear {
		t.Fatalf("session round-trip mismatch: %+v", got)
	}
}
