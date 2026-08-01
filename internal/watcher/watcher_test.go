package watcher

import "testing"

func TestIsTabImportPath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"song.txt", true},
		{"song.gp5", true},
		{"song.gpx", true},
		{"song.mp3", false},
		{"README", false},
	}
	for _, tc := range cases {
		if got := isTabImportPath(tc.path); got != tc.want {
			t.Fatalf("isTabImportPath(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestIsWatchedPath(t *testing.T) {
	dir := "/watch"
	if !IsWatchedPath(dir, "/watch/new.txt") {
		t.Fatal("expected txt under watch dir")
	}
	if IsWatchedPath(dir, "/other/new.txt") {
		t.Fatal("expected path outside watch dir to be ignored")
	}
}
