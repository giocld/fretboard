package library

import (
	"path/filepath"
	"testing"
)

func TestMoreRecentlyUsed(t *testing.T) {
	played := TabRow{ID: 1, LastPlayed: "2026-02-01 12:00:00"}
	newer := TabRow{ID: 2, LastPlayed: "2026-03-01 12:00:00"}
	never := TabRow{ID: 3}
	if !MoreRecentlyUsed(newer, played) || MoreRecentlyUsed(played, newer) {
		t.Fatal("expected newer last_played first")
	}
	if !MoreRecentlyUsed(played, never) || MoreRecentlyUsed(never, played) {
		t.Fatal("played tabs should beat never-opened tabs")
	}
}

func TestSearchEscapesUnderscore(t *testing.T) {
	st, err := NewStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	insert := func(title, artist string) {
		if _, err := st.db.Exec(
			`INSERT INTO tabs (filepath, title, artist) VALUES (?, ?, ?)`,
			title+"/"+artist, title, artist,
		); err != nil {
			t.Fatal(err)
		}
	}
	insert("AB_C", "Artist")
	insert("BXC", "Artist")

	res, err := st.Search("B_C")
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].Title != "AB_C" {
		t.Fatalf("Search(B_C) returned %d rows (titles %q), want exactly AB_C", len(res), func() []string {
			var out []string
			for _, r := range res {
				out = append(out, r.Title)
			}
			return out
		}())
	}
}
