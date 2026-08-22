package library

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"fretboard/internal/model"
)

// ErrNotFound is returned when a tab row is missing from the library.
var ErrNotFound = errors.New("library: tab not found")

// ErrNeedsDecision is returned by Import when the file being re-imported
// conflicts with local edits: the row has edited_at > 0 (in-app edits) or its
// stored title/artist differ from the freshly parsed values. Callers should
// prompt with ImportOverwrite / KeepExisting instead of silently clobbering.
// The affected row is carried by a *NeedsDecisionError wrapper.
var ErrNeedsDecision = errors.New("library: re-import needs decision: tab has local edits")

// NeedsDecisionError wraps ErrNeedsDecision and carries the affected row so
// callers can present an overwrite-vs-keep prompt without a second query.
type NeedsDecisionError struct {
	Row *TabRow
}

func (e *NeedsDecisionError) Error() string {
	return ErrNeedsDecision.Error()
}

func (e *NeedsDecisionError) Unwrap() error {
	return ErrNeedsDecision
}

// Import parses a tab and inserts it into the library. If the filepath already
// exists, it updates the existing record and returns the same ID. The
// source_badge column mirrors tab.Metadata[model.MetaKeySourceBadge] so rows
// can show provenance without loading full content.
//
// Re-imports are edit-aware: an untouched row (edited_at == 0 and matching
// title/artist) is updated silently as before, but a row that carries local
// edits returns ErrNeedsDecision (wrapped in *NeedsDecisionError with the
// row attached) so the caller can choose between ImportOverwrite and
// KeepExisting.
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
	hash, err := contentHash(filepath, content)
	if err != nil {
		return 0, fmt.Errorf("library: import %s: hash: %w", filepath, err)
	}

	var id int64
	err = s.db.QueryRow(`
		SELECT id FROM tabs WHERE filepath = ?
	`, filepath).Scan(&id)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("library: import %s: lookup filepath: %w", filepath, err)
	}

	if id > 0 {
		editedAt, err := s.RowEditedAt(id)
		if err != nil {
			return 0, fmt.Errorf("library: import %s: read edited_at: %w", filepath, err)
		}
		if editedAt > 0 {
			row, err := s.GetRow(id)
			if err != nil {
				return 0, fmt.Errorf("library: import %s: read row: %w", filepath, err)
			}
			return 0, &NeedsDecisionError{Row: row}
		}
		var storedTitle, storedArtist string
		if err := s.db.QueryRow(`SELECT title, artist FROM tabs WHERE id = ?`, id).Scan(&storedTitle, &storedArtist); err != nil {
			return 0, fmt.Errorf("library: import %s: read stored meta: %w", filepath, err)
		}
		if storedTitle != tab.Title || storedArtist != tab.Artist {
			row, err := s.GetRow(id)
			if err != nil {
				return 0, fmt.Errorf("library: import %s: read row: %w", filepath, err)
			}
			return 0, &NeedsDecisionError{Row: row}
		}
		_, err = s.db.Exec(`
			UPDATE tabs
			SET title=?, artist=?, tuning=?, content=?, source_badge=?, content_sha256=?
			WHERE id=?
		`, tab.Title, tab.Artist, string(tuningJSON), string(content), badge, hash, id)
		if err != nil {
			return 0, fmt.Errorf("library: import %s: update tab: %w", filepath, err)
		}
		return id, nil
	}

	res, err := s.db.Exec(`
		INSERT INTO tabs (filepath, title, artist, tuning, content, source_badge, content_sha256)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, filepath, tab.Title, tab.Artist, string(tuningJSON), string(content), badge, hash)
	if err != nil {
		return 0, fmt.Errorf("library: import %s: insert tab: %w", filepath, err)
	}
	id, err = res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("library: import %s: last insert id: %w", filepath, err)
	}
	return id, nil
}

// GetRow returns a single lightweight row.
func (s *Store) GetRow(id int64) (*TabRow, error) {
	var row TabRow
	var fav int
	var lastPlayed sql.NullString
	err := s.db.QueryRow(`
		SELECT id, filepath, title, artist, tuning, favorite, play_count, last_played, source_badge,
		       content_sha256, edited_at, status
		FROM tabs WHERE id = ?
	`, id).Scan(&row.ID, &row.Filepath, &row.Title, &row.Artist, &row.Tuning, &fav, &row.PlayCount, &lastPlayed, &row.SourceBadge, &row.ContentHash, &row.EditedAt, &row.Status)
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
		SELECT id, filepath, title, artist, tuning, favorite, play_count, last_played, source_badge,
		       content_sha256, edited_at, status
		FROM tabs ORDER BY title, id
	`)
	if err != nil {
		return nil, fmt.Errorf("list: %w", err)
	}
	return scanTabRows(rows)
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
		SELECT id, filepath, title, artist, tuning, favorite, play_count, last_played, source_badge,
		       content_sha256, edited_at, status
		FROM tabs
		WHERE title LIKE ? ESCAPE '\' OR artist LIKE ? ESCAPE '\'
		ORDER BY title, id
	`, q, q)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	return scanTabRows(rows)
}

// scanTabRows converts every row of a query result into TabRow summaries,
// mapping the SQLite integer favorite and nullable last_played to the Go
// types the UI consumes.
func scanTabRows(rows *sql.Rows) ([]TabRow, error) {
	defer rows.Close()
	var out []TabRow
	for rows.Next() {
		var r TabRow
		var fav int
		var lastPlayed sql.NullString
		if err := rows.Scan(&r.ID, &r.Filepath, &r.Title, &r.Artist, &r.Tuning, &fav, &r.PlayCount, &lastPlayed, &r.SourceBadge, &r.ContentHash, &r.EditedAt, &r.Status); err != nil {
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

// rowsAffected turns a zero-row UPDATE/DELETE into ErrNotFound and unwraps
// RowsAffected errors, so every mutation reports a missing tab the same way.
func rowsAffected(res sql.Result, id int64) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("library: tab %d: %w", id, ErrNotFound)
	}
	return nil
}

// UpdateMeta edits the display title/artist of a tab, rewriting both the
// row columns and the stored content so the viewer sees the change. The row
// is marked edited (edited_at = now), so a later re-import of the same file
// returns ErrNeedsDecision instead of silently overwriting the edit; callers
// resolve that with ImportOverwrite (file wins) or KeepExisting (edits win).
func (s *Store) UpdateMeta(id int64, title, artist string) error {
	tab, err := s.Get(id)
	if err != nil {
		return err
	}
	tab.Title = title
	tab.Artist = artist
	content, err := json.Marshal(tab)
	if err != nil {
		return fmt.Errorf("update meta: marshal tab: %w", err)
	}
	res, err := s.db.Exec(`
		UPDATE tabs SET title = ?, artist = ?, content = ?, edited_at = ? WHERE id = ?
	`, title, artist, string(content), time.Now().Unix(), id)
	if err != nil {
		return fmt.Errorf("update meta: %w", err)
	}
	return rowsAffected(res, id)
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
	return rowsAffected(res, id)
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
	return rowsAffected(res, id)
}

// Delete removes a tab by ID.
func (s *Store) Delete(id int64) error {
	res, err := s.db.Exec(`DELETE FROM tabs WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete: %w", err)
	}
	return rowsAffected(res, id)
}
