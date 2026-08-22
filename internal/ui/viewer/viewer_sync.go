package viewer

import (
	"fmt"
	"math"
	"sort"
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
	if m.chordSheet {
		// S1.2: a chord sheet renders its raw text verbatim (transposed by
		// the T/Z keys) instead of the bar grid; its bars are empty.
		m.viewport.SetContent(m.chordText())
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

// refineSyncPoints drops outlier anchors with a robust median/MAD fit on the
// adjacent segment tempi. Each anchor is judged by the tempo evidence of the
// segment leading into it (the first anchor by the segment leading out): its
// deviation is how far that segment's BPM sits from the median segment BPM.
// Anchors deviating more than 2*MAD are removed and the fit is recomputed on
// the survivors, iterating until stable. Fewer than 4 anchors are left
// unchanged (too few for robust statistics). Segments whose BPM cannot be
// derived (SegmentBPM returns 0 for zero spans, repeated bars, or a bar past
// the schedule end) carry no information and never count as outliers.
func refineSyncPoints(schedule []player.PlaybackStep, points []player.SyncPoint) []player.SyncPoint {
	cur := points
	for len(cur) >= 4 {
		pts := syncPointsZeroBased(cur)
		segBPM := make([]int, len(pts)-1)
		usable := 0
		for i := 0; i < len(pts)-1; i++ {
			segBPM[i] = player.SegmentBPM(schedule, pts[i], pts[i+1])
			if segBPM[i] > 0 {
				usable++
			}
		}
		if usable < 2 {
			return cur // too few derivable segments for a robust median
		}
		median := medianBPM(segBPM)
		dev := make([]float64, len(cur))
		dev[0] = segDev(segBPM[0], median)
		for i := 1; i < len(cur); i++ {
			dev[i] = segDev(segBPM[i-1], median)
		}
		mad := madDeviation(dev)
		if mad < 0 {
			return cur
		}
		limit := 2 * mad
		kept := make([]player.SyncPoint, 0, len(cur))
		dropped := false
		for i, p := range cur {
			if dev[i] < 0 || dev[i] <= limit {
				kept = append(kept, p)
			} else {
				dropped = true
			}
		}
		if !dropped {
			return cur // no outliers: the fit is stable
		}
		cur = kept
	}
	return cur
}

// medianBPM returns the median of the derivable segment tempi.
func medianBPM(segBPM []int) int {
	var vals []int
	for _, b := range segBPM {
		if b > 0 {
			vals = append(vals, b)
		}
	}
	if len(vals) == 0 {
		return 0
	}
	sort.Ints(vals)
	return vals[len(vals)/2]
}

// segDev is the absolute distance of a segment tempo from the median, or -1
// when the segment BPM is 0 (no information).
func segDev(bpm, median int) float64 {
	if bpm <= 0 {
		return -1
	}
	return math.Abs(float64(bpm) - float64(median))
}

// madDeviation is the median absolute deviation of the known per-anchor
// deviations, or -1 when none exist.
func madDeviation(dev []float64) float64 {
	var vals []float64
	for _, d := range dev {
		if d >= 0 {
			vals = append(vals, d)
		}
	}
	if len(vals) == 0 {
		return -1
	}
	sort.Float64s(vals)
	return vals[len(vals)/2]
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
