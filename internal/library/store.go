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

// NewStore opens (or creates) a SQLite database and runs migrations. On any
// failure the handle is closed so connections and file descriptors are not
// leaked.
func NewStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("wal mode: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("foreign keys: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
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
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS tabs (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			filepath     TEXT NOT NULL UNIQUE,
			title        TEXT NOT NULL DEFAULT '',
			artist       TEXT NOT NULL DEFAULT '',
			tuning       TEXT NOT NULL DEFAULT '',
			content      TEXT NOT NULL DEFAULT '',
			added_at     TEXT DEFAULT (datetime('now')),
			last_played  TEXT,
			play_count   INTEGER DEFAULT 0,
			favorite     INTEGER DEFAULT 0,
			source_badge TEXT NOT NULL DEFAULT ''
		);
	`); err != nil {
		return err
	}
	if err := s.addMissingColumns(); err != nil {
		return err
	}
	return s.dropLegacyColumns()
}

// addMissingColumns adds columns introduced after the original schema to
// existing databases, mirroring how the schema evolves across versions.
func (s *Store) addMissingColumns() error {
	existing, err := s.tableColumns("tabs")
	if err != nil {
		return err
	}
	if !existing["source_badge"] {
		if _, err := s.db.Exec(`ALTER TABLE tabs ADD COLUMN source_badge TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("add column source_badge: %w", err)
		}
	}
	return nil
}

func (s *Store) tableColumns(table string) (map[string]bool, error) {
	rows, err := s.db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return nil, fmt.Errorf("read table info: %w", err)
	}
	defer rows.Close()

	existing := make(map[string]bool)
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return nil, fmt.Errorf("scan table info: %w", err)
		}
		existing[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read table info: %w", err)
	}
	return existing, nil
}

// dropLegacyColumns removes columns from the tabs table that were part of
// older schemas but are no longer used. It only drops a column when it
// actually exists, so it is safe on fresh databases.
func (s *Store) dropLegacyColumns() error {
	existing, err := s.tableColumns("tabs")
	if err != nil {
		return err
	}

	for _, name := range []string{"difficulty", "tags"} {
		if !existing[name] {
			continue
		}
		if _, err := s.db.Exec(fmt.Sprintf("ALTER TABLE tabs DROP COLUMN %s", name)); err != nil {
			return fmt.Errorf("drop column %s: %w", name, err)
		}
	}
	return nil
}
