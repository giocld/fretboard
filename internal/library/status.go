package library

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// validStatuses are the learning states a tab can be in.
var validStatuses = map[string]bool{
	"want":     true,
	"learning": true,
	"learned":  true,
}

// RowEditedAt returns the unix timestamp of the last in-app edit for a tab,
// or 0 when the tab has never been edited.
func (s *Store) RowEditedAt(id int64) (int64, error) {
	var editedAt int64
	err := s.db.QueryRow(`SELECT edited_at FROM tabs WHERE id = ?`, id).Scan(&editedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, fmt.Errorf("library: tab %d: %w", id, ErrNotFound)
		}
		return 0, fmt.Errorf("row edited_at: %w", err)
	}
	return editedAt, nil
}

// SetStatus sets a tab's learning status: want, learning, or learned.
func (s *Store) SetStatus(id int64, status string) error {
	if !validStatuses[status] {
		return fmt.Errorf("library: invalid status %q (want|learning|learned)", status)
	}
	res, err := s.db.Exec(`UPDATE tabs SET status = ? WHERE id = ?`, status, id)
	if err != nil {
		return fmt.Errorf("set status: %w", err)
	}
	return rowsAffected(res, id)
}

// AddTag attaches a tag to a tab, creating the tag when it is new. Adding an
// already-present tag is a no-op.
func (s *Store) AddTag(id int64, tag string) error {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return fmt.Errorf("library: empty tag")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("add tag: begin: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`INSERT OR IGNORE INTO tags (name) VALUES (?)`, tag); err != nil {
		return fmt.Errorf("add tag: insert tag: %w", err)
	}
	var tagID int64
	if err := tx.QueryRow(`SELECT id FROM tags WHERE name = ?`, tag).Scan(&tagID); err != nil {
		return fmt.Errorf("add tag: lookup tag: %w", err)
	}
	if _, err := tx.Exec(
		`INSERT OR IGNORE INTO tab_tags (tab_id, tag_id) VALUES (?, ?)`,
		id, tagID,
	); err != nil {
		// A FK violation here means the tab does not exist.
		if isFKViolation(err) {
			return fmt.Errorf("library: tab %d: %w", id, ErrNotFound)
		}
		return fmt.Errorf("add tag: link tag: %w", err)
	}
	return tx.Commit()
}

// RemoveTag detaches a tag from a tab. Tags left with no tabs are removed
// from the vocabulary so AllTags stays clean.
func (s *Store) RemoveTag(id int64, tag string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("remove tag: begin: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.Exec(`
		DELETE FROM tab_tags
		WHERE tab_id = ? AND tag_id = (SELECT id FROM tags WHERE name = ?)
	`, id, tag)
	if err != nil {
		return fmt.Errorf("remove tag: unlink: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("remove tag: rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("library: tab %d has no tag %q", id, tag)
	}
	if _, err := tx.Exec(`
		DELETE FROM tags WHERE id NOT IN (SELECT DISTINCT tag_id FROM tab_tags)
	`); err != nil {
		return fmt.Errorf("remove tag: prune: %w", err)
	}
	return tx.Commit()
}

// TagsFor returns a tab's tags in alphabetical order.
func (s *Store) TagsFor(id int64) ([]string, error) {
	rows, err := s.db.Query(`
		SELECT t.name FROM tags t
		JOIN tab_tags tt ON tt.tag_id = t.id
		WHERE tt.tab_id = ?
		ORDER BY t.name
	`, id)
	if err != nil {
		return nil, fmt.Errorf("tags for %d: %w", id, err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("tags for %d: scan: %w", id, err)
		}
		out = append(out, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("tags for %d: rows: %w", id, err)
	}
	return out, nil
}

// AllTags returns every tag in the library, alphabetically.
func (s *Store) AllTags() ([]string, error) {
	rows, err := s.db.Query(`SELECT name FROM tags ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("all tags: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("all tags: scan: %w", err)
		}
		out = append(out, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("all tags: rows: %w", err)
	}
	return out, nil
}

// isFKViolation reports whether err is a SQLite foreign-key constraint
// failure (raised by modernc sqlite for dangling references).
func isFKViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "FOREIGN KEY constraint failed")
}
