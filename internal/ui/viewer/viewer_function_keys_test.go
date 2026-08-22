package viewer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fretboard/internal/model"
	"fretboard/internal/player"
)

// audioSeekingModel returns a viewer whose engine plays a local file in
// audio mode, with the fake mpv reporting the given position/duration. The
// fake reports the duration asynchronously, so the test waits for it.
func audioSeekingModel(t *testing.T, status string) ViewerModel {
	t.Helper()
	writeFakeMPVTest(t, status)
	m := NewViewerModel()
	dir := t.TempDir()
	audio := filepath.Join(dir, "backing.mp3")
	if err := os.WriteFile(audio, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	tab := &model.Tab{Title: "X", Tuning: model.Standard,
		Bars: []model.Bar{{Strings: []model.StringLine{{Segments: []model.Segment{{Char: '0', Value: 0, Position: 0, Width: 1}}}}}}}
	if err := m.engine.PlaySource(tab, 120, player.AudioSource{Kind: player.SourceLocal, Path: audio}, player.PlayContext{}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for m.engine.AudioDuration() <= 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if m.engine.AudioDuration() <= 0 {
		t.Fatal("fake mpv never reported a duration")
	}
	m.playing = true
	return m
}

// TestSeekByClamps pins the seek math: a seek past the track end clamps to
// the audio duration, a seek before zero clamps to zero, and a model that is
// not in audio playback reports no seek at all.
func TestSeekByClamps(t *testing.T) {
	t.Run("nearEndClampsToDuration", func(t *testing.T) {
		m := audioSeekingModel(t, `{"pos": 50.0, "dur": 60.0}`)
		pos, ok := seekBy(m, 15*time.Second)
		if !ok {
			t.Fatal("seekBy on a playing audio model should succeed")
		}
		if pos != 60*time.Second {
			t.Fatalf("seekBy(15s from 50s of 60s) = %v, want 60s clamp", pos)
		}
		m.engine.Stop()
	})
	t.Run("belowZeroClampsToZero", func(t *testing.T) {
		m := audioSeekingModel(t, `{"pos": 0.0, "dur": 60.0}`)
		pos, ok := seekBy(m, -15*time.Second)
		if !ok {
			t.Fatal("seekBy on a playing audio model should succeed")
		}
		if pos != 0 {
			t.Fatalf("seekBy(-15s from 0) = %v, want 0 clamp", pos)
		}
		m.engine.Stop()
	})
	t.Run("noAudioReturnsFalse", func(t *testing.T) {
		m := NewViewerModel()
		tab := &model.Tab{Title: "X", Tuning: model.Standard,
			Bars: []model.Bar{{Strings: []model.StringLine{{Segments: []model.Segment{{Char: '0', Value: 0, Position: 0, Width: 1}}}}}}}
		m.LoadTab(tab, "x.txt", 0)
		m.playing = true
		if pos, ok := seekBy(m, 15*time.Second); ok || pos != 0 {
			t.Fatalf("seekBy without audio = (%v, %v), want (0, false)", pos, ok)
		}
	})
}

// TestF7SeeksForward guards the F7 seek shortcut: on a playing audio model it
// re-seeks the engine without error and keeps audio alive. The fake mpv
// re-reports its canned position after the restart, so the assertion is that
// the seek path ran cleanly, not the exact target position.
func TestF7SeeksForward(t *testing.T) {
	m := audioSeekingModel(t, `{"pos": 0.0, "dur": 60.0}`)
	m, cmd := m.Update(key("f7"))
	if cmd != nil {
		t.Fatalf("f7 should not return a cmd, got %v", cmd)
	}
	if m.errMsg != "" {
		t.Fatalf("f7 surfaced an error: %q", m.errMsg)
	}
	if m.engine.Mode() != "audio" {
		t.Fatalf("f7 dropped audio mode: %q", m.engine.Mode())
	}
	if el := m.engine.Elapsed(); el < 0 || el > 60*time.Second {
		t.Fatalf("elapsed after f7 out of range: %v", el)
	}
	m.engine.Stop()
}

// TestF8CyclesCountIn guards F8 mirroring the C key: count-in cycles
// 0 -> 1 -> 2 -> 0.
func TestF8CyclesCountIn(t *testing.T) {
	m := NewViewerModel()
	tab := &model.Tab{Title: "X", Tuning: model.Standard,
		Bars: []model.Bar{{Strings: []model.StringLine{{Segments: []model.Segment{{Char: '0', Value: 0, Position: 0, Width: 1}}}}}}}
	m.LoadTab(tab, "x.txt", 0)
	for _, want := range []int{1, 2, 0} {
		m, _ = m.Update(key("f8"))
		if m.countIn != want {
			t.Fatalf("countIn after f8 = %d, want %d", m.countIn, want)
		}
	}
}

// TestF12ShowsEventSummary guards F12 reporting the repeat-order event
// summary in the status info line.
func TestF12ShowsEventSummary(t *testing.T) {
	m := NewViewerModel()
	tab := &model.Tab{Title: "X", Tuning: model.Standard,
		Bars: []model.Bar{{Strings: []model.StringLine{{Segments: []model.Segment{{Char: '0', Value: 0, Position: 0, Width: 1}}}}}}}
	m.LoadTab(tab, "x.txt", 0)
	m, _ = m.Update(key("f12"))
	if !strings.Contains(m.infoMsg, "events") {
		t.Fatalf("infoMsg = %q, want an event summary", m.infoMsg)
	}
	if m.errMsg != "" {
		t.Fatalf("f12 surfaced an error: %q", m.errMsg)
	}
}
