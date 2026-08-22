package library

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"fretboard/internal/config"
)

// archiveVersion is the format version written by ExportArchive and checked
// by ImportArchive. Bump when the JSON shape changes incompatibly.
const archiveVersion = 1

// tabsDir returns the configured tab root directory, or "" when unset.
// It is a package-level var so tests (and headless tools) can override the
// user config without writing a config file; Wave 2 wires config normally.
var tabsDir = func() string {
	c, err := config.Load()
	if err != nil {
		return ""
	}
	return c.TabsDir
}

// archiveFile is one manifest entry: where the tab file lives relative to the
// export root (or absolute when the root was unset) and its content hash.
type archiveFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type archiveTab struct {
	ID          int64  `json:"id"`
	Filepath    string `json:"filepath"`
	Title       string `json:"title"`
	Artist      string `json:"artist"`
	Tuning      string `json:"tuning"`
	Content     string `json:"content"`
	AddedAt     string `json:"added_at"`
	LastPlayed  string `json:"last_played"`
	PlayCount   int64  `json:"play_count"`
	Favorite    bool   `json:"favorite"`
	SourceBadge string `json:"source_badge"`
	ContentHash string `json:"content_sha256"`
	EditedAt    int64  `json:"edited_at"`
	Status      string `json:"status"`
}

type archiveTabTag struct {
	TabID int64  `json:"tab_id"`
	Tag   string `json:"tag"`
}

type archiveSetlist struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

type archiveSetlistItem struct {
	SetlistID int64 `json:"setlist_id"`
	TabID     int64 `json:"tab_id"`
	Position  int64 `json:"position"`
}

type archivePracticeEvent struct {
	TabID           int64 `json:"tab_id"`
	StartedAt       int64 `json:"started_at"`
	DurationSeconds int64 `json:"duration_seconds"`
	TempoBPM        int64 `json:"tempo_bpm"`
	Loops           int64 `json:"loops"`
}

type archive struct {
	Version        int                    `json:"version"`
	TabsDir        string                 `json:"tabs_dir,omitempty"`
	Tabs           []archiveTab           `json:"tabs"`
	Tags           []string               `json:"tags"`
	TabTags        []archiveTabTag        `json:"tab_tags"`
	Setlists       []archiveSetlist       `json:"setlists"`
	SetlistItems   []archiveSetlistItem   `json:"setlist_items"`
	PracticeEvents []archivePracticeEvent `json:"practice_events"`
	Files          []archiveFile          `json:"files"`
}

// manifestPath renders a tab's filepath for the manifest: relative to the
// export root (slash-separated) when the root is set and the file sits under
// it, otherwise the absolute path verbatim.
func manifestPath(root, path string) string {
	if root == "" {
		return path
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return path // outside the root: keep absolute
	}
	return filepath.ToSlash(rel)
}

