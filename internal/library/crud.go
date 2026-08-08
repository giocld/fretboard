package library

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"fretboard/internal/model"
	"fretboard/internal/parser"
)

// ErrNotFound is returned when a tab row is missing from the library.
var ErrNotFound = errors.New("library: tab not found")

// Import parses a tab and inserts it into the library. If the filepath already
// exists, it updates the existing record and returns the same ID. The
// source_badge column mirrors tab.Metadata[model.MetaKeySourceBadge] so rows
// can show provenance without loading full content.
func (s *Store) Import(filepath string, tab *model.Tab) (int64, error) {
	if tab == nil {
		return 0, fmt.Errorf("library: import %s: nil tab", filepath)
	}
	content, err := json.Marshal(tab)
	if err != nil {
		return 0, fmt.Errorf("library: import %s: marshal tab: %w", filepath, err)
	}
	tuningJSON, err := json.Marshal(tab.Tuning)
	if err != nil {
		return 0, fmt.Errorf("library: import %s: marshal tuning: %w", filepath, err)
	}
	badge := strings.TrimSpace(tab.Metadata[model.MetaKeySourceBadge])

	var id int64
	err = s.db.QueryRow(`
		SELECT id FROM tabs WHERE filepath = ?
	`, filepath).Scan(&id)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("library: import %s: lookup filepath: %w", filepath, err)
	}

	if id > 0 {
		_, err := s.db.Exec(`
			UPDATE tabs
			SET title=?, artist=?, tuning=?, content=?, source_badge=?
			WHERE id=?
		`, tab.Title, tab.Artist, string(tuningJSON), string(content), badge, id)
		if err != nil {
			return 0, fmt.Errorf("library: import %s: update tab: %w", filepath, err)
		}
		return id, nil
	}

	res, err := s.db.Exec(`
		INSERT INTO tabs (filepath, title, artist, tuning, content, source_badge)
		VALUES (?, ?, ?, ?, ?, ?)
	`, filepath, tab.Title, tab.Artist, string(tuningJSON), string(content), badge)
	if err != nil {
		return 0, fmt.Errorf("library: import %s: insert tab: %w", filepath, err)
	}
	id, err = res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("library: import %s: last insert id: %w", filepath, err)
	}
	return id, nil
}

// ImportFile reads a tab file from disk and imports it.
func (s *Store) ImportFile(filepath string) (int64, error) {
	tab, err := parser.ParsePath(filepath)
	if err != nil {
		return 0, fmt.Errorf("parse file: %w", err)
	}
	return s.Import(filepath, tab)
}

// GetRow returns a single lightweight row.
func (s *Store) GetRow(id int64) (*TabRow, error) {
	var row TabRow
	var fav int
	var lastPlayed sql.NullString
	err := s.db.QueryRow(`
		SELECT id, filepath, title, artist, tuning, favorite, play_count, last_played, source_badge
		FROM tabs WHERE id = ?
	`, id).Scan(&row.ID, &row.Filepath, &row.Title, &row.Artist, &row.Tuning, &fav, &row.PlayCount, &lastPlayed, &row.SourceBadge)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("library: tab %d: %w", id, ErrNotFound)
		}
		return nil, fmt.Errorf("get row: %w", err)
	}
	row.Favorite = fav != 0
	if lastPlayed.Valid {
		row.LastPlayed = lastPlayed.String
	}
	return &row, nil
}

// Get reconstructs a full model.Tab from the database.
func (s *Store) Get(id int64) (*model.Tab, error) {
	var content string
	err := s.db.QueryRow(`
		SELECT content FROM tabs WHERE id = ?
	`, id).Scan(&content)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("library: tab %d: %w", id, ErrNotFound)
		}
		return nil, fmt.Errorf("get content: %w", err)
	}
	var tab model.Tab
	if err := json.Unmarshal([]byte(content), &tab); err != nil {
		return nil, fmt.Errorf("unmarshal tab: %w", err)
	}
	return &tab, nil
}

