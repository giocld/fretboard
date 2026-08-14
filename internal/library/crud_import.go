package library

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"fretboard/internal/parser"
)

// ImportFile reads a tab file from disk and imports it.
func (s *Store) ImportFile(filepath string) (int64, error) {
	tab, err := parser.ParsePath(filepath)
	if err != nil {
		return 0, fmt.Errorf("parse file: %w", err)
	}
	return s.Import(filepath, tab)
}

// ImportDirectory walks a directory recursively and imports .txt tabs and
// Guitar Pro files (.gp3–.gpx). Files that fail to parse are skipped so the
// rest of the directory still gets imported; an error is only returned when
// the walk itself fails or no file could be imported.
func (s *Store) ImportDirectory(dir string) error {
	var skipped []error
	imported := 0
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(path), ".txt") && !parser.IsGpFile(path) {
			return nil
		}
		if _, err := s.ImportFile(path); err != nil {
			skipped = append(skipped, fmt.Errorf("import %s: %w", path, err))
			return nil
		}
		imported++
		return nil
	})
	if err != nil {
		return err
	}
	if imported == 0 && len(skipped) > 0 {
		return fmt.Errorf("import directory %s: no tabs imported: %v", dir, skipped)
	}
	return nil
}
