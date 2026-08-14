package viewer

import (
	"fmt"
	"path/filepath"
	"strings"

	"fretboard/internal/model"
	"fretboard/internal/player"
	"fretboard/internal/ui/kit"
)

// View is part of the tea.Model interface. The tab viewer separates the title
// (primary), the status row (secondary), and the tab body (content): the
// status indicators no longer pile onto a single title line.
func (m ViewerModel) View() string {
	crumb := kit.FormatBreadcrumb("home", "library", "viewer")
	var title, status string
	if m.tab != nil {
		crumb = kit.FormatBreadcrumb("home", "library", m.tab.Title)
		title = kit.PanelTitleStyle.Render(m.tab.Title)
		if m.tab.Artist != "" {
			title += kit.MutedStyle.Render("  — " + m.tab.Artist)
		}
		if b := strings.TrimSpace(m.tab.Metadata[model.MetaKeySourceBadge]); b != "" {
			title += kit.MutedStyle.Render("  " + b)
		}
		// The two-axis state label leads the status row: [load|sync] plus
		// the calibrating "..." and track-ended "[end]" tags.
		status = kit.MutedStyle.Render(fmt.Sprintf("%s · %s · bar %d/%d · %d BPM",
			syncStateOf(m).label(), m.tab.Tuning.Label(), m.cursorBar+1, len(m.tab.Bars), m.bpm))
		if sec := m.currentSection(); sec != "" {
			status = kit.MutedStyle.Render(sec) + "  " + status
		}
		if m.playing {
			label := m.engine.ActiveDriver
			if m.engine.Mode() == "audio" {
				if label == "" {
					label = "audio"
				}
				status += "  " + kit.SuccessStyle.Render(""+label)
			} else if label != "" {
				status += "  " + kit.SuccessStyle.Render("midi:"+label)
			} else {
				status += "  " + kit.SuccessStyle.Render("midi")
			}
		}
		if src := m.selectedSource(); !m.playing {
			if m.fetchingCatalog || m.fetchingAudio {
				status += kit.MutedStyle.Render("  ... finding audio")
			} else if src.Kind == player.SourceMIDI {
				status += kit.MutedStyle.Render("  midi")
			} else if m.resolvedAudio != "" {
				status += kit.MutedStyle.Render("   " + filepath.Base(m.resolvedAudio))
			}
		}
		if m.audioOffset != 0 {
			status += kit.MutedStyle.Render(fmt.Sprintf("  offset %+.1fs", m.audioOffset))
		}
		if len(m.syncPoints) > 0 {
			status += kit.MutedStyle.Render(fmt.Sprintf("  anchors %d", len(m.syncPoints)))
		}
		if q, ok := m.syncQuality(); ok {
			status += kit.InfoStyle.Render(fmt.Sprintf("  ±%.2fs", q))
		}
		if map_, ok := m.tempoMap(); ok {
			status += kit.InfoStyle.Render(fmt.Sprintf("  %d->%d bpm", map_[0], map_[1]))
		}
		if m.loopStartBar > 0 && m.loopEndBar > 0 {
			status += kit.MutedStyle.Render(fmt.Sprintf("  loop %d-%d", m.loopStartBar, m.loopEndBar))
		}
		if m.metronome {
			status += kit.MutedStyle.Render("   metronome")
		}
		if m.countIn > 0 {
			status += kit.MutedStyle.Render(fmt.Sprintf("  %d-bar count-in", m.countIn))
		}
		if m.program != 0 {
			status += kit.InfoStyle.Render("  " + programLabel(m.program))
		}
		if m.transpose != 0 {
			status += kit.InfoStyle.Render(fmt.Sprintf("  transpose %+d", m.transpose))
		}
		if m.showNotes {
			status += kit.InfoStyle.Render("  notes")
		}
		if len(m.searchMatches) > 0 {
			status += kit.WarningStyle.Render(fmt.Sprintf("  [%s] %d/%d", m.searchInput, m.searchIdx+1, len(m.searchMatches)))
		}
		if m.perfMode {
			status += kit.InfoStyle.Render("  perf")
		}
		if m.driftMs != 0 && !m.audioSync {
			status += kit.WarningStyle.Render(fmt.Sprintf("  drift %dms", m.driftMs))
		}
		// SyncDrift carries the live drift magnitude the [load|drift] state
		// tag cannot; the plain auto-sync marker is folded into the state.
		if m.audioSync && m.autoActive && (m.syncDrift > 0.04 || m.syncDrift < -0.04) {
			status += kit.WarningStyle.Render(fmt.Sprintf("  drift %+.2fs", m.syncDrift))
		}
		if total := m.practiceTotal(); total > 0 {
			status += kit.MutedStyle.Render(fmt.Sprintf("  practice %d:%02d", total/60, total%60))
		}
		if rate := m.engine.Rate(); rate != 1 {
			status += kit.MutedStyle.Render(fmt.Sprintf("  x %.2f", rate))
		}
	}
	if m.jumpBuffer != "" {
		status += kit.MutedStyle.Render("  [jump " + m.jumpBuffer + "]")
	}
	if m.infoMsg != "" {
		status += "  " + kit.InfoStyle.Render(kit.Truncate(m.infoMsg, 48))
	}
	if m.errMsg != "" {
		status += "  " + kit.ErrorStyle.Render("! "+kit.Truncate(m.errMsg, 48))
	}
	status = kit.Truncate(status, m.width-8)

	body := "\n"
	if m.tab != nil {
		if m.searchActive {
			body += kit.RenderPanel(m.width-2, "Search tab", "/ "+m.searchInput+"_") + "\n"
		}
		body += title + "\n"
		if status != "" {
			body += status + "\n"
		}
		body += kit.RenderDivider(m.width-4) + "\n\n"
		body += kit.RenderPanel(m.width-2, "", m.viewport.View())
	} else {
		body += kit.RenderPanel(m.width-2, "Tab", m.viewport.View())
	}
	if m.showAlignmentConfirm {
		body += renderAlignmentConfirm(m)
	} else if m.showAudioPicker {
		body += RenderAudioPicker(m.width, m.audioCatalog, m.audioCursor, m.fetchingCatalog, m.strictAudio, m.recommendedSourceIdx(), rejectedSources(m.tab))
	}

	playLabel := "play"
	if m.playing {
		playLabel = "pause"
	}
	statusLine := ""
	if m.tab != nil && m.tab.Title != "" {
		statusLine = fmt.Sprintf(" %s", kit.Truncate(m.tab.Title, 24))
	}
	footer := kit.RenderFooterWithStatus(m.width, statusLine, []kit.KeyHint{
		{Key: "a", Label: "audio"},
		{Key: "Space/p", Label: playLabel},
		{Key: "+/-", Label: "BPM"},
		{Key: "> <", Label: "speed"},
		{Key: "m", Label: "metronome"},
		{Key: "C", Label: "count-in"},
		{Key: "y", Label: "instrument"},
		{Key: "[ ] , .", Label: "sync"},
		{Key: "o", Label: "reset"},
		{Key: "s", Label: "sync bar"},
		{Key: "i/u", Label: "loop"},
		{Key: "v", Label: "layout"},
		{Key: "f", Label: "follow"},
		{Key: "P", Label: "perf"},
		{Key: "X", Label: "export"},
		{Key: "j/k", Label: "scroll"},
		{Key: "/", Label: "search"},
		{Key: "n/N", Label: "next"},
		{Key: "T/Z", Label: "transpose"},
		{Key: "e", Label: "notes"},
		{Key: "b", Label: "library"},
		{Key: "q", Label: "quit"},
	})
	return kit.LayoutScreen(m.width, m.height, crumb, body, footer)
}

