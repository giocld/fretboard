package viewer

import (
	"fmt"
	"math"
	"time"

	"fretboard/internal/model"
	"fretboard/internal/player"
	"fretboard/internal/ui/kit"
)

func (m *ViewerModel) refresh() {
	if m.tab == nil {
		m.viewport.SetContent(kit.MutedStyle.Render("No tab loaded."))
		return
	}
	cur := &kit.TabCursor{Bar: m.cursorBar, Col: m.cursorCol, Playing: m.playing, ShowNotes: m.showNotes, SearchBar: -1, SearchCol: -1}
	if m.searchIdx >= 0 && m.searchIdx < len(m.searchMatches) {
		cur.SearchBar = m.searchMatches[m.searchIdx].bar
		cur.SearchCol = m.searchMatches[m.searchIdx].col
	}
	if m.loopStartBar > 0 && m.loopEndBar > m.loopStartBar {
		cur.LoopStartBar = m.loopStartBar - 1
		cur.LoopEndBar = m.loopEndBar
	}
	display := m.displayTab()
	if m.perfMode {
		m.viewport.SetContent(m.renderPerformance())
	} else if m.linear {
		m.viewport.SetContent(kit.RenderTabLinearBody(display, m.panOffset, cur))
	} else {
		m.viewport.SetContent(kit.RenderTabGridBody(display, m.viewport.Width, m.panOffset, cur))
	}
	if m.follow {
		m.ensureCursorVisible()
	}
}

func (m ViewerModel) displayTab() *model.Tab {
	if m.transpose == 0 {
		return m.tab
	}
	return model.TransposedTab(m.tab, m.transpose)
}

// syncPointsZeroBased converts persisted sync points (user-facing 1-based
// bars) to the schedule's 0-based bar indices for time mapping.
func syncPointsZeroBased(points []player.SyncPoint) []player.SyncPoint {
	out := make([]player.SyncPoint, 0, len(points))
	for _, p := range points {
		if p.Bar > 0 {
			p.Bar--
		}
		out = append(out, p)
	}
	return out
}

// tempoMap returns the low->high BPM range spanned by the per-segment tempi
// derived from the sync anchors, when at least two anchors exist and the
// tempo actually varies between them.
func (m ViewerModel) tempoMap() ([2]int, bool) {
	if len(m.syncPoints) < 2 || len(m.schedule) == 0 {
		return [2]int{}, false
	}
	points := syncPointsZeroBased(m.syncPoints)
	low, high := 0, 0
	for i := 0; i+1 < len(points); i++ {
		b := player.SegmentBPM(m.schedule, points[i], points[i+1])
		if b <= 0 {
			return [2]int{}, false
		}
		if low == 0 || b < low {
			low = b
		}
		high = max(high, b)
	}
	if low <= 0 || high <= low {
		return [2]int{}, false
	}
	return [2]int{low, high}, true
}

// syncQuality returns the RMS drift (seconds) between the sync anchors and
// the tempo implied by the first segment, when at least two anchors exist.
// Each later anchor is predicted from the first segment's tempo; the error
// between prediction and reality is the drift the anchor mapping corrects.
func (m ViewerModel) syncQuality() (float64, bool) {
	if len(m.syncPoints) < 2 || len(m.schedule) == 0 {
		return 0, false
	}
	points := syncPointsZeroBased(m.syncPoints)
	baseBPM := player.SegmentBPM(m.schedule, points[0], points[1])
	if baseBPM <= 0 {
		return 0, false
	}
	var sum float64
	n := 0
	for i := 2; i < len(points); i++ {
		ticks := player.TicksBetweenBars(m.schedule, points[0].Bar, points[i].Bar)
		predicted := points[0].Seconds + player.TicksToSeconds(ticks, baseBPM)
		err := predicted - points[i].Seconds
		sum += err * err
		n++
	}
	if n == 0 {
		return 0, false
	}
	return math.Sqrt(sum / float64(n)), true
}

func (m *ViewerModel) ensureCursorVisible() {
	if m.tab == nil {
		return
	}
	target := m.cursorBarLineOffset()
	if target < m.viewport.YOffset {
		m.viewport.SetYOffset(target)
	} else if target >= m.viewport.YOffset+m.viewport.Height {
		m.viewport.SetYOffset(target - m.viewport.Height + 1)
	}
}

// trackEndedBanner explains an early audio-file end (radio edits, live
// cuts) and the restart path. "before the tab finished" is the key
// distinction from a normal end-of-tab stop.
func trackEndedBanner(dur time.Duration) string {
	if dur > 0 {
		m := int(dur / time.Minute)
		s := int(dur/time.Second) % 60
		return fmt.Sprintf("Track ended (%d:%02d) before the tab finished — Space restarts from this bar", m, s)
	}
	return "Track ended before the tab finished — Space restarts from this bar"
}
