package library

import (
	"errors"
	"path/filepath"
	"testing"

	"fretboard/internal/model"
)

// TestMutationMethods pins the update/delete behaviors of the store so the
// shared rows-affected + not-found handling can be deduplicated safely.
func TestMutationMethods(t *testing.T) {
	st, err := NewStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	id, err := st.Import("m.txt", &model.Tab{Title: "Mutation", Artist: "A"})
	if err != nil {
		t.Fatal(err)
	}

	// SetFavorite flips the row flag in both directions.
	if err := st.SetFavorite(id, true); err != nil {
		t.Fatal(err)
	}
	row, err := st.GetRow(id)
	if err != nil {
		t.Fatal(err)
	}
	if !row.Favorite {
		t.Fatal("SetFavorite(true) should set favorite")
	}
	if err := st.SetFavorite(id, false); err != nil {
		t.Fatal(err)
	}
	row, err = st.GetRow(id)
	if err != nil {
		t.Fatal(err)
	}
	if row.Favorite {
		t.Fatal("SetFavorite(false) should clear favorite")
	}

	// RecordPlay increments play_count and stamps last_played.
	if err := st.RecordPlay(id); err != nil {
		t.Fatal(err)
	}
	if err := st.RecordPlay(id); err != nil {
		t.Fatal(err)
	}
	row, err = st.GetRow(id)
	if err != nil {
		t.Fatal(err)
	}
	if row.PlayCount != 2 {
		t.Fatalf("play_count = %d, want 2", row.PlayCount)
	}
	if row.LastPlayed == "" {
		t.Fatal("RecordPlay should set last_played")
	}

	// Mutations against a missing row surface ErrNotFound.
	if err := st.SetFavorite(999, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("SetFavorite(999) err = %v, want ErrNotFound", err)
	}
	if err := st.RecordPlay(999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("RecordPlay(999) err = %v, want ErrNotFound", err)
	}
	if err := st.UpdateMeta(999, "x", "y"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("UpdateMeta(999) err = %v, want ErrNotFound", err)
	}
	if err := st.Delete(999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete(999) err = %v, want ErrNotFound", err)
	}

	// Delete removes the row from the library.
	if err := st.Delete(id); err != nil {
		t.Fatal(err)
	}
	rows, err := st.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("List after Delete = %d rows, want 0", len(rows))
	}
}
