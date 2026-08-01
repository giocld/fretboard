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
	store, _ := library.NewStore(db)
	defer store.Close()
	rows, _ := store.List()
	tab, _ := store.Get(rows[0].ID)
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
