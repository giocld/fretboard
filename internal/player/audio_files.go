package player

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"fretboard/internal/config"
	"fretboard/internal/model"
)

var audioExtensions = []string{".mp3", ".flac", ".ogg", ".wav", ".m4a", ".opus", ".aac"}

// FindAudio locates a backing-track file for tab.
func FindAudio(tab *model.Tab, tabPath string, extraDirs []string) string {
	if tab == nil {
		return ""
	}

	var dirs []string
	if tabPath != "" && !strings.HasPrefix(tabPath, "online://") {
		if dir := filepath.Dir(tabPath); dir != "" && dir != "." {
			dirs = append(dirs, dir)
		}
		base := strings.TrimSuffix(filepath.Base(tabPath), filepath.Ext(tabPath))
		if path := findAudioByBasename(dirs, base); path != "" {
			return path
		}
	}

	for _, d := range extraDirs {
		if d = strings.TrimSpace(d); d != "" {
			dirs = append(dirs, expandHome(d))
		}
	}
	if cfgDir, err := config.AudioDir(); err == nil {
		dirs = append(dirs, cfgDir)
	}

	names := audioNameCandidates(tab)
	for _, dir := range uniqueDirs(dirs) {
		if path := findAudioByNames(dir, names); path != "" {
			return path
		}
	}
	return ""
}

func audioNameCandidates(tab *model.Tab) []string {
	title := strings.TrimSpace(tab.Title)
	artist := strings.TrimSpace(tab.Artist)
	var names []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		for _, n := range names {
			if strings.EqualFold(n, s) {
				return
			}
		}
		names = append(names, s)
	}
	add(title)
	if artist != "" && title != "" {
		add(fmt.Sprintf("%s - %s", artist, title))
		add(fmt.Sprintf("%s_%s", artist, title))
	}
	return names
}

func findAudioByBasename(dirs []string, base string) string {
	for _, dir := range dirs {
		if path := findAudioByNames(dir, []string{base}); path != "" {
			return path
		}
	}
	return ""
}

func findAudioByNames(dir string, names []string) string {
	if dir == "" {
		return ""
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return ""
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	files := audioFileNames(entries)
	// Pass 1: exact normalized match.
	for _, name := range names {
		want := normalizeAudioName(name)
		for _, fname := range files {
			stem := normalizeAudioName(strings.TrimSuffix(fname, filepath.Ext(fname)))
			if stem == want {
				return filepath.Join(dir, fname)
			}
		}
	}
	// Pass 2 (relaxed): a file whose normalized name *contains* a candidate
	// — "Sultans of Swing (Live 1984).mp3" pairs with the tab even though
	// the extra words break exact matching. The closest candidate (shortest
	// stem) wins so "Sultans of Swing" beats "Sultans of Swing 2".
	best := ""
	bestLen := 0
	for _, name := range names {
		want := normalizeAudioName(name)
		if len(want) < 4 {
			continue
		}
		for _, fname := range files {
			stem := normalizeAudioName(strings.TrimSuffix(fname, filepath.Ext(fname)))
			if strings.Contains(stem, want) && (best == "" || len(stem) < bestLen) {
				best = filepath.Join(dir, fname)
				bestLen = len(stem)
			}
		}
	}
	return best
}

// audioFileNames returns the non-directory entries whose extension is a
// recognized audio extension, in directory order.
func audioFileNames(entries []os.DirEntry) []string {
	var out []string
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		if !isAudioExt(strings.ToLower(filepath.Ext(ent.Name()))) {
			continue
		}
		out = append(out, ent.Name())
	}
	return out
}

func normalizeAudioName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func isAudioExt(ext string) bool {
	return slices.Contains(audioExtensions, ext)
}

func uniqueDirs(dirs []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, d := range dirs {
		d = filepath.Clean(expandHome(d))
		if d == "." {
			continue
		}
		if _, ok := seen[d]; ok {
			continue
		}
		seen[d] = struct{}{}
		out = append(out, d)
	}
	return out
}
