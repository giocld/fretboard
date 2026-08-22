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
			id             INTEGER PRIMARY KEY AUTOINCREMENT,
			filepath       TEXT NOT NULL UNIQUE,
			title          TEXT NOT NULL DEFAULT '',
			artist         TEXT NOT NULL DEFAULT '',
			tuning         TEXT NOT NULL DEFAULT '',
			content        TEXT NOT NULL DEFAULT '',
			added_at       TEXT DEFAULT (datetime('now')),
			last_played    TEXT,
			play_count     INTEGER DEFAULT 0,
			favorite       INTEGER DEFAULT 0,
			source_badge   TEXT NOT NULL DEFAULT '',
			content_sha256 TEXT NOT NULL DEFAULT '',
			edited_at      INTEGER NOT NULL DEFAULT 0,
			status         TEXT NOT NULL DEFAULT 'want'
		);
	`); err != nil {
		return err
	}
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS tags (
			id   INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE
		);
		CREATE TABLE IF NOT EXISTS tab_tags (
			tab_id INTEGER NOT NULL REFERENCES tabs(id) ON DELETE CASCADE,
			tag_id INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
			PRIMARY KEY (tab_id, tag_id)
		);
		CREATE TABLE IF NOT EXISTS setlists (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			name       TEXT NOT NULL,
			created_at TEXT DEFAULT (datetime('now'))
		);
		CREATE TABLE IF NOT EXISTS setlist_items (
			setlist_id INTEGER NOT NULL REFERENCES setlists(id) ON DELETE CASCADE,
			tab_id     INTEGER NOT NULL REFERENCES tabs(id) ON DELETE CASCADE,
			position   INTEGER NOT NULL,
			PRIMARY KEY (setlist_id, tab_id)
		);
		CREATE TABLE IF NOT EXISTS practice_events (
			id               INTEGER PRIMARY KEY AUTOINCREMENT,
			tab_id           INTEGER NOT NULL REFERENCES tabs(id) ON DELETE CASCADE,
			started_at       INTEGER NOT NULL,
			duration_seconds INTEGER NOT NULL,
			tempo_bpm        INTEGER,
			loops            INTEGER
		);
	`); err != nil {
		return err
	}
	if err := s.addMissingColumns(); err != nil {
		return err
	}
	if err := s.addIndexes(); err != nil {
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
	if !existing["content_sha256"] {
		if _, err := s.db.Exec(`ALTER TABLE tabs ADD COLUMN content_sha256 TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("add column content_sha256: %w", err)
		}
	}
	if !existing["edited_at"] {
		if _, err := s.db.Exec(`ALTER TABLE tabs ADD COLUMN edited_at INTEGER NOT NULL DEFAULT 0`); err != nil {
			return fmt.Errorf("add column edited_at: %w", err)
		}
	}
	if !existing["status"] {
		if _, err := s.db.Exec(`ALTER TABLE tabs ADD COLUMN status TEXT NOT NULL DEFAULT 'want'`); err != nil {
			return fmt.Errorf("add column status: %w", err)
		}
	}
	return nil
}

// addIndexes creates indexes that speed up hash-based relocation scans and
// setlist ordering. All are IF NOT EXISTS so re-migration is a no-op.
func (s *Store) addIndexes() error {
	if _, err := s.db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_tabs_content_sha256 ON tabs(content_sha256);
		CREATE INDEX IF NOT EXISTS idx_setlist_items_order ON setlist_items(setlist_id, position);
	`); err != nil {
		return fmt.Errorf("create indexes: %w", err)
	}
	return nil
}

func (s *Store) tableColumns(table string) (map[string]bool, error) {
	rows, err := s.db.Query(`SELECT name FROM pragma_table_info('` + table + `')`)
	if err != nil {
		return nil, fmt.Errorf("read table info: %w", err)
	}
	defer rows.Close()

	existing := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
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
