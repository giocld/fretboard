package viewer

import (
	"testing"
	"time"

	"fretboard/internal/model"
	"fretboard/internal/player"
	"fretboard/internal/ui/msgs"
	tea "github.com/charmbracelet/bubbletea"
)

func TestMIDILateTickStillPlaysNextStep(t *testing.T) {
	m := NewViewerModel()
	m.playing = true
	m.bpm = 120
	m.playGen = 7
	m.tab = &model.Tab{Tuning: model.Standard, Bars: make([]model.Bar, 1)}
	m.schedule = []player.PlaybackStep{
		{Bar: 0, Col: 0, Ticks: 120},
		{Bar: 0, Col: 2, Ticks: 120},
		{Bar: 0, Col: 4, Ticks: 120},
		{Bar: 0, Col: 6, Ticks: 120},
	}
	m.stepClock.Start(-3 * time.Second) // UI paused past several old deadlines
	for want := 1; want < len(m.schedule); want++ {
		var cmd tea.Cmd
		m, cmd = m.Update(msgs.PlaybackTickMsg{Gen: m.playGen})
		if m.stepIdx != want || m.cursorCol != m.schedule[want].Col {
			t.Fatalf("tick %d: played index %d at col %d; must not skip any note", want, m.stepIdx, m.cursorCol)
		}
		if cmd == nil {
			t.Fatal("MIDI tick did not schedule its successor")
		}
		// The old path returned a BatchMsg with a *new* monitor for each
		// tick. Only the next MIDI tick is allowed to be scheduled here.
		if _, ok := cmd().(msgs.PlaybackTickMsg); !ok {
			t.Fatal("MIDI tick must only arm the next tick, not another monitor")
		}
	}
}

func TestPlaybackStartUsesSoundedTimeAndOneMonitor(t *testing.T) {
	m := NewViewerModel()
	m.tab = &model.Tab{Tuning: model.Standard, Bars: make([]model.Bar, 1)}
	started := time.Now().Add(-40 * time.Millisecond)
	dur := 100 * time.Millisecond
	m, cmd := m.Update(msgs.PlaybackStartedMsg{
		Schedule: []player.PlaybackStep{{Bar: 0, Ticks: 120}},
		Duration: dur, Started: started,
	})
	if got, want := m.stepClock.Deadline(), started.Add(dur); !got.Equal(want) {
		t.Fatalf("deadline = %v; first note sounded at %v and ends at %v", got, started, want)
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok || len(batch) != 2 { // one tick chain and one monitor chain
		t.Fatalf("MIDI start should arm exactly two timers; got %T", cmd())
	}
	if m.playGen == 0 {
		t.Fatal("session must have a nonzero timer generation")
	}
}

func TestStaleTimersCannotAdvanceNewSession(t *testing.T) {
	m := NewViewerModel()
	m.tab = &model.Tab{Tuning: model.Standard, Bars: make([]model.Bar, 1)}
	m, _ = m.Update(msgs.PlaybackStartedMsg{
		Schedule: []player.PlaybackStep{{Bar: 0, Ticks: 120}, {Bar: 0, Col: 2, Ticks: 120}},
		Duration: 125 * time.Millisecond,
	})
	old := m.playGen
	m.resetPlayback()
	m, _ = m.Update(msgs.PlaybackStartedMsg{
		Schedule: []player.PlaybackStep{{Bar: 0, Ticks: 120}, {Bar: 0, Col: 2, Ticks: 120}},
		Duration: 125 * time.Millisecond,
	})
	if m.playGen == old {
		t.Fatal("new session reused the old generation")
	}
	before := m.stepClock.Deadline()
	var cmd tea.Cmd
	m, cmd = m.Update(msgs.PlaybackTickMsg{Gen: old})
	if cmd != nil || m.stepIdx != 0 || !m.stepClock.Deadline().Equal(before) {
		t.Fatal("old tick advanced or scheduled work in the new session")
	}
	m, cmd = m.Update(msgs.PlaybackMonitorMsg{Gen: old})
	if cmd != nil || !m.playing {
		t.Fatal("old monitor stopped or rescheduled work in the new session")
	}
}

func TestAudioStartOnlyArmsMonitor(t *testing.T) {
	m := NewViewerModel()
	m.tab = &model.Tab{Tuning: model.Standard, Bars: make([]model.Bar, 1)}
	m, cmd := m.Update(msgs.PlaybackStartedMsg{
		Schedule:  []player.PlaybackStep{{Bar: 0, Ticks: 120}},
		AudioSync: true, Duration: 80 * time.Millisecond,
	})
	if !m.stepClock.Deadline().IsZero() {
		t.Fatal("audio should follow the player's position, not the MIDI step clock")
	}
	if _, ok := cmd().(msgs.PlaybackMonitorMsg); !ok {
		t.Fatal("audio start must only arm the cursor monitor")
	}
	_, next := m.Update(msgs.PlaybackTickMsg{Gen: m.playGen})
	if next != nil {
		t.Fatal("a stray audio tick must not start a second monitor chain")
	}
}
