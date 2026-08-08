package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Session is the persisted last-used state, restored on the next start (when
// no tab file argument is given): which tab was open, where the cursor was,
// and the playback/layout settings.
type Session struct {
	TabID   int64  `json:"tab_id"`
	TabPath string `json:"tab_path,omitempty"`
	Bar     int    `json:"bar"` // 1-based cursor bar
	BPM     int    `json:"bpm"`
	Linear  bool   `json:"linear"`
	SavedAt string `json:"saved_at"`
}

// SessionPath returns the session file path.
func SessionPath() string {
	if dir, err := Dir(); err == nil {
		return filepath.Join(dir, "session.json")
	}
	return ""
}

// LoadSession reads the persisted session, or returns a zero session when
// none exists.
func LoadSession() Session {
	path := SessionPath()
	if path == "" {
		return Session{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Session{}
	}
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return Session{}
	}
	return s
}

// SaveSession persists the session atomically.
func SaveSession(s Session) error {
	path := SessionPath()
	if path == "" {
		return nil
	}
	s.SavedAt = time.Now().Format(time.RFC3339)
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
