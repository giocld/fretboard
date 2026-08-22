package library

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"fretboard/internal/model"
	"fretboard/internal/parser"
)

// ImportFile reads a tab file from disk and imports it.
func (s *Store) ImportFile(filepath string) (int64, error) {
	tab, err := parser.ParsePath(filepath)
	if err != nil {
		return 0, fmt.Errorf("parse file: %w", err)
	}
	return s.Import(filepath, tab)
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

// ImportOverwrite resolves an ErrNeedsDecision the "file wins" way: the row
// is re-pointed at path and its title/artist/tuning/content are replaced with
// the file's freshly parsed values, and the edit marker is cleared. Used when
// the user chooses to discard their local edits in favor of the file.
func (s *Store) ImportOverwrite(id int64, path string) error {
	// Verify the target row exists first so a missing row reports
	// ErrNotFound rather than a misleading path-conflict error.
	if _, err := s.GetRow(id); err != nil {
		return err
	}
	tab, err := parser.ParsePath(path)
	if err != nil {
		return fmt.Errorf("overwrite %d with %s: parse file: %w", id, path, err)
	}
	if err := s.ensurePathFree(path, id); err != nil {
		return err
	}
	content, err := json.Marshal(tab)
	if err != nil {
		return fmt.Errorf("overwrite %d: marshal tab: %w", id, err)
	}
	tuningJSON, err := json.Marshal(tab.Tuning)
	if err != nil {
		return fmt.Errorf("overwrite %d: marshal tuning: %w", id, err)
	}
	hash, err := hashFile(path)
	if err != nil {
		return fmt.Errorf("overwrite %d: hash file: %w", id, err)
	}
	badge := strings.TrimSpace(tab.Metadata[model.MetaKeySourceBadge])
	res, err := s.db.Exec(`
		UPDATE tabs
		SET filepath=?, title=?, artist=?, tuning=?, content=?, source_badge=?,
		    content_sha256=?, edited_at=0
		WHERE id=?
	`, path, tab.Title, tab.Artist, string(tuningJSON), string(content), badge, hash, id)
	if err != nil {
		return fmt.Errorf("overwrite %d: update tab: %w", id, err)
	}
	return rowsAffected(res, id)
}

// KeepExisting resolves an ErrNeedsDecision the "edits win" way: the row is
// re-pointed at the file's new path but its stored content, title, artist,
// and edit marker are left untouched, so the local edits survive the file
// having moved (and possibly diverged) on disk.
func (s *Store) KeepExisting(id int64, path string) error {
	// Verify the target row exists first so a missing row reports
	// ErrNotFound rather than a misleading path-conflict error.
	if _, err := s.GetRow(id); err != nil {
		return err
	}
	if err := s.ensurePathFree(path, id); err != nil {
		return err
	}
	res, err := s.db.Exec(`UPDATE tabs SET filepath = ? WHERE id = ?`, path, id)
	if err != nil {
		return fmt.Errorf("keep existing %d: update filepath: %w", id, err)
	}
	return rowsAffected(res, id)
}
