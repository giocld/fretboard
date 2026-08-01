// Package config handles user preferences: config directory, theme, and
// application settings.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config holds user-configurable preferences.
type Config struct {
	ThemeName      string `json:"theme"`
	UGDelayMs      int    `json:"ug_delay_ms"`
	AutoImportPath string `json:"auto_import_path,omitempty"`
	VolumePercent  int    `json:"volume_percent"`
	Soundfont        string   `json:"soundfont,omitempty"`
	AudioSearchPaths []string `json:"audio_search_paths,omitempty"`
	AutoFetchAudio   *bool    `json:"auto_fetch_audio,omitempty"`
}

// Defaults returns the default configuration.
func Defaults() Config {
	autoFetch := true
	return Config{
		ThemeName:      "default",
		UGDelayMs:      500,
		VolumePercent:  80,
		AutoFetchAudio: &autoFetch,
	}
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
		return c, fmt.Errorf("parse config: %w", err)
	}
	return c, nil
}

// Save writes the config file.
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
	return os.WriteFile(path, data, 0644)
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


// AutoFetchAudioEnabled reports whether online backing-track lookup is enabled.
func AutoFetchAudioEnabled(c Config) bool {
	if c.AutoFetchAudio == nil {
		return true
	}
	return *c.AutoFetchAudio
}
