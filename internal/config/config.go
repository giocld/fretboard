// Package config handles user preferences: config directory, theme, and
// application settings.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Config holds user-configurable preferences.
type Config struct {
	ThemeName            string   `json:"theme"`
	UGDelayMs            int      `json:"ug_delay_ms"`
	AutoImportPath       string   `json:"auto_import_path,omitempty"`
	VolumePercent        int      `json:"volume_percent"`
	Soundfont            string   `json:"soundfont,omitempty"`
	AudioSearchPaths     []string `json:"audio_search_paths,omitempty"`
	AutoFetchAudio       bool     `json:"auto_fetch_audio"`
	StrictAudioSelection bool     `json:"strict_audio_selection"` 
}

// Defaults returns the default configuration.
func Defaults() Config {
	return Config{
		ThemeName:            "default",
		UGDelayMs:            500,
		VolumePercent:        80,
		AutoFetchAudio:       true,
		StrictAudioSelection: true,
	}
}

// UnmarshalJSON defaults AutoFetchAudio and StrictAudioSelection to true when
// the keys are absent from the JSON document.
func (c *Config) UnmarshalJSON(data []byte) error {
	type alias Config
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(data, &keys); err != nil {
		return err
	}
	if _, ok := keys["auto_fetch_audio"]; !ok {
		a.AutoFetchAudio = true
	}
	if _, ok := keys["strict_audio_selection"]; !ok {
		a.StrictAudioSelection = true
	}
	*c = Config(a)
	return nil
}

// Dir returns the user's config directory for fretboard.
func Dir() (string, error) {
	cfg, err := os.UserConfigDir()
	if err != nil {
		cfg = filepath.Join(os.Getenv("HOME"), ".config")
	}
	dir := filepath.Join(cfg, "fretboard")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create config dir: %w", err)
	}
	return dir, nil
}

// ErrCorruptConfig marks a config file that could not be parsed. Callers may
// warn and continue with defaults instead of aborting; a truncated or partial
// write must not lock the user out of the app.
var ErrCorruptConfig = errors.New("config file is unreadable")

// Load reads the config file or returns defaults if it doesn't exist.
func Load() (Config, error) {
	c := Defaults()
	dir, err := Dir()
	if err != nil {
		return c, err
	}
	path := filepath.Join(dir, "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return c, nil
		}
		return c, fmt.Errorf("read config: %w", err)
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return c, fmt.Errorf("%w: %v (using defaults)", ErrCorruptConfig, err)
	}
	return c, nil
}

// Save writes the config file atomically: the data lands in a temp file that
// is renamed over the target, so a crash mid-write never leaves a corrupt
// config that Load would refuse.
func Save(c Config) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "config.json")
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("install config: %w", err)
	}
	return nil
}

// Path returns the full path to the config file.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// AudioDir returns the default directory for backing-track audio files.
func AudioDir() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	audio := filepath.Join(dir, "audio")
	if err := os.MkdirAll(audio, 0755); err != nil {
		return "", fmt.Errorf("create audio dir: %w", err)
	}
	return audio, nil
}
