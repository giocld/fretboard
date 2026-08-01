package library

// TabRow is a lightweight summary of a stored tab.
type TabRow struct {
	ID         int64
	Filepath   string
	Title      string
	Artist     string
	Tuning     string
	Favorite   bool
	PlayCount  int64
	LastPlayed string // SQLite datetime from last_played, empty if never opened
}

// MoreRecentlyUsed reports whether a should sort before b in recent-tab lists.
func MoreRecentlyUsed(a, b TabRow) bool {
	if a.LastPlayed != b.LastPlayed {
		if a.LastPlayed == "" {
			return false
		}
		if b.LastPlayed == "" {
			return true
		}
		return a.LastPlayed > b.LastPlayed
	}
	if a.PlayCount != b.PlayCount {
		return a.PlayCount > b.PlayCount
	}
	return a.ID > b.ID
}