// renderPerformance renders the performance-mode view: the current section
// large, the progress through the song, and the playback state — no tab
// body, so the musician plays from memory.
func (m ViewerModel) renderPerformance() string {
	if m.tab == nil {
		return ""
	}
	sec := m.currentSection()
	if sec == "" {
		sec = fmt.Sprintf("Section %d", m.cursorBar+1)
	}
	total := len(m.tab.Bars)
	n := m.cursorBar + 1
	pct := 0
	if total > 0 {
		pct = n * 100 / total
	}
	state := "paused"
	if m.playing {
		state = "playing"
	}
	src := "midi"
	if m.engine.Mode() == "audio" {
		src = "audio"
	}
	const scale = 40
	filled := 0
	if total > 0 {
		filled = n * scale / total
	}
	return strings.Join([]string{
		"",
		kit.PanelTitleStyle.Render("   " + sec),
		"",
		kit.InfoStyle.Render(fmt.Sprintf("   bar %d / %d  (%d%%)", n, total, pct)),
		"",
		kit.MutedStyle.Render(fmt.Sprintf("   BPM %d   %s   %s", m.bpm, state, src)),
		"",
		kit.SuccessStyle.Render("   " + strings.Repeat("#", filled) + strings.Repeat(".", scale-filled)),
		"",
	}, "\n")
}

// currentSection returns the section name of the cursor bar, walking back to
// the nearest bar that names its section.
func (m ViewerModel) currentSection() string {
	if m.tab == nil {
		return ""
	}
	for b := m.cursorBar; b >= 0 && b < len(m.tab.Bars); b-- {
		if sec := strings.TrimSpace(m.tab.Bars[b].Section); sec != "" {
			return sec
		}
	}
	return ""
}

func (m *ViewerModel) maxPanOffset() int {
	if m.tab == nil {
		return 0
	}
	metrics := kit.BarGridLayout(m.tab, m.viewport.Width)
	gridWidth := metrics.BarsPerRow * metrics.BarWidth
	visible := m.viewport.Width - 10
	if visible < 20 {
		visible = 20
	}
	if gridWidth <= visible {
		return 0
	}
	return gridWidth - visible
}

// cursorBarLineOffset returns the content line where the cursor bar begins,
// using the active layout's own row math (grid rows vs linear blocks) so
// follow-scroll lands exactly on the playhead.
func (m *ViewerModel) cursorBarLineOffset() int {
	if m.tab == nil || m.cursorBar < 0 {
		return 0
	}
	var offsets []int
	if m.linear {
		offsets = kit.LinearBarLineOffsets(m.tab)
	} else {
		offsets = kit.GridBarLineOffsets(m.tab, m.viewport.Width)
	}
	if m.cursorBar >= len(offsets) {
		return 0
	}
	return offsets[m.cursorBar]
}
