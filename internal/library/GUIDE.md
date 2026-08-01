### WHAT THIS PACKAGE DOES
Persistent storage for your tab collection. Import tabs, search them, tag them,
track play counts. Like a music library but for tabs.

### FILE: db.go

#### WHAT IT DOES
Creates the SQLite schema, provides CRUD operations for tabs.

#### HOW TO THINK ABOUT IT
You're using `database/sql` (Go's standard DB interface) with a pure-Go
SQLite driver. The pattern is always the same:
1. Open DB.
2. Run migrations (CREATE TABLE IF NOT EXISTS).
3. Implement thin functions: `InsertTab(t *Tab), ListTabs(), SearchTabs(query), GetTab(id)`.

#### STEP-BY-STEP
1. Import `modernc.org/sqlite` with a blank import: `_ "modernc.org/sqlite"`.
   This registers the driver. You use `database/sql` API as normal.
2. `func OpenDB(path string) (*sql.DB, error)` — calls `sql.Open("sqlite", path)`.
   Enable WAL mode and foreign keys with PRAGMAs.
3. `func Migrate(db *sql.DB) error` — runs CREATE TABLE statements.
   Use a simple version table approach or just IF NOT EXISTS.
4. CRUD functions — each takes `*sql.DB` and returns domain objects:
   ```
   InsertTab(db, tab) (int64, error)      // returns new ID
   TabByID(db, id) (*Tab, error)
   TabsByFilter(db, filter) ([]*Tab, error)
   DeleteTab(db, id) error
   UpdateMeta(db, id, meta) error
   ```

#### SCHEMA
```
CREATE TABLE IF NOT EXISTS tabs (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    filepath   TEXT NOT NULL UNIQUE,
    title      TEXT NOT NULL DEFAULT 'Untitled',
    artist     TEXT NOT NULL DEFAULT 'Unknown',
    tuning     TEXT NOT NULL DEFAULT '["E2","A2","D3","G3","B3","E4"]',
    added_at   TEXT DEFAULT (datetime('now')),
    last_played TEXT,
    play_count  INTEGER DEFAULT 0,
    favorite   INTEGER DEFAULT 0    -- 0 or 1
);
```

#### GO CONCEPTS
- `database/sql` — standard library database interface.
- `sql.DB`, `sql.Rows`, `sql.Stmt` — key types.
- Row scanning: `row.Scan(&field1, &field2, ...)`.
- `database/sql` uses prepared statements internally. No need to manually prepare.
- SQLite PRAGMAs: `db.Exec("PRAGMA journal_mode=WAL")`.

#### GOTCHAS
- Always check `err` after `rows.Err()` when iterating, not just after `rows.Next()`.
- Close rows with `defer rows.Close()`.
- Don't share `*sql.DB` across goroutines without care — it's safe for concurrent use but
  SQLite has a single-writer lock. Modernc.org/sqlite handles this better than CGo sqlite3.
- JSON fields (tuning): marshal/unmarshal with `encoding/json` before storing/loading.
  Consider using `database/sql`'s `Scanner` interface for custom types.
- Foreign keys: SQLite needs `PRAGMA foreign_keys = ON` at connection start.

#### IF STUCK
- "golang database/sql tutorial" — essential starting point
- "modernc.org/sqlite example" — the pure-Go driver docs
- "golang sqlite3 json column" — storing arrays in sqlite
- "database/sql rows scan into struct"

#### SKELETON

import (
    "database/sql"
    _ "modernc.org/sqlite"
)

type Store struct {
    db *sql.DB
}

func NewStore(path string) (*Store, error) {
    db, err := sql.Open("sqlite", path)
    if err != nil { return nil, err }
    db.Exec("PRAGMA journal_mode=WAL")
    db.Exec("PRAGMA foreign_keys=ON")
    s := &Store{db: db}
    return s, s.migrate()
}

func (s *Store) migrate() error {
    _, err := s.db.Exec(`
        CREATE TABLE IF NOT EXISTS tabs (
            id         INTEGER PRIMARY KEY AUTOINCREMENT,
            filepath   TEXT NOT NULL UNIQUE,
            title      TEXT NOT NULL DEFAULT '',
            artist     TEXT NOT NULL DEFAULT '',
            tuning     TEXT NOT NULL DEFAULT '',
            added_at   TEXT DEFAULT (datetime('now')),
            last_played TEXT,
            play_count  INTEGER DEFAULT 0,
            favorite   INTEGER DEFAULT 0
        );
    `)
    return err
}

func (s *Store) Insert(title, artist, filepath, tuning string) (int64, error) {
    res, err := s.db.Exec(
        "INSERT INTO tabs (title, artist, filepath, tuning) VALUES (?, ?, ?, ?)",
        title, artist, filepath, tuning,
    )
    if err != nil { return 0, err }
    return res.LastInsertId()
}

func (s *Store) List() ([]TabRow, error) {
    rows, err := s.db.Query("SELECT id, title, artist, favorite FROM tabs ORDER BY title")
    if err != nil { return nil, err }
    defer rows.Close()
    var tabs []TabRow
    for rows.Next() {
        var t TabRow
        rows.Scan(&t.ID, &t.Title, &t.Artist, &t.Favorite)
        tabs = append(tabs, t)
    }
    return tabs, rows.Err()
}