// ExportArchive writes a single portable JSON archive: a dump of the
// schema-relevant rows (tabs, tags, setlists, practice events) plus a file
// manifest with relative paths and sha256 hashes. Relative paths are computed
// against the configured TabsDir root; when unset, absolute paths are stored.
// The output is deterministic for identical library state.
func (s *Store) ExportArchive(path string) error {
	root := tabsDir()
	a := archive{Version: archiveVersion, TabsDir: root}

	// Tabs.
	rows, err := s.db.Query(`
		SELECT id, filepath, title, artist, tuning, content, added_at, last_played,
		       play_count, favorite, source_badge, content_sha256, edited_at, status
		FROM tabs
	`)
	if err != nil {
		return fmt.Errorf("export: read tabs: %w", err)
	}
	for rows.Next() {
		var t archiveTab
		var fav int
		var lastPlayed sql.NullString
		if err := rows.Scan(&t.ID, &t.Filepath, &t.Title, &t.Artist, &t.Tuning, &t.Content, &t.AddedAt,
			&lastPlayed, &t.PlayCount, &fav, &t.SourceBadge, &t.ContentHash, &t.EditedAt, &t.Status); err != nil {
			rows.Close()
			return fmt.Errorf("export: scan tab: %w", err)
		}
		t.Favorite = fav != 0
		if lastPlayed.Valid {
			t.LastPlayed = lastPlayed.String
		}
		a.Tabs = append(a.Tabs, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("export: tabs rows: %w", err)
	}

	// Tags + links.
	tagRows, err := s.db.Query(`SELECT id, name FROM tags`)
	if err != nil {
		return fmt.Errorf("export: read tags: %w", err)
	}
	tagIDToName := map[int64]string{}
	for tagRows.Next() {
		var id int64
		var name string
		if err := tagRows.Scan(&id, &name); err != nil {
			tagRows.Close()
			return fmt.Errorf("export: scan tag: %w", err)
		}
		tagIDToName[id] = name
		a.Tags = append(a.Tags, name)
	}
	tagRows.Close()
	if err := tagRows.Err(); err != nil {
		return fmt.Errorf("export: tags rows: %w", err)
	}
	sort.Strings(a.Tags)

	linkRows, err := s.db.Query(`SELECT tab_id, tag_id FROM tab_tags`)
	if err != nil {
		return fmt.Errorf("export: read tab_tags: %w", err)
	}
	for linkRows.Next() {
		var tt archiveTabTag
		var tagID int64
		if err := linkRows.Scan(&tt.TabID, &tagID); err != nil {
			linkRows.Close()
			return fmt.Errorf("export: scan tab_tag: %w", err)
		}
		tt.Tag = tagIDToName[tagID]
		if tt.Tag == "" {
			continue // orphaned link: skip rather than emit garbage
		}
		a.TabTags = append(a.TabTags, tt)
	}
	linkRows.Close()
	if err := linkRows.Err(); err != nil {
		return fmt.Errorf("export: tab_tags rows: %w", err)
	}

	// Setlists + items.
	slRows, err := s.db.Query(`SELECT id, name, created_at FROM setlists`)
	if err != nil {
		return fmt.Errorf("export: read setlists: %w", err)
	}
	for slRows.Next() {
		var sl archiveSetlist
		if err := slRows.Scan(&sl.ID, &sl.Name, &sl.CreatedAt); err != nil {
			slRows.Close()
			return fmt.Errorf("export: scan setlist: %w", err)
		}
		a.Setlists = append(a.Setlists, sl)
	}
	slRows.Close()
	if err := slRows.Err(); err != nil {
		return fmt.Errorf("export: setlists rows: %w", err)
	}

	itemRows, err := s.db.Query(`SELECT setlist_id, tab_id, position FROM setlist_items`)
	if err != nil {
		return fmt.Errorf("export: read setlist_items: %w", err)
	}
	for itemRows.Next() {
		var it archiveSetlistItem
		if err := itemRows.Scan(&it.SetlistID, &it.TabID, &it.Position); err != nil {
			itemRows.Close()
			return fmt.Errorf("export: scan setlist item: %w", err)
		}
		a.SetlistItems = append(a.SetlistItems, it)
	}
	itemRows.Close()
	if err := itemRows.Err(); err != nil {
		return fmt.Errorf("export: setlist_items rows: %w", err)
	}

	// Practice events.
	peRows, err := s.db.Query(`SELECT tab_id, started_at, duration_seconds, tempo_bpm, loops FROM practice_events`)
	if err != nil {
		return fmt.Errorf("export: read practice_events: %w", err)
	}
	for peRows.Next() {
		var pe archivePracticeEvent
		var tempo, loops sql.NullInt64
		if err := peRows.Scan(&pe.TabID, &pe.StartedAt, &pe.DurationSeconds, &tempo, &loops); err != nil {
			peRows.Close()
			return fmt.Errorf("export: scan practice event: %w", err)
		}
		if tempo.Valid {
			pe.TempoBPM = tempo.Int64
		}
		if loops.Valid {
			pe.Loops = loops.Int64
		}
		a.PracticeEvents = append(a.PracticeEvents, pe)
	}
	peRows.Close()
	if err := peRows.Err(); err != nil {
		return fmt.Errorf("export: practice_events rows: %w", err)
	}

	// File manifest: one entry per tab file, hash from disk when present
	// (what matters for portability), else the stored content hash.
	for _, t := range a.Tabs {
		mp := manifestPath(root, t.Filepath)
		h := t.ContentHash
		if fh, err := hashFile(t.Filepath); err == nil {
			h = fh
		}
		a.Files = append(a.Files, archiveFile{Path: mp, SHA256: h})
	}
	sort.Slice(a.Tabs, func(i, j int) bool { return a.Tabs[i].ID < a.Tabs[j].ID })
	sort.Slice(a.Files, func(i, j int) bool { return a.Files[i].Path < a.Files[j].Path })
	sort.Slice(a.TabTags, func(i, j int) bool {
		if a.TabTags[i].TabID != a.TabTags[j].TabID {
			return a.TabTags[i].TabID < a.TabTags[j].TabID
		}
		return a.TabTags[i].Tag < a.TabTags[j].Tag
	})
	sort.Slice(a.Setlists, func(i, j int) bool { return a.Setlists[i].ID < a.Setlists[j].ID })
	sort.Slice(a.SetlistItems, func(i, j int) bool {
		if a.SetlistItems[i].SetlistID != a.SetlistItems[j].SetlistID {
			return a.SetlistItems[i].SetlistID < a.SetlistItems[j].SetlistID
		}
		return a.SetlistItems[i].Position < a.SetlistItems[j].Position
	})
	sort.Slice(a.PracticeEvents, func(i, j int) bool { return a.PracticeEvents[i].StartedAt < a.PracticeEvents[j].StartedAt })

	data, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return fmt.Errorf("export: marshal archive: %w", err)
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("export: mkdir %s: %w", dir, err)
		}
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("export: write %s: %w", path, err)
	}
	return nil
}

