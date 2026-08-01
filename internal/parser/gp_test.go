package parser

import "testing"

func TestIsGpFile(t *testing.T) {
	cases := map[string]bool{
		"song.gp5": true,
		"song.GPX": true,
		"song.gp3": true,
		"song.gp4": true,
		"song.gp":  true,
		"song.txt": false,
		"song.md":  false,
		"noext":    false,
	}
	for path, want := range cases {
		if got := IsGpFile(path); got != want {
			t.Errorf("IsGpFile(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestDecodeGPTabJSON(t *testing.T) {
	const raw = `{
		"title": "Test Song",
		"artist": "Test Artist",
		"tuning": [40, 45, 50, 55, 59, 64],
		"bars": [{
			"number": 1,
			"column_ticks": [480, 240],
			"strings": [{
				"segments": [
					{"char": "0", "value": 0, "position": 0, "width": 1},
					{"char": "3", "value": 3, "position": 4, "width": 1}
				]
			}]
		}],
		"metadata": {"source": "guitar-pro"}
	}`

	tab, err := decodeGPTabJSON([]byte(raw))
	if err != nil {
		t.Fatalf("decodeGPTabJSON: %v", err)
	}
	if tab.Title != "Test Song" || tab.Artist != "Test Artist" {
		t.Fatalf("unexpected metadata: %+v", tab)
	}
	if len(tab.Bars) != 1 {
		t.Fatalf("expected 1 bar, got %d", len(tab.Bars))
	}
	bar := tab.Bars[0]
	if len(bar.ColumnTicks) != 2 || bar.ColumnTicks[0] != 480 {
		t.Fatalf("unexpected column ticks: %v", bar.ColumnTicks)
	}
	if len(bar.Strings) != 1 || len(bar.Strings[0].Segments) != 2 {
		t.Fatalf("unexpected segments: %+v", bar.Strings)
	}
	if tab.Metadata["source"] != "guitar-pro" {
		t.Fatalf("metadata: %v", tab.Metadata)
	}
}
