package library

import (
	"os"
	"path/filepath"
	"testing"

	"fretboard/internal/model"
)

const testTabBody = "Title: Hash Song\nArtist: Hash Artist\nTuning: E Standard\n\ne|0-3-5|\n"

func writeTestTab(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestImportSetsContentHash pins 6.1: every import stores a sha256 and the
// hash survives a store reload. Virtual paths (no backing file) fall back to
// hashing the serialized content, which is stable across re-imports.
func TestImportSetsContentHash(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	st, err := NewStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}

	// Virtual import: fallback hash, deterministic for identical tabs.
	id, err := st.Import("virtual.txt", &model.Tab{Title: "A", Artist: "B", Tuning: model.Standard})
	if err != nil {
		t.Fatal(err)
	}
	row, err := st.GetRow(id)
	if err != nil {
		t.Fatal(err)
	}
	if row.ContentHash == "" {
		t.Fatal("Import should set content_sha256")
	}
	firstHash := row.ContentHash
	if id2, err := st.Import("virtual.txt", &model.Tab{Title: "A", Artist: "B", Tuning: model.Standard}); err != nil || id2 != id {
		t.Fatalf("re-import of identical tab: id %d err %v", id2, err)
	}
	row, _ = st.GetRow(id)
	if row.ContentHash != firstHash {
		t.Fatalf("fallback hash changed across re-import: %q -> %q", firstHash, row.ContentHash)
	}

	// Real file import: hash covers the raw file bytes.
	fp := filepath.Join(dir, "tabs", "song.txt")
	writeTestTab(t, fp, testTabBody)
	id2, err := st.ImportFile(fp)
	if err != nil {
		t.Fatal(err)
	}
	row2, err := st.GetRow(id2)
	if err != nil {
		t.Fatal(err)
	}
	want, err := hashFile(fp)
	if err != nil {
		t.Fatal(err)
	}
	if row2.ContentHash != want {
		t.Fatalf("file hash = %q, want %q", row2.ContentHash, want)
	}

	// Survives reload.
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st2, err := NewStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	r1, err := st2.GetRow(id)
	if err != nil {
		t.Fatal(err)
	}
	if r1.ContentHash != firstHash {
		t.Fatalf("hash after reload = %q, want %q", r1.ContentHash, firstHash)
	}
	r2, err := st2.GetRow(id2)
	if err != nil {
		t.Fatal(err)
	}
	if r2.ContentHash != want {
		t.Fatalf("file hash after reload = %q, want %q", r2.ContentHash, want)
	}
}

// TestScanForRelocationsAutoRelinks: a moved file whose bytes match the row's
// stored hash is found and the row is re-pointed at the new location.
func TestScanForRelocationsAutoRelinks(t *testing.T) {
	dir := t.TempDir()
	st, err := NewStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	src := filepath.Join(dir, "old", "song.txt")
	writeTestTab(t, src, testTabBody)
	id, err := st.ImportFile(src)
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "new", "song.txt")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(src, dst); err != nil {
		t.Fatal(err)
	}

	relocs, err := st.ScanForRelocations(filepath.Join(dir, "new"))
	if err != nil {
		t.Fatal(err)
	}
	if len(relocs) != 1 || relocs[0].Ambiguous {
		t.Fatalf("relocations = %+v, want one unambiguous match", relocs)
	}
	if relocs[0].Path != src || relocs[0].FoundAt != dst || relocs[0].RowID != id {
		t.Fatalf("relocation = %+v, want Path=%s FoundAt=%s", relocs[0], src, dst)
	}
	row, err := st.GetRow(id)
	if err != nil {
		t.Fatal(err)
	}
	if row.Filepath != dst {
		t.Fatalf("row filepath = %q, want auto-relinked to %q", row.Filepath, dst)
	}

	// A second scan finds nothing: the row is in place now.
	relocs, err = st.ScanForRelocations(filepath.Join(dir, "new"))
	if err != nil {
		t.Fatal(err)
	}
	if len(relocs) != 0 {
		t.Fatalf("second scan = %+v, want no relocations", relocs)
	}
}

