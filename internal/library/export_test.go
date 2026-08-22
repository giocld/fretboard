package library

import (
	"os"
	"path/filepath"
	"testing"

	"fretboard/internal/model"
)

// setTabsDir swaps the tabsDir root for the duration of a test.
func setTabsDir(t *testing.T, root string) {
	t.Helper()
	old := tabsDir
	tabsDir = func() string { return root }
	t.Cleanup(func() { tabsDir = old })
}

// TestExportImportRoundTrip pins 6.4: an archive written under one tabs root
// restores all rows into a fresh store, resolves files copied under a new
// root (relinking the rows), and reports what is still unresolved.
func TestExportImportRoundTrip(t *testing.T) {
	srcDir := t.TempDir()
	srcRoot := filepath.Join(srcDir, "tabs")
	setTabsDir(t, srcRoot)

	st, err := NewStore(filepath.Join(srcDir, "lib.db"))
	if err != nil {
		t.Fatal(err)
	}

	// A real file under the tabs root.
	fp := filepath.Join(srcRoot, "songs", "song.txt")
	writeTestTab(t, fp, testTabBody)
	id, err := st.ImportFile(fp)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetStatus(id, "learning"); err != nil {
		t.Fatal(err)
	}
	if err := st.AddTag(id, "rock"); err != nil {
		t.Fatal(err)
	}
	slID, err := st.CreateSetlist("My Set")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AddToSetlist(slID, id); err != nil {
		t.Fatal(err)
	}
	if err := st.RecordPractice(id, 300, 110, 2); err != nil {
		t.Fatal(err)
	}
	// A virtual row (no backing file) exercises fallback hashes.
	vid, err := st.Import("online://ug/9", &model.Tab{Title: "Virtual", Artist: "V"})
	if err != nil {
		t.Fatal(err)
	}

	archivePath := filepath.Join(srcDir, "lib-export.json")
	if err := st.ExportArchive(archivePath); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	// Determinism: a second export of the same state is byte-identical.
	stAgain, err := NewStore(filepath.Join(srcDir, "lib.db"))
	if err != nil {
		t.Fatal(err)
	}
	again := filepath.Join(srcDir, "lib-export-2.json")
	if err := stAgain.ExportArchive(again); err != nil {
		t.Fatal(err)
	}
	stAgain.Close()
	a1, _ := os.ReadFile(archivePath)
	a2, _ := os.ReadFile(again)
	if string(a1) != string(a2) {
		t.Fatal("archives of identical state must be byte-identical")
	}

	// Import into a fresh store whose tabs root is elsewhere; the file has
	// not been copied yet, so its manifest entry is unresolved.
	dstDir := t.TempDir()
	dstRoot := filepath.Join(dstDir, "tabs")
	setTabsDir(t, dstRoot)
	st2, err := NewStore(filepath.Join(dstDir, "lib.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()

	unresolved, err := st2.ImportArchive(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(unresolved) == 0 {
		t.Fatal("expected unresolved manifest entries before copying files")
	}

	// All rows restored, even without the files.
	rows, err := st2.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("List after import = %d rows, want 2", len(rows))
	}
	row, err := st2.GetRow(id)
	if err != nil {
		t.Fatal(err)
	}
	if row.Title != "Hash Song" || row.Status != "learning" {
		t.Fatalf("restored row = %+v", row)
	}
	tags, err := st2.TagsFor(id)
	if err != nil || len(tags) != 1 || tags[0] != "rock" {
		t.Fatalf("restored tags = %v, %v", tags, err)
	}
	lists, err := st2.Setlists()
	if err != nil || len(lists) != 1 || lists[0].Name != "My Set" {
		t.Fatalf("restored setlists = %+v, %v", lists, err)
	}
	slTabs, err := st2.SetlistTabs(lists[0].ID)
	if err != nil || len(slTabs) != 1 || slTabs[0].ID != id {
		t.Fatalf("restored setlist tabs = %+v, %v", slTabs, err)
	}
	total, byTab, err := st2.PracticeStats(4000)
	if err != nil || total != 5 || len(byTab) != 1 {
		t.Fatalf("restored practice = %d %+v, %v", total, byTab, err)
	}
	if _, err := st2.GetRow(vid); err != nil {
		t.Fatalf("virtual row not restored: %v", err)
	}

	// Copy the file under the new root and re-import: the entry resolves and
	// the row is relinked to the local copy.
	writeTestTab(t, filepath.Join(dstRoot, "songs", "song.txt"), testTabBody)
	unresolved, err = st2.ImportArchive(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	stillMissing := false
	for _, u := range unresolved {
		if filepath.ToSlash(u) == "songs/song.txt" {
			stillMissing = true
		}
	}
	if stillMissing {
		t.Fatalf("copied file should resolve, unresolved = %v", unresolved)
	}
	row2, err := st2.GetRow(id)
	if err != nil {
		t.Fatal(err)
	}
	wantPath := filepath.Join(dstRoot, "songs", "song.txt")
	if row2.Filepath != wantPath {
		t.Fatalf("row filepath = %q, want relinked to %q", row2.Filepath, wantPath)
	}
}

// TestExportAbsoluteWhenNoRoot: with TabsDir unset, the manifest stores
// absolute paths and import resolves them directly when present.
func TestExportAbsoluteWhenNoRoot(t *testing.T) {
	dir := t.TempDir()
	setTabsDir(t, "") // unset root

	st, err := NewStore(filepath.Join(dir, "lib.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	fp := filepath.Join(dir, "abs.txt")
	writeTestTab(t, fp, testTabBody)
	id, err := st.ImportFile(fp)
	if err != nil {
		t.Fatal(err)
	}

	archivePath := filepath.Join(dir, "abs-export.json")
	if err := st.ExportArchive(archivePath); err != nil {
		t.Fatal(err)
	}

	// Fresh store, same absolute file still on disk: everything resolves.
	st2, err := NewStore(filepath.Join(dir, "lib2.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	unresolved, err := st2.ImportArchive(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(unresolved) != 0 {
		t.Fatalf("absolute manifest with present files should fully resolve, got %v", unresolved)
	}
	row, err := st2.GetRow(id)
	if err != nil {
		t.Fatal(err)
	}
	if row.Filepath != fp {
		t.Fatalf("filepath = %q, want %q", row.Filepath, fp)
	}
}

// TestImportArchiveRejectsUnsupportedVersion guards forward compatibility.
func TestImportArchiveRejectsUnsupportedVersion(t *testing.T) {
	dir := t.TempDir()
	st, err := NewStore(filepath.Join(dir, "lib.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte(`{"version": 99, "tabs": [], "tags": [], "tab_tags": [], "setlists": [], "setlist_items": [], "practice_events": [], "files": []}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ImportArchive(bad); err == nil {
		t.Fatal("unsupported archive version should be rejected")
	}
}
