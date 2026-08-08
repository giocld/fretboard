package library

import (
	"database/sql"
	"path/filepath"
	"testing"

	"fretboard/internal/model"
	_ "modernc.org/sqlite"
)

// TestImportPersistsSourceBadge verifies the provenance badge flows from tab
// metadata into the source_badge column and back through every row query.
func TestImportPersistsSourceBadge(t *testing.T) {
	st, err := NewStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	tab := &model.Tab{
		Title:    "Sultans of Swing",
		Artist:   "Dire Straits",
		Tuning:   model.Standard,
		Metadata: map[string]string{model.MetaKeySourceBadge: "[UG ★4.9]"},
	}
	id, err := st.Import("sultans.txt", tab)
	if err != nil {
		t.Fatal(err)
	}

	row, err := st.GetRow(id)
	if err != nil {
		t.Fatal(err)
	}
	if row.SourceBadge != "[UG ★4.9]" {
		t.Fatalf("GetRow badge = %q, want [UG ★4.9]", row.SourceBadge)
	}

	list, err := st.List()
	if err != nil || len(list) != 1 || list[0].SourceBadge != "[UG ★4.9]" {
		t.Fatalf("List badge wrong: %+v, err %v", list, err)
	}

	found, err := st.Search("sultans")
	if err != nil || len(found) != 1 || found[0].SourceBadge != "[UG ★4.9]" {
		t.Fatalf("Search badge wrong: %+v, err %v", found, err)
	}

	// A re-import without metadata clears the badge (content is the source
	// of truth; a local file re-import is not an online tab anymore).
	plain := &model.Tab{Title: "Sultans of Swing", Artist: "Dire Straits", Tuning: model.Standard}
	if _, err := st.Import("sultans.txt", plain); err != nil {
		t.Fatal(err)
	}
	row2, err := st.GetRow(id)
	if err != nil {
		t.Fatal(err)
	}
	if row2.SourceBadge != "" {
		t.Fatalf("re-import without metadata should clear badge, got %q", row2.SourceBadge)
	}
}

// TestMigrationAddsSourceBadgeColumn verifies existing databases (created
// before the column existed) get the column added by NewStore.
func TestMigrationAddsSourceBadgeColumn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v15.db")

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		CREATE TABLE tabs (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			filepath    TEXT NOT NULL UNIQUE,
			title       TEXT NOT NULL DEFAULT '',
			artist      TEXT NOT NULL DEFAULT '',
			tuning      TEXT NOT NULL DEFAULT '',
			content     TEXT NOT NULL DEFAULT '',
			added_at    TEXT DEFAULT (datetime('now')),
			last_played TEXT,
			play_count  INTEGER DEFAULT 0,
			favorite    INTEGER DEFAULT 0
		);
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO tabs (filepath, title, artist) VALUES ('a.txt', 'A', 'Artist')
	`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	st, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore on pre-badge schema: %v", err)
	}
	defer st.Close()

	var n int
	if err := st.db.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('tabs') WHERE name = 'source_badge'`,
	).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatal("source_badge column should have been added by migration")
	}

	// Existing rows read back with an empty badge.
	rows, err := st.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].SourceBadge != "" {
		t.Fatalf("migrated rows should have empty badges, got %+v", rows)
	}

	// And new imports write the badge.
	id, err := st.Import("b.txt", &model.Tab{
		Title:    "B",
		Artist:   "Artist",
		Metadata: map[string]string{model.MetaKeySourceBadge: "[ST]"},
	})
	if err != nil {
		t.Fatal(err)
	}
	row, err := st.GetRow(id)
	if err != nil {
		t.Fatal(err)
	}
	if row.SourceBadge != "[ST]" {
		t.Fatalf("badge = %q after migration, want [ST]", row.SourceBadge)
	}
}
