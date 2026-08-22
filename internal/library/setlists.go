package library

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// ErrSetlistNotFound is returned when a setlist is missing from the library.
var ErrSetlistNotFound = errors.New("library: setlist not found")

// Setlist is a lightweight summary of a named tab collection.
type Setlist struct {
	ID        int64
	Name      string
	CreatedAt string // SQLite datetime, e.g. "2026-08-23 12:00:00"
	TabCount  int
}

// CreateSetlist creates a setlist and returns its ID.
func (s *Store) CreateSetlist(name string) (int64, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, fmt.Errorf("library: empty setlist name")
	}
	res, err := s.db.Exec(`INSERT INTO setlists (name) VALUES (?)`, name)
	if err != nil {
		return 0, fmt.Errorf("create setlist: %w", err)
	}
	return res.LastInsertId()
}

// setlistExists reports whether a setlist with the given ID exists.
func (s *Store) setlistExists(id int64) bool {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM setlists WHERE id = ?`, id).Scan(&n); err != nil {
		return false
	}
	return n > 0
}

// AddToSetlist appends a tab to a setlist. Adding a tab that is already a
// member is a no-op (the pair is the primary key).
func (s *Store) AddToSetlist(setlistID, tabID int64) error {
	if !s.setlistExists(setlistID) {
		return fmt.Errorf("library: setlist %d: %w", setlistID, ErrSetlistNotFound)
	}
	// FK on tab_id catches unknown tabs; surface ErrNotFound for those.
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM tabs WHERE id = ?`, tabID).Scan(&n); err != nil {
		return fmt.Errorf("add to setlist: check tab: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("library: tab %d: %w", tabID, ErrNotFound)
	}
	var pos int64
	if err := s.db.QueryRow(`
		SELECT COALESCE(MAX(position), -1) + 1 FROM setlist_items WHERE setlist_id = ?
	`, setlistID).Scan(&pos); err != nil {
		return fmt.Errorf("add to setlist: next position: %w", err)
	}
	if _, err := s.db.Exec(`
		INSERT OR IGNORE INTO setlist_items (setlist_id, tab_id, position) VALUES (?, ?, ?)
	`, setlistID, tabID, pos); err != nil {
		return fmt.Errorf("add to setlist: %w", err)
	}
	return nil
}

// RemoveFromSetlist removes a tab from a setlist and renumbers the remaining
// items so positions stay contiguous. Removing a non-member is a no-op.
func (s *Store) RemoveFromSetlist(setlistID, tabID int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("remove from setlist: begin: %w", err)
	}
	defer tx.Rollback()

	var pos sql.NullInt64
	if err := tx.QueryRow(`
		SELECT position FROM setlist_items WHERE setlist_id = ? AND tab_id = ?
	`, setlistID, tabID).Scan(&pos); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("remove from setlist: lookup: %w", err)
	}
	if err == sql.ErrNoRows || !pos.Valid {
		return tx.Commit() // not a member: nothing to do
	}
	if _, err := tx.Exec(`
		DELETE FROM setlist_items WHERE setlist_id = ? AND tab_id = ?
	`, setlistID, tabID); err != nil {
		return fmt.Errorf("remove from setlist: delete: %w", err)
	}
	if _, err := tx.Exec(`
		UPDATE setlist_items SET position = position - 1
		WHERE setlist_id = ? AND position > ?
	`, setlistID, pos.Int64); err != nil {
		return fmt.Errorf("remove from setlist: renumber: %w", err)
	}
	return tx.Commit()
}

// ReorderSetlist sets the order of setlist items. Listed tabs get positions
// 0..n-1 in the given order; any member not listed keeps its relative order
// appended after them, so no tab is ever silently dropped.
func (s *Store) ReorderSetlist(setlistID int64, tabIDs []int64) error {
	if !s.setlistExists(setlistID) {
		return fmt.Errorf("library: setlist %d: %w", setlistID, ErrSetlistNotFound)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("reorder setlist: begin: %w", err)
	}
	defer tx.Rollback()

	rows, err := tx.Query(`
		SELECT tab_id FROM setlist_items WHERE setlist_id = ? ORDER BY position, tab_id
	`, setlistID)
	if err != nil {
		return fmt.Errorf("reorder setlist: read members: %w", err)
	}
	var current []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return fmt.Errorf("reorder setlist: scan member: %w", err)
		}
		current = append(current, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("reorder setlist: rows: %w", err)
	}

	listed := make(map[int64]bool, len(tabIDs))
	final := make([]int64, 0, len(current))
	for _, id := range tabIDs {
		if listed[id] {
			continue
		}
		listed[id] = true
		final = append(final, id)
	}
	for _, id := range current {
		if !listed[id] {
			final = append(final, id)
		}
	}

	for i, id := range final {
		if _, err := tx.Exec(`
			UPDATE setlist_items SET position = ? WHERE setlist_id = ? AND tab_id = ?
		`, i, setlistID, id); err != nil {
			return fmt.Errorf("reorder setlist: update: %w", err)
		}
	}
	return tx.Commit()
}

// Setlists returns all setlists ordered by name.
func (s *Store) Setlists() ([]Setlist, error) {
	rows, err := s.db.Query(`
		SELECT sl.id, sl.name, sl.created_at, COUNT(si.tab_id)
		FROM setlists sl
		LEFT JOIN setlist_items si ON si.setlist_id = sl.id
		GROUP BY sl.id
		ORDER BY sl.name, sl.id
	`)
	if err != nil {
		return nil, fmt.Errorf("setlists: %w", err)
	}
	defer rows.Close()
	var out []Setlist
	for rows.Next() {
		var sl Setlist
		if err := rows.Scan(&sl.ID, &sl.Name, &sl.CreatedAt, &sl.TabCount); err != nil {
			return nil, fmt.Errorf("setlists: scan: %w", err)
		}
		out = append(out, sl)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("setlists: rows: %w", err)
	}
	return out, nil
}

// SetlistTabs returns the tabs in a setlist in their configured order.
func (s *Store) SetlistTabs(setlistID int64) ([]TabRow, error) {
	if !s.setlistExists(setlistID) {
		return nil, fmt.Errorf("library: setlist %d: %w", setlistID, ErrSetlistNotFound)
	}
	rows, err := s.db.Query(`
		SELECT t.id, t.filepath, t.title, t.artist, t.tuning, t.favorite, t.play_count,
		       t.last_played, t.source_badge, t.content_sha256, t.edited_at, t.status
		FROM setlist_items si
		JOIN tabs t ON t.id = si.tab_id
		WHERE si.setlist_id = ?
		ORDER BY si.position, t.id
	`, setlistID)
	if err != nil {
		return nil, fmt.Errorf("setlist tabs %d: %w", setlistID, err)
	}
	return scanTabRows(rows)
}
