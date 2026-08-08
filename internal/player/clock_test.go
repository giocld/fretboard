package player

import (
	"fretboard/internal/model"

	"testing"
	"time"
)

// TestStepClock guards the deadline math: Start/Next roll absolute deadlines,
// Late reports positive lateness at a given now.
func TestStepClock(t *testing.T) {
	var c StepClock
	c.Start(250 * time.Millisecond)
	if c.Until() > 260*time.Millisecond || c.Until() < 240*time.Millisecond {
		t.Fatalf("Start deadline should be ~250ms out, got %v", c.Until())
	}
	first := c.Deadline()
	c.Next(125 * time.Millisecond)
	if !c.Deadline().Equal(first.Add(125 * time.Millisecond)) {
		t.Fatalf("Next should roll the deadline forward exactly, got %v want %v", c.Deadline(), first.Add(125*time.Millisecond))
	}
	// Late at a simulated now 30ms past the deadline.
	if lat := c.Late(c.Deadline().Add(30 * time.Millisecond)); lat != 30*time.Millisecond {
		t.Fatalf("Late should report +30ms, got %v", lat)
	}
	if lat := c.Late(c.Deadline().Add(-30 * time.Millisecond)); lat != -30*time.Millisecond {
		t.Fatalf("Early should report -30ms, got %v", lat)
	}
	// Rebase restarts from now.
	c.Rebase(100 * time.Millisecond)
	if c.Until() > 110*time.Millisecond || c.Until() < 90*time.Millisecond {
		t.Fatalf("Rebase should restart from now + delay, got %v", c.Until())
	}
}

// TestBuildScheduleEmitsRestSteps guards the rest-bar fix: a bar with no
// notes produces one Rest step with the bar's duration, and the metronome
// can beat on it.
func TestBuildScheduleEmitsRestSteps(t *testing.T) {
	tab := &model.Tab{Bars: []model.Bar{
		{Number: 1, Strings: []model.StringLine{{Segments: []model.Segment{
			{Char: '0', Value: 0, Position: 0, Width: 1},
			{Char: '-', Position: 1}, {Char: '-', Position: 2}, {Char: '-', Position: 3},
			{Char: '3', Value: 3, Position: 4, Width: 1},
		}}}},
		{Number: 2, Strings: []model.StringLine{{Segments: []model.Segment{
			{Char: '-', Position: 0}, {Char: '-', Position: 1}, {Char: '-', Position: 2}, {Char: '-', Position: 3},
		}}}}, // rest bar: 4 columns
		{Number: 3, Strings: []model.StringLine{{Segments: []model.Segment{
			{Char: '5', Value: 5, Position: 0, Width: 1},
		}}}},
	}}
	sched := BuildSchedule(tab)
	// bar 1: 2 note steps; bar 2: 1 rest step; bar 3: 1 note step.
	if len(sched) != 4 {
		t.Fatalf("expected 4 steps, got %d: %+v", len(sched), sched)
	}
	rest := sched[2]
	if !rest.Rest || rest.Bar != 1 || rest.Ticks != 4*(ticksPerQuarter/4) {
		t.Fatalf("rest step wrong: %+v", rest)
	}
	if sched[3].Bar != 2 {
		t.Fatalf("notes must continue after the rest, got %+v", sched[3])
	}
}

// TestBeatColumnsRestBar guards metronome beats on rest bars: quarter beats
// land on the column grid (every 8 columns).
func TestBeatColumnsRestBar(t *testing.T) {
	rest := modelBar("------", "------", "------", "------", "------", "------")
	beats := BeatColumns(rest)
	if len(beats) != 1 || beats[0] != 0 {
		t.Fatalf("6-column rest bar should beat at column 0 only, got %v", beats)
	}
	wide := modelBar("----------------", "----------------", "----------------", "----------------", "----------------", "----------------")
	beats = BeatColumns(wide)
	if len(beats) != 2 || beats[0] != 0 || beats[1] != 8 {
		t.Fatalf("16-column rest bar should beat at 0 and 8, got %v", beats)
	}
	// Rhythm-marked rest bar: beats at quarter marks.
	marked := modelBar("----------------", "----------------", "----------------", "----------------", "----------------", "----------------")
	marked.Rhythm = []model.RhythmMark{
		{Position: 0, Ticks: ticksPerQuarter},
		{Position: 8, Ticks: ticksPerQuarter},
		{Position: 12, Ticks: ticksPerQuarter / 2},
	}
	beats = BeatColumns(marked)
	if len(beats) != 2 || beats[0] != 0 || beats[1] != 8 {
		t.Fatalf("rhythm rest bar should beat at quarter marks, got %v", beats)
	}
}

// modelBar builds a single-string bar from dashes (test helper).
func modelBar(lines ...string) model.Bar {
	var sl model.StringLine
	for i, r := range lines[0] {
		_ = i
		sl.Segments = append(sl.Segments, model.Segment{Char: r, Value: 0, Position: i, Width: 1})
	}
	return model.Bar{Strings: []model.StringLine{sl}}
}
