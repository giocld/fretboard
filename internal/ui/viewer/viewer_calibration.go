package viewer

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"fretboard/internal/model"
	"fretboard/internal/player"
	tea "github.com/charmbracelet/bubbletea"
)

// currentSourceID returns the ID of the selected audio source, or "" when no
// catalog is loaded. Calibration is stored per source so switching from a
// studio version (short intro) to a live version (long intro) restores the
// right offset instead of sharing one misaligned value.
func (m ViewerModel) currentSourceID() string {
	src := m.selectedSource()
	if src.Kind == player.SourceMIDI {
		return ""
	}
	return src.ID
}

// audioOffsetKey returns the metadata key holding the calibrated intro
// offset for the current source, falling back to the legacy global key.
func (m ViewerModel) audioOffsetKey() string {
	if id := m.currentSourceID(); id != "" {
		return model.MetaKeyAudioOffset + ":" + id
	}
	return model.MetaKeyAudioOffset
}

// syncPointsKey returns the metadata key holding sync anchors for the current
// source, falling back to the legacy global key.
func (m ViewerModel) syncPointsKey() string {
	if id := m.currentSourceID(); id != "" {
		return model.MetaKeySyncPoints + ":" + id
	}
	return model.MetaKeySyncPoints
}

// restoreCalibrationForSource loads the intro offset and sync anchors for the
// currently selected source (falling back to the legacy per-tab values), so a
// source switch never carries another recording's calibration.
func (m *ViewerModel) restoreCalibrationForSource() {
	m.audioOffset = 0
	m.syncPoints = nil
	if m.tab == nil || m.tab.Metadata == nil {
		return
	}
	offset := ""
	for _, key := range []string{m.audioOffsetKey(), model.MetaKeyAudioOffset} {
		if offset = strings.TrimSpace(m.tab.Metadata[key]); offset != "" {
			break
		}
	}
	if offset != "" {
		if v, err := strconv.ParseFloat(offset, 64); err == nil {
			m.audioOffset = v
		}
	}
	for _, key := range []string{m.syncPointsKey(), model.MetaKeySyncPoints} {
		if points := parseSyncPoints(m.tab.Metadata[key]); len(points) > 0 {
			m.syncPoints = points
			break
		}
	}
	// Restore the persisted auto tempo map for this source (measured bar
	// anchors + onsets + strengths), so a later session keeps the alignment
	// without re-running the analysis.
	m.autoAnchors, m.autoOnsets, m.autoStrengths = nil, nil, nil
	m.autoActive = false
	if id := m.currentSourceID(); id != "" {
		if anchors, onsets, strengths := player.UnmarshalTempoMap(m.tab.Metadata["tempo_map:"+id]); len(anchors) >= 2 {
			m.autoAnchors, m.autoOnsets, m.autoStrengths, m.autoActive = anchors, onsets, strengths, true
		}
	}
}

// saveCalibrationForSource persists the current offset and anchors under the
// current source's keys, mirroring the legacy keys so tabs without source
// metadata keep working (and older builds stay compatible).
func (m *ViewerModel) saveCalibrationForSource() {
	if m.tab == nil {
		return
	}
	if m.tab.Metadata == nil {
		m.tab.Metadata = map[string]string{}
	}
	offset := strconv.FormatFloat(m.audioOffset, 'f', 1, 64)
	m.tab.Metadata[m.audioOffsetKey()] = offset
	m.tab.Metadata[model.MetaKeyAudioOffset] = offset
	m.saveSyncPoints()
}

// adjustAudioOffset nudges the audio start offset used to sync the tab cursor
// with a real recording, and persists it under the current source's key.
func (m ViewerModel) adjustAudioOffset(key string) (ViewerModel, tea.Cmd) {
	switch key {
	case "[":
		m.audioOffset -= 0.5
	case ",":
		m.audioOffset -= 0.1
	case "{":
		m.audioOffset -= 5
	case "]":
		m.audioOffset += 0.5
	case ".":
		m.audioOffset += 0.1
	case "}":
		m.audioOffset += 5
	case "o":
		// Reset, with undo: pressing o again restores the previous offset
		// (a fat-fingered reset must not be irreversible).
		if m.audioOffset != 0 {
			m.prevOffset = m.audioOffset
			m.audioOffset = 0
		} else if m.prevOffset != 0 {
			m.audioOffset, m.prevOffset = m.prevOffset, 0
		}
	}
	m.audioOffset = min(max(m.audioOffset, -60), 300)
	m.refresh()
	if m.tab == nil {
		return m, nil
	}
	if m.tab.Metadata == nil {
		m.tab.Metadata = map[string]string{}
	}
	offset := strconv.FormatFloat(m.audioOffset, 'f', 1, 64)
	m.tab.Metadata[m.audioOffsetKey()] = offset
	m.tab.Metadata[model.MetaKeyAudioOffset] = offset
	return m, m.saveTabPrefsCmd()
}

