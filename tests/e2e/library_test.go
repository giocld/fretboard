package e2e_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fretboard/internal/library"
	"fretboard/internal/parser"
	"fretboard/tests/helpers"
)

func TestStoreFullLifecycleE2E(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "library.db")

	store, err := library.NewStore(path)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	tab, err := parser.Parse(strings.NewReader(helpers.SultansTab))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	id, err := store.Import("test.txt", tab)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if id <= 0 {
		t.Fatalf("expected positive id, got %d", id)
	}

	list, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 tab in list, got %d", len(list))
	}
	if list[0].Title != "Sultans of Swing" || list[0].Artist != "Dire Straits" {
		t.Errorf("list row mismatch: got %+v", list[0])
	}

	got, err := store.Get(id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil || got.Title != "Sultans of Swing" {
		t.Fatalf("Get returned wrong tab: %+v", got)
	}
	if len(got.Tuning) != 6 {
		t.Errorf("tuning not preserved, got %v", got.Tuning)
	}
	if len(got.Bars) < 2 {
		t.Errorf("bars not preserved, got %d", len(got.Bars))
	}

	search, err := store.Search("Sultans")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(search) != 1 {
		t.Fatalf("expected 1 search result, got %d", len(search))
	}
	searchNone, err := store.Search("ZZZ")
	if err != nil {
		t.Fatalf("Search empty: %v", err)
	}
	if len(searchNone) != 0 {
		t.Fatalf("expected 0 search results for ZZZ, got %d", len(searchNone))
	}

	if err := store.SetFavorite(id, true); err != nil {
		t.Fatalf("SetFavorite: %v", err)
	}
	row, err := store.GetRow(id)
	if err != nil {
		t.Fatalf("GetRow after favorite: %v", err)
	}
	if !row.Favorite {
		t.Errorf("expected favorite=true, got %v", row.Favorite)
	}

	if err := store.Delete(id); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	listAfter, err := store.List()
	if err != nil {
		t.Fatalf("List after delete: %v", err)
	}
	if len(listAfter) != 0 {
		t.Fatalf("expected 0 tabs after delete, got %d", len(listAfter))
	}
	_, err = store.Get(id)
	if err == nil {
		t.Fatalf("expected error after Get of deleted id")
	}
}

func TestStoreImportFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "library.db")
	store, err := library.NewStore(path)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	fp := filepath.Join(dir, "smoke.txt")
	if err := os.WriteFile(fp, []byte(helpers.SultansTab), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	id, err := store.ImportFile(fp)
	if err != nil {
		t.Fatalf("ImportFile: %v", err)
	}
	if id <= 0 {
		t.Fatalf("expected positive id, got %d", id)
	}
	row, err := store.GetRow(id)
	if err != nil {
		t.Fatalf("GetRow: %v", err)
	}
	if row.Filepath != fp {
		t.Errorf("filepath mismatch: got %q", row.Filepath)
	}
}

func TestStoreDuplicateFilepath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "library.db")
	store, err := library.NewStore(path)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	tab, _ := parser.Parse(strings.NewReader(helpers.SultansTab))
	id1, err := store.Import("dup.txt", tab)
	if err != nil {
		t.Fatalf("first import: %v", err)
	}
	id2, err := store.Import("dup.txt", tab)
	if err != nil {
		t.Fatalf("second import: %v", err)
	}
	if id1 != id2 {
		t.Fatalf("duplicate import should return same id, got %d and %d", id1, id2)
	}
	list, _ := store.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 tab after duplicate import, got %d", len(list))
	}
}
