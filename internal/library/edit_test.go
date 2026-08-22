package library

import (
	"errors"
	"path/filepath"
	"testing"

	"fretboard/internal/model"
)

// TestReimportNeedsDecision pins 6.2: untouched rows re-import silently, but
// a row with local edits (edited_at > 0) or diverging title/artist surfaces
// ErrNeedsDecision carrying the affected row.
func TestReimportNeedsDecision(t *testing.T) {
	st, err := NewStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	tab := &model.Tab{Title: "Song", Artist: "Artist", Tuning: model.Standard}
	id, err := st.Import("s.txt", tab)
	if err != nil {
		t.Fatal(err)
	}

	// Untouched re-import: silent, same row.
	id2, err := st.Import("s.txt", tab)
	if err != nil {
		t.Fatalf("untouched re-import should succeed, got %v", err)
	}
	if id2 != id {
		t.Fatalf("re-import id = %d, want %d", id2, id)
	}

	// Diverging title -> decision required, row attached.
	_, err = st.Import("s.txt", &model.Tab{Title: "Changed", Artist: "Artist"})
	if !errors.Is(err, ErrNeedsDecision) {
		t.Fatalf("diverging title err = %v, want ErrNeedsDecision", err)
	}
	var nde *NeedsDecisionError
	if !errors.As(err, &nde) || nde.Row == nil || nde.Row.ID != id {
		t.Fatalf("ErrNeedsDecision must carry the row, got %+v", nde)
	}

	// Diverging artist also triggers.
	_, err = st.Import("s.txt", &model.Tab{Title: "Song", Artist: "Other"})
	if !errors.Is(err, ErrNeedsDecision) {
		t.Fatalf("diverging artist err = %v, want ErrNeedsDecision", err)
	}

	// Local edit stamps edited_at; re-import of identical content now needs
	// a decision.
	if err := st.UpdateMeta(id, "Song", "Artist"); err != nil {
		t.Fatal(err)
	}
	editedAt, err := st.RowEditedAt(id)
	if err != nil {
		t.Fatal(err)
	}
	if editedAt == 0 {
		t.Fatal("UpdateMeta should stamp edited_at")
	}
	_, err = st.Import("s.txt", tab)
	if !errors.Is(err, ErrNeedsDecision) {
		t.Fatalf("edited row re-import err = %v, want ErrNeedsDecision", err)
	}

	// RowEditedAt on a fresh row is 0; on a missing row it errors.
	plain, err := st.Import("p.txt", &model.Tab{Title: "Plain"})
	if err != nil {
		t.Fatal(err)
	}
	if at, _ := st.RowEditedAt(plain); at != 0 {
		t.Fatalf("fresh row edited_at = %d, want 0", at)
	}
	if _, err := st.RowEditedAt(999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("RowEditedAt(999) err = %v, want ErrNotFound", err)
	}
}

// TestImportOverwrite resolves the decision the "file wins" way: content,
// path, and hash come from the file and the edit marker clears.
func TestImportOverwrite(t *testing.T) {
	dir := t.TempDir()
	st, err := NewStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	id, err := st.Import("s.txt", &model.Tab{Title: "Old", Artist: "O", Tuning: model.Standard})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateMeta(id, "Edited", "E"); err != nil {
		t.Fatal(err)
	}

	fp := filepath.Join(dir, "song.txt")
	writeTestTab(t, fp, "Title: File Title\nArtist: File Artist\nTuning: E Standard\n\ne|0-3-5|\n")
	if err := st.ImportOverwrite(id, fp); err != nil {
		t.Fatal(err)
	}
	row, err := st.GetRow(id)
	if err != nil {
		t.Fatal(err)
	}
	if row.Title != "File Title" || row.Artist != "File Artist" {
		t.Fatalf("overwrite should replace meta: %+v", row)
	}
	if row.Filepath != fp {
		t.Fatalf("overwrite should re-point at file: %q", row.Filepath)
	}
	if row.EditedAt != 0 {
		t.Fatalf("overwrite should clear the edit marker, got %d", row.EditedAt)
	}
	want, _ := hashFile(fp)
	if row.ContentHash != want {
		t.Fatalf("overwrite hash = %q, want %q", row.ContentHash, want)
	}

	// Errors: missing row, and path claimed by another row.
	if err := st.ImportOverwrite(999, fp); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ImportOverwrite(999) err = %v, want ErrNotFound", err)
	}
	id2, err := st.Import("other.txt", &model.Tab{Title: "O2", Artist: "A"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ImportOverwrite(id2, fp); err == nil {
		t.Fatal("overwrite onto another row's path should fail")
	}
}

// TestKeepExisting resolves the decision the "edits win" way: only the
// filepath changes; content, title, artist, and edit marker survive.
func TestKeepExisting(t *testing.T) {
	dir := t.TempDir()
	st, err := NewStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	id, err := st.Import("s.txt", &model.Tab{Title: "Song", Artist: "Artist"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateMeta(id, "Edited Title", "Edited Artist"); err != nil {
		t.Fatal(err)
	}

	fp := filepath.Join(dir, "moved.txt")
	writeTestTab(t, fp, "Title: File Title\nArtist: File Artist\nTuning: E Standard\n\ne|0-3-5|\n")
	if err := st.KeepExisting(id, fp); err != nil {
		t.Fatal(err)
	}
	row, err := st.GetRow(id)
	if err != nil {
		t.Fatal(err)
	}
	if row.Filepath != fp {
		t.Fatalf("KeepExisting should record the new path, got %q", row.Filepath)
	}
	if row.Title != "Edited Title" || row.Artist != "Edited Artist" {
		t.Fatalf("KeepExisting must not clobber meta: %+v", row)
	}
	if row.EditedAt == 0 {
		t.Fatal("KeepExisting must keep the edit marker")
	}
	tab, err := st.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if tab.Title != "Edited Title" {
		t.Fatalf("stored content clobbered: %+v", tab)
	}

	if err := st.KeepExisting(999, fp); !errors.Is(err, ErrNotFound) {
		t.Fatalf("KeepExisting(999) err = %v, want ErrNotFound", err)
	}
	id2, err := st.Import("other.txt", &model.Tab{Title: "O2"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.KeepExisting(id2, fp); err == nil {
		t.Fatal("KeepExisting onto another row's path should fail")
	}
}

// TestUpdateMetaStampsEditedAt guards the edit-awareness contract: any
// UpdateMeta call marks the row edited even when values are unchanged.
func TestUpdateMetaStampsEditedAt(t *testing.T) {
	st, err := NewStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	id, err := st.Import("s.txt", &model.Tab{Title: "Song", Artist: "A"})
	if err != nil {
		t.Fatal(err)
	}
	if at, _ := st.RowEditedAt(id); at != 0 {
		t.Fatalf("before UpdateMeta edited_at = %d, want 0", at)
	}
	if err := st.UpdateMeta(id, "Song", "A"); err != nil {
		t.Fatal(err)
	}
	at, err := st.RowEditedAt(id)
	if err != nil {
		t.Fatal(err)
	}
	if at == 0 {
		t.Fatal("UpdateMeta must stamp edited_at even for a no-op change")
	}
	// Row stays functional: still listed and searchable.
	rows, err := st.List()
	if err != nil || len(rows) != 1 || rows[0].ID != id {
		t.Fatalf("List after UpdateMeta: %+v, %v", rows, err)
	}
}
