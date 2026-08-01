package e2e_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/YOUR_USERNAME/fretboard/internal/library"
	"github.com/YOUR_USERNAME/fretboard/tests/helpers"
)

func TestImportDirectory(t *testing.T) {
	dir := t.TempDir()
	store, err := library.NewStore(filepath.Join(dir, "library.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	tabsDir := filepath.Join(dir, "tabs")
	if err := os.MkdirAll(tabsDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	files := []string{"a.txt", "b.txt", "c.md"}
	for i, name := range files {
		path := filepath.Join(tabsDir, name)
		content := helpers.SultansTab
		if i == 2 {
			content = "not a tab\n"
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	if err := store.ImportDirectory(tabsDir); err != nil {
		t.Fatalf("ImportDirectory: %v", err)
	}

	list, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 imported .txt tabs, got %d", len(list))
	}
}

func TestImportDirectorySkipsUnparseableFiles(t *testing.T) {
	dir := t.TempDir()
	store, err := library.NewStore(filepath.Join(dir, "library.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	tabsDir := filepath.Join(dir, "tabs")
	if err := os.MkdirAll(tabsDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	good1 := filepath.Join(tabsDir, "good1.txt")
	good2 := filepath.Join(tabsDir, "good2.txt")
	for _, path := range []string{good1, good2} {
		if err := os.WriteFile(path, []byte(helpers.SultansTab), 0644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	// bad.txt is unparseable: a single line longer than the parser's 8MB token
	// limit makes ParsePath fail.
	bad := filepath.Join(tabsDir, "bad.txt")
	if err := os.WriteFile(bad, []byte(strings.Repeat("x", 9*1024*1024)+"\n"), 0644); err != nil {
		t.Fatalf("write %s: %v", bad, err)
	}

	if err := store.ImportDirectory(tabsDir); err != nil {
		t.Fatalf("ImportDirectory: %v", err)
	}

	list, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 imported tabs, bad.txt skipped, got %d", len(list))
	}
	got := map[string]bool{}
	for _, row := range list {
		got[row.Filepath] = true
	}
	for _, path := range []string{good1, good2} {
		if !got[path] {
			t.Fatalf("expected %s to be imported, got %v", path, got)
		}
	}
	if got[bad] {
		t.Fatalf("expected %s to be skipped, got %v", bad, got)
	}
}
