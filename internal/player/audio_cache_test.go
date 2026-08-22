package player

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"fretboard/internal/config"
	"fretboard/internal/model"
	"fretboard/internal/testutil"
)

// cacheDirForTest redirects the user config dir to a temp dir and returns
// the audio cache directory inside it.
func cacheDirForTest(t *testing.T) string {
	t.Helper()
	testutil.RedirectConfigDir(t)
	dir, err := config.AudioDir()
	if err != nil {
		t.Fatalf("AudioDir: %v", err)
	}
	return dir
}

func writeCacheFile(t *testing.T, dir, name string, size int64) string {
	t.Helper()
	path := filepath.Join(dir, name)
	data := make([]byte, size)
	for i := range data {
		data[i] = byte('x')
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func setMtime(t *testing.T, path string, when time.Time) {
	t.Helper()
	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatalf("chtimes %s: %v", path, err)
	}
}

func TestCacheStatsCountsAudioFilesAndSizes(t *testing.T) {
	dir := cacheDirForTest(t)
	writeCacheFile(t, dir, "song a.mp3", 100)
	writeCacheFile(t, dir, "song b.flac", 200)
	// Non-audio entries must not count as cache entries.
	writeCacheFile(t, dir, "song c.mp3.part", 999)
	if err := os.MkdirAll(filepath.Join(dir, "subdir"), 0755); err != nil {
		t.Fatal(err)
	}

	entries, total, gotDir := CacheStats()
	if entries != 2 {
		t.Errorf("entries = %d, want 2", entries)
	}
	if total != 300 {
		t.Errorf("totalBytes = %d, want 300", total)
	}
	if gotDir != dir {
		t.Errorf("dir = %q, want %q", gotDir, dir)
	}
}

func TestCacheStatsEmptyDir(t *testing.T) {
	dir := cacheDirForTest(t)
	entries, total, gotDir := CacheStats()
	if entries != 0 || total != 0 {
		t.Errorf("empty cache = %d entries, %d bytes; want 0, 0", entries, total)
	}
	if gotDir != dir {
		t.Errorf("dir = %q, want %q", gotDir, dir)
	}
}

func TestEvictLRURemovesOldestFirst(t *testing.T) {
	dir := cacheDirForTest(t)
	base := time.Now().Add(-10 * time.Hour)
	old := writeCacheFile(t, dir, "oldest.mp3", 40)
	middle := writeCacheFile(t, dir, "middle.flac", 50)
	newest := writeCacheFile(t, dir, "newest.ogg", 60)
	setMtime(t, old, base)
	setMtime(t, middle, base.Add(1*time.Hour))
	setMtime(t, newest, base.Add(2*time.Hour))

	// Keep 110 of the 150 bytes: only the oldest 40-byte file must go.
	freed, err := EvictLRU(110)
	if err != nil {
		t.Fatalf("EvictLRU: %v", err)
	}
	if freed != 40 {
		t.Errorf("freed = %d, want 40", freed)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("oldest file still exists, stat err = %v", err)
	}
	if _, err := os.Stat(middle); err != nil {
		t.Errorf("middle file removed, want kept: %v", err)
	}
	if _, err := os.Stat(newest); err != nil {
		t.Errorf("newest file removed, want kept: %v", err)
	}
}

func TestEvictLRURespectsKeepBytesBoundary(t *testing.T) {
	dir := cacheDirForTest(t)
	base := time.Now().Add(-5 * time.Hour)
	a := writeCacheFile(t, dir, "a.mp3", 10)
	b := writeCacheFile(t, dir, "b.mp3", 20)
	setMtime(t, a, base)
	setMtime(t, b, base.Add(1*time.Hour))

	// keepBytes exactly equal to the total: nothing is evicted.
	freed, err := EvictLRU(30)
	if err != nil {
		t.Fatalf("EvictLRU: %v", err)
	}
	if freed != 0 {
		t.Errorf("freed = %d, want 0 (cache already at cap)", freed)
	}
	// keepBytes 0: everything goes, oldest first.
	freed, err = EvictLRU(0)
	if err != nil {
		t.Fatalf("EvictLRU: %v", err)
	}
	if freed != 30 {
		t.Errorf("freed = %d, want 30", freed)
	}
	if _, err := os.Stat(a); !os.IsNotExist(err) {
		t.Errorf("a.mp3 still exists after EvictLRU(0)")
	}
	if _, err := os.Stat(b); !os.IsNotExist(err) {
		t.Errorf("b.mp3 still exists after EvictLRU(0)")
	}
}

func TestEnforceCacheCapEvictsOverCap(t *testing.T) {
	dir := cacheDirForTest(t)
	base := time.Now().Add(-3 * time.Hour)
	writeCacheFile(t, dir, "keep.flac", 10)
	old := writeCacheFile(t, dir, "old.ogg", 20)
	setMtime(t, old, base)

	oldCap := AudioCacheMaxGB
	AudioCacheMaxGB = 0 // cap of 0 bytes: everything must be evicted
	defer func() { AudioCacheMaxGB = oldCap }()

	if err := EnforceCacheCap(); err != nil {
		t.Fatalf("EnforceCacheCap: %v", err)
	}
	_, total, _ := CacheStats()
	if total != 0 {
		t.Errorf("total = %d after EnforceCacheCap, want 0", total)
	}
}

func TestEnforceCacheCapNoopUnderCap(t *testing.T) {
	cacheDirForTest(t)
	// Tiny cache (1 byte) vs default 5 GiB cap: no eviction, no error.
	oldCap := AudioCacheMaxGB
	AudioCacheMaxGB = 5
	defer func() { AudioCacheMaxGB = oldCap }()
	if err := EnforceCacheCap(); err != nil {
		t.Fatalf("EnforceCacheCap under cap: %v", err)
	}
}

func TestTouchCacheEntryBumpsMtime(t *testing.T) {
	dir := cacheDirForTest(t)
	path := writeCacheFile(t, dir, "touched.mp3", 10)
	setMtime(t, path, time.Now().Add(-48*time.Hour))
	if err := TouchCacheEntry(path); err != nil {
		t.Fatalf("TouchCacheEntry: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if age := time.Since(info.ModTime()); age > time.Minute {
		t.Errorf("mtime not bumped: file %v old", age)
	}
	if err := TouchCacheEntry(""); err != nil {
		t.Errorf("TouchCacheEntry(\"\") = %v, want nil", err)
	}
}

func TestParseProgressLine(t *testing.T) {
	cases := []struct {
		name    string
		line    string
		percent float64
		ok      bool
	}{
		{"typical", "[download]  45.3% of 4.21MiB at 1.23MiB/s ETA 00:02", 45.3, true},
		{"integral percent", "[download] 100% of 4.21MiB in 00:03", 100, true},
		{"zero start", "[download]   0.0% of ~ 4.21MiB at Unknown B/s ETA Unknown", 0, true},
		{"single digit", "[download]  7% of 1.00MiB", 7, true},
		{"no leading spaces", "[download] 45.3% of 4.21MiB at 1.23MiB/s ETA 00:02", 45.3, true},
		{"KiB size", "[download]  50.0% of 999.99KiB at 123.4KiB/s ETA 00:05", 50, true},
		{"destination line", "[download] Destination: song [abc123].mp3", 0, false},
		{"ytdlp info line", "[youtube] abc123: Downloading webpage", 0, false},
		{"extract audio line", "[ExtractAudio] Destination: song.mp3", 0, false},
		{"empty", "", 0, false},
		{"percent without download tag", "45.3% of 4.21MiB", 0, false},
		{"garbage", "hello world", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, ok := ParseProgressLine(tc.line)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if ok && p != tc.percent {
				t.Errorf("percent = %v, want %v", p, tc.percent)
			}
		})
	}
}

func TestNotifyDownloadProgressInvokesHook(t *testing.T) {
	old := OnDownloadProgress
	defer func() { OnDownloadProgress = old }()

	var got []float64
	OnDownloadProgress = func(p float64) { got = append(got, p) }

	NotifyDownloadProgress("[download]  10.0% of 1MiB at 1MiB/s ETA 00:01")
	NotifyDownloadProgress("[download] Destination: song.mp3")
	NotifyDownloadProgress("[download]  20.5% of 2MiB at 1MiB/s ETA 00:01")
	NotifyDownloadProgress("")

	want := []float64{10, 20.5}
	if len(got) != len(want) {
		t.Fatalf("hook got %d calls, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("call %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestNotifyDownloadProgressNilHook(t *testing.T) {
	old := OnDownloadProgress
	OnDownloadProgress = nil
	defer func() { OnDownloadProgress = old }()

	// Must not panic.
	NotifyDownloadProgress("[download]  50% of 1MiB")
	NotifyDownloadProgress("not a progress line")
}

func TestFindAudioTouchesCachedFile(t *testing.T) {
	dir := cacheDirForTest(t)
	path := writeCacheFile(t, dir, "Fancy Song.mp3", 10)
	setMtime(t, path, time.Now().Add(-72*time.Hour))

	tab := &model.Tab{Title: "Fancy Song"}
	if got := FindAudio(tab, "", nil); got != path {
		t.Fatalf("FindAudio = %q, want %q", got, path)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if age := time.Since(info.ModTime()); age > time.Minute {
		t.Errorf("cache hit mtime not bumped: %v old", age)
	}
}