// parseSyncPoints decodes the persisted [[bar, seconds], ...] metadata.
func parseSyncPoints(raw string) []player.SyncPoint {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var points []player.SyncPoint
	if err := json.Unmarshal([]byte(raw), &points); err != nil {
		return nil
	}
	seen := map[int]bool{}
	clean := points[:0]
	for _, p := range points {
		if p.Bar < 1 || seen[p.Bar] {
			continue
		}
		seen[p.Bar] = true
		clean = append(clean, p)
	}
	return clean
}

// syncSnapWindow is the reflex-latency window: the user hears a beat and
// taps ~200-400ms late, so the true beat is searched in the window that
// ends at the tap.
const syncSnapWindow = 400 * time.Millisecond

// syncElapsed snaps the tap moment to the strongest onset in the 400ms
// window BEFORE the tap (the user reacts to the beat they heard). With
// per-onset strengths available the strongest onset in the window wins;
// without them the nearest onset in the window does. No onset in the
// window (or no onsets at all): the raw tap moment stands, so a tap in
// a silence gap is never yanked sideways.
func syncElapsed(elapsed time.Duration, onsets []time.Duration, strengths []float64) time.Duration {
	if len(onsets) == 0 {
		return elapsed
	}
	windowStart := elapsed - syncSnapWindow
	haveStrengths := len(strengths) == len(onsets)
	best := -1
	for i, o := range onsets {
		if o < windowStart || o > elapsed {
			continue
		}
		if best < 0 {
			best = i
			continue
		}
		if haveStrengths {
			if strengths[i] > strengths[best] {
				best = i
			}
		} else if elapsed-o < elapsed-onsets[best] {
			best = i
		}
	}
	if best < 0 {
		return elapsed
	}
	return onsets[best]
}

// setSyncPoint anchors the current bar to the current audio position. The
// tap snaps to the strongest onset in the 400ms window before it (reflex
// latency), and once enough anchors exist, confirmed outliers are dropped by
// median/MAD refinement (refineSyncPoints).
func (m ViewerModel) setSyncPoint() (ViewerModel, tea.Cmd) {
	elapsed := m.engine.Elapsed()
	snapped := syncElapsed(elapsed, m.autoOnsets, m.autoStrengths)
	bar := m.cursorBar + 1
	if bar == 1 {
		m.audioOffset = snapped.Seconds()
	}
	m.syncPoints = append(m.syncPoints, player.SyncPoint{Bar: bar, Seconds: snapped.Seconds()})
	if m.tab != nil {
		m.syncPoints = refineSyncPoints(player.BuildSchedule(m.tab), m.syncPoints)
	}
	m.saveSyncPoints()
	if m.tab != nil && m.tab.Metadata != nil {
		m.tab.Metadata[m.audioOffsetKey()] = strconv.FormatFloat(m.audioOffset, 'f', 1, 64)
		m.tab.Metadata[model.MetaKeyAudioOffset] = strconv.FormatFloat(m.audioOffset, 'f', 1, 64)
	}
	m.refresh()
	return m, m.saveTabPrefsCmd()
}

// saveSyncPoints persists the anchors (plus the bar-1 offset anchor) to the
// current source's metadata key (mirroring the legacy key) as JSON.
func (m *ViewerModel) saveSyncPoints() {
	if m.tab == nil || m.tab.Metadata == nil {
		return
	}
	points := append([]player.SyncPoint{{Bar: 1, Seconds: m.audioOffset}}, m.syncPoints...)
	seen := map[int]bool{}
	out := points[:0]
	for _, p := range points {
		if seen[p.Bar] {
			continue
		}
		seen[p.Bar] = true
		out = append(out, p)
	}
	if len(out) == 1 {
		m.tab.Metadata[m.syncPointsKey()] = ""
		m.tab.Metadata[model.MetaKeySyncPoints] = ""
		return
	}
	data, err := json.Marshal(out)
	if err != nil {
		return
	}
	m.tab.Metadata[m.syncPointsKey()] = string(data)
	m.tab.Metadata[model.MetaKeySyncPoints] = string(data)
}

// audioOffsetDur returns the calibrated intro offset as a duration.
func (m ViewerModel) audioOffsetDur() time.Duration {
	return time.Duration(m.audioOffset * float64(time.Second))
}

// driftNudge returns a one-time hint when the recording's derived tempo
// differs enough from the tab's that the un-anchored cursor will drift, or
// "" when the tempos agree (or can't be derived).
func driftNudge(derived, tabBPM int) string {
	if derived <= 0 || tabBPM <= 0 {
		return ""
	}
	diff := math.Abs(float64(derived-tabBPM)) / float64(tabBPM)
	if diff <= 0.02 {
		return ""
	}
	return fmt.Sprintf("Audio tempo ~ %d BPM vs tab %d — drifting ~%.0fs/min; press s at a recognizable bar to anchor", derived, tabBPM, diff*60)
}
