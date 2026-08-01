package player_test

import (
	"os"
	"path/filepath"
	"testing"

	"fretboard/internal/library"
	"fretboard/internal/player"
)

func TestLibraryTabMIDSize(t *testing.T) {
	cfg, err := os.UserConfigDir()
	if err != nil {
		t.Skip(err)
	}
	db := filepath.Join(cfg, "fretboard", "fretboard.db")
	store, err := library.NewStore(db)
	if err != nil {
		t.Skip(err)
	}
	defer store.Close()
	rows, _ := store.List()
	if len(rows) == 0 {
		t.Skip("no tabs")
	}
	tab, err := store.Get(rows[len(rows)-1].ID) // Layla likely last
	if err != nil {
		t.Fatal(err)
	}
	evts, err := player.Events(tab, 120)
	if err != nil {
		t.Fatal(err)
	}
	data, err := player.WriteSMF(evts, 120)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("tab=%s bars=%d events=%d midBytes=%d", tab.Title, len(tab.Bars), len(evts), len(data))
	if len(evts) == 0 {
		t.Fatal("no events")
	}
	os.WriteFile("/tmp/fb_layla.mid", data, 0644)
}
