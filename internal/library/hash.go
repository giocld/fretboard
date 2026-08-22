package library

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// sha256Hex returns the lowercase hex SHA-256 of data.
func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// hashFile returns the SHA-256 of a file's raw bytes.
func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return sha256Hex(data), nil
}

// contentHash identifies a tab's content. When the file at path is readable
// (local imports), the hash covers the raw file bytes so a moved file can be
// relocated later. For virtual paths (online:// URLs, unit-test imports with
// no backing file) it falls back to hashing the serialized content, which is
// still stable across re-imports of the same tab.
func contentHash(path string, content []byte) (string, error) {
	if h, err := hashFile(path); err == nil {
		return h, nil
	}
	return sha256Hex(content), nil
}

// Relocation describes a tab file that was found under a scanned directory at
// a path different from the row's stored path.
type Relocation struct {
	RowID     int64  // the tab row whose content matched
	Path      string // row's stored filepath (old location)
	FoundAt   string // where the matching file was found
	Hash      string // content hash that matched
	Ambiguous bool   // true when several files in the scan matched the same hash
}

// Relink points a row at a new filepath without touching its content. It
// fails when another row already tracks that path (filepath is UNIQUE).
func (s *Store) Relink(rowID int64, newPath string) error {
	if err := s.ensurePathFree(newPath, rowID); err != nil {
		return err
	}
	res, err := s.db.Exec(`UPDATE tabs SET filepath = ? WHERE id = ?`, newPath, rowID)
	if err != nil {
		return fmt.Errorf("relink: %w", err)
	}
	return rowsAffected(res, rowID)
}

// ensurePathFree verifies no row other than exceptID already tracks path,
// since tabs.filepath is UNIQUE.
func (s *Store) ensurePathFree(path string, exceptID int64) error {
	var holder int64
	err := s.db.QueryRow(`SELECT id FROM tabs WHERE filepath = ? AND id != ?`, path, exceptID).Scan(&holder)
	if err == nil {
		return fmt.Errorf("library: path %s already tracked by tab %d", path, holder)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("library: check path %s: %w", path, err)
	}
	return nil
}

// ScanForRelocations walks dir, hashes every file, and looks for tab rows
// whose content hash matches a file at a path other than the row's stored
// one (i.e. the file moved). Unambiguous matches are auto-relinked via
// Relink; ambiguous ones (several candidates for the same hash, or the only
// candidate already tracked by another row) are reported without relinking.
// Rows whose file is simply gone are not touched here — see MissingRows.
func (s *Store) ScanForRelocations(dir string) ([]Relocation, error) {
	filesByHash := make(map[string][]string)
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		h, err := hashFile(path)
		if err != nil {
			return nil // unreadable files are skipped, not fatal
		}
		filesByHash[h] = append(filesByHash[h], path)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan %s: %w", dir, err)
	}

	rows, err := s.List()
	if err != nil {
		return nil, err
	}

	var out []Relocation
	for _, row := range rows {
		if row.ContentHash == "" {
			continue
		}
		// File already in place: no relocation.
		if h, err := hashFile(row.Filepath); err == nil && h == row.ContentHash {
			continue
		}
		var candidates []string
		for _, p := range filesByHash[row.ContentHash] {
			if p != row.Filepath {
				candidates = append(candidates, p)
			}
		}
		if len(candidates) == 0 {
			continue
		}
		// A single candidate that another row already tracks is not
		// unambiguous: relinking would violate the UNIQUE filepath.
		ambiguous := len(candidates) > 1
		var winner string
		if !ambiguous {
			winner = candidates[0]
			var holder int64
			if err := s.db.QueryRow(`SELECT id FROM tabs WHERE filepath = ? AND id != ?`, winner, row.ID).Scan(&holder); err == nil {
				ambiguous = true
			} else if !errors.Is(err, sql.ErrNoRows) {
				return nil, fmt.Errorf("scan: check candidate %s: %w", winner, err)
			}
		}
		for _, cand := range candidates {
			out = append(out, Relocation{
				RowID:     row.ID,
				Path:      row.Filepath,
				FoundAt:   cand,
				Hash:      row.ContentHash,
				Ambiguous: ambiguous,
			})
		}
		if !ambiguous {
			if err := s.Relink(row.ID, winner); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

// MissingRows returns every local tab whose backing file no longer exists on
// disk. Rows are never deleted by this scan; they stay in the library so the
// user can relink or remove them explicitly. Virtual paths (online:// URLs)
// are skipped — they never correspond to a local file.
func (s *Store) MissingRows() ([]TabRow, error) {
	rows, err := s.List()
	if err != nil {
		return nil, err
	}
	var out []TabRow
	for _, r := range rows {
		if strings.Contains(r.Filepath, "://") {
			continue
		}
		if _, err := os.Stat(r.Filepath); err != nil {
			// Stat failures (missing, permission, broken symlink) all mean
			// the row's file is not usable where the row points.
			out = append(out, r)
		}
	}
	return out, nil
}