// ImportArchive restores tabs, tags, setlists, and practice events from an
// archive written by ExportArchive, then resolves each manifest file under
// the local tabs dir (config.TabsDir, overridable via the tabsDir var). Rows
// are always restored; resolved files have their row's filepath relinked to
// the local copy. The returned slice lists the manifest paths that could not
// be resolved locally (missing file, or content mismatch) so the caller can
// report which files still need copying.
func (s *Store) ImportArchive(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("import archive: read %s: %w", path, err)
	}
	var a archive
	if err := json.Unmarshal(data, &a); err != nil {
		return nil, fmt.Errorf("import archive: parse %s: %w", path, err)
	}
	if a.Version != archiveVersion {
		return nil, fmt.Errorf("import archive: %s: unsupported version %d (want %d)", path, a.Version, archiveVersion)
	}
	root := tabsDir()

	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("import archive: begin: %w", err)
	}
	defer tx.Rollback()

	// Tags: create missing names, map name -> id.
	tagIDs := make(map[string]int64, len(a.Tags))
	for _, name := range a.Tags {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO tags (name) VALUES (?)`, name); err != nil {
			return nil, fmt.Errorf("import archive: insert tag %s: %w", name, err)
		}
		var tid int64
		if err := tx.QueryRow(`SELECT id FROM tags WHERE name = ?`, name).Scan(&tid); err != nil {
			return nil, fmt.Errorf("import archive: lookup tag %s: %w", name, err)
		}
		tagIDs[name] = tid
	}

	// Tabs: merge by filepath (same file -> same row), preserving archive
	// IDs when free, remapping otherwise.
	tabIDs := make(map[int64]int64, len(a.Tabs))
	for _, t := range a.Tabs {
		var fav int
		if t.Favorite {
			fav = 1
		}
		var actual int64
		err := tx.QueryRow(`SELECT id FROM tabs WHERE filepath = ?`, t.Filepath).Scan(&actual)
		if err == nil {
			if _, err := tx.Exec(`
				UPDATE tabs SET title=?, artist=?, tuning=?, content=?, added_at=?, last_played=?,
				       play_count=?, favorite=?, source_badge=?, content_sha256=?, edited_at=?, status=?
				WHERE id=?
			`, t.Title, t.Artist, t.Tuning, t.Content, t.AddedAt, t.LastPlayed, t.PlayCount, fav,
				t.SourceBadge, t.ContentHash, t.EditedAt, t.Status, actual); err != nil {
				return nil, fmt.Errorf("import archive: update tab %s: %w", t.Filepath, err)
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("import archive: lookup tab %s: %w", t.Filepath, err)
		} else {
			res, insErr := tx.Exec(`
				INSERT INTO tabs (id, filepath, title, artist, tuning, content, added_at, last_played,
				                  play_count, favorite, source_badge, content_sha256, edited_at, status)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			`, t.ID, t.Filepath, t.Title, t.Artist, t.Tuning, t.Content, t.AddedAt, t.LastPlayed,
				t.PlayCount, fav, t.SourceBadge, t.ContentHash, t.EditedAt, t.Status)
			if insErr != nil {
				// Archive ID already taken in the target: fall back to autoincrement.
				res, insErr = tx.Exec(`
					INSERT INTO tabs (filepath, title, artist, tuning, content, added_at, last_played,
					                  play_count, favorite, source_badge, content_sha256, edited_at, status)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				`, t.Filepath, t.Title, t.Artist, t.Tuning, t.Content, t.AddedAt, t.LastPlayed,
					t.PlayCount, fav, t.SourceBadge, t.ContentHash, t.EditedAt, t.Status)
				if insErr != nil {
					return nil, fmt.Errorf("import archive: insert tab %s: %w", t.Filepath, insErr)
				}
			}
			actual, err = res.LastInsertId()
			if err != nil {
				return nil, fmt.Errorf("import archive: last insert id: %w", err)
			}
		}
		tabIDs[t.ID] = actual
	}

	// Tab-tag links.
	for _, tt := range a.TabTags {
		tagID, ok := tagIDs[tt.Tag]
		if !ok {
			continue // tag missing from archive Tags: skip the dangling link
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO tab_tags (tab_id, tag_id) VALUES (?, ?)`, tabIDs[tt.TabID], tagID); err != nil {
			return nil, fmt.Errorf("import archive: link tag: %w", err)
		}
	}

	// Setlists: preserve archive IDs when free, remap otherwise. Names are
	// not unique, so duplicates merge by ID only.
	setlistIDs := make(map[int64]int64, len(a.Setlists))
	for _, sl := range a.Setlists {
		var actual int64
		res, err := tx.Exec(`INSERT INTO setlists (id, name, created_at) VALUES (?, ?, ?)`, sl.ID, sl.Name, sl.CreatedAt)
		if err != nil {
			res, err = tx.Exec(`INSERT INTO setlists (name, created_at) VALUES (?, ?)`, sl.Name, sl.CreatedAt)
			if err != nil {
				return nil, fmt.Errorf("import archive: insert setlist %s: %w", sl.Name, err)
			}
		}
		actual, err = res.LastInsertId()
		if err != nil {
			return nil, fmt.Errorf("import archive: setlist id: %w", err)
		}
		setlistIDs[sl.ID] = actual
	}
	for _, it := range a.SetlistItems {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO setlist_items (setlist_id, tab_id, position) VALUES (?, ?, ?)`,
			setlistIDs[it.SetlistID], tabIDs[it.TabID], it.Position); err != nil {
			return nil, fmt.Errorf("import archive: insert setlist item: %w", err)
		}
	}

	// Practice events.
	for _, pe := range a.PracticeEvents {
		if _, err := tx.Exec(`
			INSERT INTO practice_events (tab_id, started_at, duration_seconds, tempo_bpm, loops)
			VALUES (?, ?, ?, ?, ?)
		`, tabIDs[pe.TabID], pe.StartedAt, pe.DurationSeconds, pe.TempoBPM, pe.Loops); err != nil {
			return nil, fmt.Errorf("import archive: insert practice event: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("import archive: commit: %w", err)
	}

	// Resolve manifest files against the local tabs dir, relinking rows whose
	// file is present and content-identical.
	var unresolved []string
	for _, f := range a.Files {
		var local string
		if filepath.IsAbs(f.Path) {
			local = f.Path
		} else if root != "" {
			local = filepath.Join(root, filepath.FromSlash(f.Path))
		}
		if local == "" {
			unresolved = append(unresolved, f.Path)
			continue
		}
		ok, err := fileMatches(local, f.SHA256)
		if err != nil || !ok {
			unresolved = append(unresolved, f.Path)
			continue
		}
		// Relink the imported row to the local copy.
		dumped := f.Path
		if !filepath.IsAbs(f.Path) && a.TabsDir != "" {
			dumped = filepath.Join(a.TabsDir, filepath.FromSlash(f.Path))
		}
		var target int64
		err = s.db.QueryRow(`SELECT id FROM tabs WHERE filepath = ?`, dumped).Scan(&target)
		if err != nil {
			continue // row not found (was merged away): nothing to relink
		}
		var holder int64
		if err := s.db.QueryRow(`SELECT id FROM tabs WHERE filepath = ? AND id != ?`, local, target).Scan(&holder); err == nil {
			continue // local path already tracked by another row
		}
		if _, err := s.db.Exec(`UPDATE tabs SET filepath = ? WHERE id = ?`, local, target); err != nil {
			continue
		}
	}
	return unresolved, nil
}

// fileMatches reports whether the file at path exists and, when want is
// non-empty, hashes to exactly want.
func fileMatches(path, want string) (bool, error) {
	h, err := hashFile(path)
	if err != nil {
		return false, err
	}
	if want != "" && h != want {
		return false, nil
	}
	return true, nil
}
