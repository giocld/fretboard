package library

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"fretboard/internal/model"
)

// TestPracticeStats pins 6.3: RecordPractice logs sessions with a "now"
// timestamp, PracticeStats aggregates whole minutes inside the window, orders
// tabs by descending time, and ignores events older than the window.
func TestPracticeStats(t *testing.T) {
	st, err := NewStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	a, err := st.Import("a.txt", &model.Tab{Title: "Alpha", Artist: "A"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := st.Import("b.txt", &model.Tab{Title: "Beta", Artist: "B"})
	if err != nil {
		t.Fatal(err)
	}

	// Two sessions for Alpha: 90s + 90s = 3 whole minutes.
	for i := 0; i < 2; i++ {
		if err := st.RecordPractice(a, 90, 120, 3); err != nil {
			t.Fatal(err)
		}
	}
	// One hour for Beta.
	if err := st.RecordPractice(b, 3600, 90, 1); err != nil {
		t.Fatal(err)
	}
	// A session far outside the window must not count (31 days: safely beyond
	// the 30-day window even with clock skew within the same second).
	old := time.Now().Unix() - 31*86400
	if _, err := st.db.Exec(`
		INSERT INTO practice_events (tab_id, started_at, duration_seconds, tempo_bpm, loops)
		VALUES (?, ?, ?, ?, ?)
	`, a, old, 999999, 0, 0); err != nil {
		t.Fatal(err)
	}

	total, byTab, err := st.PracticeStats(7)
	if err != nil {
		t.Fatal(err)
	}
	if total != 63 { // 3 + 60
		t.Fatalf("total minutes = %d, want 63", total)
	}
	if len(byTab) != 2 {
		t.Fatalf("byTab = %+v, want 2 tabs", byTab)
	}
	if byTab[0].TabID != b || byTab[0].Minutes != 60 || byTab[0].Title != "Beta" {
		t.Fatalf("top tab = %+v, want Beta/60", byTab[0])
	}
	if byTab[1].TabID != a || byTab[1].Minutes != 3 {
		t.Fatalf("second tab = %+v, want Alpha/3", byTab[1])
	}

	// Wider window still excludes the 30-day-old event only if beyond it.
	total30, _, err := st.PracticeStats(30)
	if err != nil {
		t.Fatal(err)
	}
	if total30 != 63 {
		t.Fatalf("30-day total = %d, want 63 (old event at 30d excluded)", total30)
	}

	// Missing tab errors.
	if err := st.RecordPractice(999, 1, 1, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("RecordPractice(999) err = %v, want ErrNotFound", err)
	}

	// A very wide window includes everything (all sessions are within it).
	totalAll, _, err := st.PracticeStats(4000)
	if err != nil {
		t.Fatal(err)
	}
	if totalAll != 63+16666 { // 999999s/60 = 16666 whole minutes from the old event
		t.Fatalf("wide-window total = %d, want %d", totalAll, 63+16666)
	}
}
