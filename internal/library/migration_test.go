package library

import (
	"database/sql"
	"path/filepath"
	"testing"

	"fretboard/internal/model"
	_ "modernc.org/sqlite"
)

func TestMigrateDropsLegacyColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v1.db")

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
			difficulty  INTEGER DEFAULT 0,
			tags        TEXT DEFAULT '[]',
			added_at    TEXT DEFAULT (datetime('now')),
			last_played TEXT,
			play_count  INTEGER DEFAULT 0,
			favorite    INTEGER DEFAULT 0
		);
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO tabs (filepath, title, artist, tuning, content, difficulty, tags)
		VALUES ('a.txt', 'A', 'Artist', '["E2"]', '{}', 3, '["x"]')
	`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	st, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore on v1 schema: %v", err)
	}
	defer st.Close()

	for _, col := range []string{"difficulty", "tags"} {
		var n int
		if err := st.db.QueryRow(
			`SELECT COUNT(*) FROM pragma_table_info('tabs') WHERE name = ?`, col,
		).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatalf("legacy column %q should have been dropped", col)
		}
	}

	rows, err := st.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Title != "A" {
		t.Fatalf("existing rows should survive migration, got %+v", rows)
	}

	id, err := st.Import("b.txt", &model.Tab{Title: "B", Artist: "Artist"})
	if err != nil {
		t.Fatalf("Import after migration: %v", err)
	}
	if _, err := st.Get(id); err != nil {
		t.Fatalf("Get after migration: %v", err)
	}
}
