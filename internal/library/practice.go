package library

import (
	"fmt"
	"time"
)

// TabStat aggregates practice time for a single tab.
type TabStat struct {
	TabID   int64
	Title   string
	Artist  string
	Minutes int // whole minutes of practice in the window
}

// RecordPractice logs one practice session for a tab. started_at is stamped
// now; duration is in seconds; tempo_bpm and loops may be zero when unknown.
func (s *Store) RecordPractice(tabID int64, duration int64, tempo int, loops int) error {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM tabs WHERE id = ?`, tabID).Scan(&n); err != nil {
		return fmt.Errorf("record practice: check tab: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("library: tab %d: %w", tabID, ErrNotFound)
	}
	if _, err := s.db.Exec(`
		INSERT INTO practice_events (tab_id, started_at, duration_seconds, tempo_bpm, loops)
		VALUES (?, ?, ?, ?, ?)
	`, tabID, time.Now().Unix(), duration, tempo, loops); err != nil {
		return fmt.Errorf("record practice: %w", err)
	}
	return nil
}

// PracticeStats summarizes practice within the last days days (the window is
// started_at >= now - days*24h). totalMinutes is the sum of whole minutes
// across all tabs; byTab lists tabs by descending minutes.
func (s *Store) PracticeStats(days int) (totalMinutes int, byTab []TabStat, err error) {
	if days < 0 {
		days = 0
	}
	cutoff := time.Now().Unix() - int64(days)*86400

	rows, err := s.db.Query(`
		SELECT pe.tab_id, t.title, t.artist, SUM(pe.duration_seconds) AS total_secs
		FROM practice_events pe
		JOIN tabs t ON t.id = pe.tab_id
		WHERE pe.started_at >= ?
		GROUP BY pe.tab_id, t.title, t.artist
		ORDER BY total_secs DESC, pe.tab_id
	`, cutoff)
	if err != nil {
		return 0, nil, fmt.Errorf("practice stats: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var st TabStat
		var secs int64
		if err := rows.Scan(&st.TabID, &st.Title, &st.Artist, &secs); err != nil {
			return 0, nil, fmt.Errorf("practice stats: scan: %w", err)
		}
		st.Minutes = int(secs / 60)
		totalMinutes += st.Minutes
		byTab = append(byTab, st)
	}
	if err := rows.Err(); err != nil {
		return 0, nil, fmt.Errorf("practice stats: rows: %w", err)
	}
	return totalMinutes, byTab, nil
}
