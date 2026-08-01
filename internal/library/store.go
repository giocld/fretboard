package library

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Store provides persistent storage for tabs.
type Store struct {
	db *sql.DB
}

// NewStore opens (or creates) a SQLite database and runs migrations.
func NewStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return nil, fmt.Errorf("wal mode: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		return nil, fmt.Errorf("foreign keys: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS tabs (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			filepath    TEXT NOT NULL UNIQUE,
			title       TEXT NOT NULL DEFAULT '',
			artist      TEXT NOT NULL DEFAULT '',
			tuning      TEXT NOT NULL DEFAULT '',
			content     TEXT NOT NULL DEFAULT '',
			difficulty  INTEGER DEFAULT 0,
			tags        TEXT DEFAULT '[]',
			added_at    TEXT DEFAULT (datetime('now')),
			last_played TEXT,
			play_count  INTEGER DEFAULT 0,
			favorite    INTEGER DEFAULT 0
		);
	`)
	return err
}
