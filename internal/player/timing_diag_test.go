package player_test

import (
	"os"
	"path/filepath"
	"testing"

	"fretboard/internal/library"
	"fretboard/internal/player"
)

func TestLibraryEventTiming(t *testing.T) {
	cfg, _ := os.UserConfigDir()
	db := filepath.Join(cfg, "fretboard", "fretboard.db")
	store, err := library.NewStore(db)
	if err != nil {
		t.Skipf("no library db: %v", err)
	}
	defer store.Close()
	rows, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Skip("library db has no tabs")
	}
	tab, err := store.Get(rows[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	evts, _ := player.Events(tab, 120)
	for i, e := range evts {
		if i > 12 {
			break
		}
		t.Logf("%d %v tick=%d note=%d", i, e.Type, e.Tick, e.Note)
	}
	steps := player.BuildSchedule(tab)
	for i, s := range steps {
		if i > 8 {
			break
		}
		t.Logf("step %d bar=%d col=%d ticks=%d", i, s.Bar, s.Col, s.Ticks)
	}
}