// TestScanForRelocationsAmbiguous: two identical candidate files leave the
// row untouched and both candidates are reported.
func TestScanForRelocationsAmbiguous(t *testing.T) {
	dir := t.TempDir()
	st, err := NewStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	src := filepath.Join(dir, "orig", "song.txt")
	writeTestTab(t, src, testTabBody)
	id, err := st.ImportFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(src); err != nil {
		t.Fatal(err)
	}
	c1 := filepath.Join(dir, "copy1", "song.txt")
	c2 := filepath.Join(dir, "copy2", "song.txt")
	writeTestTab(t, c1, testTabBody)
	writeTestTab(t, c2, testTabBody)

	relocs, err := st.ScanForRelocations(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(relocs) != 2 {
		t.Fatalf("relocations = %+v, want two ambiguous candidates", relocs)
	}
	for _, r := range relocs {
		if !r.Ambiguous {
			t.Fatalf("relocation %+v should be ambiguous", r)
		}
	}
	row, err := st.GetRow(id)
	if err != nil {
		t.Fatal(err)
	}
	if row.Filepath != src {
		t.Fatalf("ambiguous scan must not relink; filepath = %q", row.Filepath)
	}
}

// TestScanForRelocationsClaimedCandidate: the only matching file is already
// tracked by another row, so the match is ambiguous and nothing is relinked.
func TestScanForRelocationsClaimedCandidate(t *testing.T) {
	dir := t.TempDir()
	st, err := NewStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	fa := filepath.Join(dir, "a", "same.txt")
	fb := filepath.Join(dir, "b", "same.txt")
	writeTestTab(t, fa, testTabBody)
	writeTestTab(t, fb, testTabBody)
	idA, err := st.ImportFile(fa)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.ImportFile(fb); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(fa); err != nil {
		t.Fatal(err)
	}

	relocs, err := st.ScanForRelocations(filepath.Join(dir, "b"))
	if err != nil {
		t.Fatal(err)
	}
	if len(relocs) != 1 || !relocs[0].Ambiguous {
		t.Fatalf("relocations = %+v, want one ambiguous claimed-candidate match", relocs)
	}
	row, err := st.GetRow(idA)
	if err != nil {
		t.Fatal(err)
	}
	if row.Filepath != fa {
		t.Fatalf("claimed candidate must not relink; filepath = %q", row.Filepath)
	}
}

// TestMissingRowsKeepsRows: deleted files surface via MissingRows but the
// rows stay in the library, and virtual online paths are not reported.
func TestMissingRowsKeepsRows(t *testing.T) {
	dir := t.TempDir()
	st, err := NewStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	fp := filepath.Join(dir, "song.txt")
	writeTestTab(t, fp, testTabBody)
	id, err := st.ImportFile(fp)
	if err != nil {
		t.Fatal(err)
	}
	vid, err := st.Import("online://ug/9", &model.Tab{Title: "Virtual", Artist: "V"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(fp); err != nil {
		t.Fatal(err)
	}

	missing, err := st.MissingRows()
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 1 || missing[0].ID != id {
		t.Fatalf("missing rows = %+v, want exactly the deleted local tab", missing)
	}
	// Rows stay in the library.
	if _, err := st.GetRow(id); err != nil {
		t.Fatalf("missing tab was deleted: %v", err)
	}
	if _, err := st.GetRow(vid); err != nil {
		t.Fatalf("virtual tab vanished: %v", err)
	}
}

// TestRelink: direct relink works and refuses paths claimed by another row.
func TestRelink(t *testing.T) {
	st, err := NewStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	id1, err := st.Import("a.txt", &model.Tab{Title: "A"})
	if err != nil {
		t.Fatal(err)
	}
	id2, err := st.Import("b.txt", &model.Tab{Title: "B"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Relink(id1, "b.txt"); err == nil {
		t.Fatal("relink onto another row's path should fail")
	}
	if err := st.Relink(id1, "c.txt"); err != nil {
		t.Fatalf("relink to free path: %v", err)
	}
	row, _ := st.GetRow(id1)
	if row.Filepath != "c.txt" {
		t.Fatalf("filepath = %q, want c.txt", row.Filepath)
	}
	if err := st.Relink(999, "d.txt"); err == nil {
		t.Fatal("relink of missing row should fail")
	}
	if _, err := st.GetRow(id2); err != nil {
		t.Fatalf("other row must be untouched: %v", err)
	}
}