// List returns a summary of all tabs ordered by title.
func (s *Store) List() ([]TabRow, error) {
	rows, err := s.db.Query(`
		SELECT id, filepath, title, artist, tuning, favorite, play_count, last_played, source_badge
		FROM tabs ORDER BY title, id
	`)
	if err != nil {
		return nil, fmt.Errorf("list: %w", err)
	}
	defer rows.Close()

	var out []TabRow
	for rows.Next() {
		var r TabRow
		var fav int
		var lastPlayed sql.NullString
		if err := rows.Scan(&r.ID, &r.Filepath, &r.Title, &r.Artist, &r.Tuning, &fav, &r.PlayCount, &lastPlayed, &r.SourceBadge); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		r.Favorite = fav != 0
		if lastPlayed.Valid {
			r.LastPlayed = lastPlayed.String
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return out, nil
}

// Search returns tabs whose title or artist matches the query.
func (s *Store) Search(query string) ([]TabRow, error) {
	like := func(s string) string {
		s = strings.ReplaceAll(s, "\\", "\\\\")
		s = strings.ReplaceAll(s, "%", "\\%")
		s = strings.ReplaceAll(s, "_", "\\_")
		return "%" + s + "%"
	}
	q := like(query)
	rows, err := s.db.Query(`
		SELECT id, filepath, title, artist, tuning, favorite, play_count, last_played, source_badge
		FROM tabs
		WHERE title LIKE ? ESCAPE '\' OR artist LIKE ? ESCAPE '\'
		ORDER BY title, id
	`, q, q)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	defer rows.Close()

	var out []TabRow
	for rows.Next() {
		var r TabRow
		var fav int
		var lastPlayed sql.NullString
		if err := rows.Scan(&r.ID, &r.Filepath, &r.Title, &r.Artist, &r.Tuning, &fav, &r.PlayCount, &lastPlayed, &r.SourceBadge); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		r.Favorite = fav != 0
		if lastPlayed.Valid {
			r.LastPlayed = lastPlayed.String
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return out, nil
}

// SetFavorite toggles the favorite flag.
func (s *Store) SetFavorite(id int64, favorite bool) error {
	var fav int
	if favorite {
		fav = 1
	}
	res, err := s.db.Exec(`UPDATE tabs SET favorite = ? WHERE id = ?`, fav, id)
	if err != nil {
		return fmt.Errorf("set favorite: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("library: tab %d: %w", id, ErrNotFound)
	}
	return nil
}

// RecordPlay increments play_count and updates last_played for a tab.
func (s *Store) RecordPlay(id int64) error {
	res, err := s.db.Exec(`
		UPDATE tabs
		SET play_count = play_count + 1, last_played = datetime('now')
		WHERE id = ?
	`, id)
	if err != nil {
		return fmt.Errorf("record play: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("library: tab %d: %w", id, ErrNotFound)
	}
	return nil
}

// Delete removes a tab by ID.
func (s *Store) Delete(id int64) error {
	res, err := s.db.Exec(`DELETE FROM tabs WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("library: tab %d: %w", id, ErrNotFound)
	}
	return nil
}

// ImportDirectory walks a directory recursively and imports .txt tabs and
// Guitar Pro files (.gp3–.gpx). Files that fail to parse are skipped so the
// rest of the directory still gets imported; an error is only returned when
// the walk itself fails or no file could be imported.
func (s *Store) ImportDirectory(dir string) error {
	var skipped []error
	imported := 0
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(path), ".txt") && !parser.IsGpFile(path) {
			return nil
		}
		if _, err := s.ImportFile(path); err != nil {
			skipped = append(skipped, fmt.Errorf("import %s: %w", path, err))
			return nil
		}
		imported++
		return nil
	})
	if err != nil {
		return err
	}
	if imported == 0 && len(skipped) > 0 {
		return fmt.Errorf("import directory %s: no tabs imported: %v", dir, skipped)
	}
	return nil
}
