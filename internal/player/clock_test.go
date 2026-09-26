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
	// A rest bar is a whole measure of rest, whatever its width on the page.
	rest := sched[2]
	if !rest.Rest || rest.Bar != 1 || rest.Ticks != ticksPerMeasure {
		t.Fatalf("rest step wrong: %+v", rest)
	}
	if sched[3].Bar != 2 {
		t.Fatalf("notes must continue after the rest, got %+v", sched[3])
	}
}

// TestRestBarIsAMeasureOnTheBeat guards metronome beats on rest bars: a
// rest bar is a whole measure whose single step starts on the downbeat, so
// the metronome clicks (accented) as it begins.
func TestRestBarIsAMeasureOnTheBeat(t *testing.T) {
	rest := modelBar("----------------")
	steps := BarSteps(&model.Tab{Bars: []model.Bar{rest}})[0]
	if len(steps) != 1 || !steps[0].Rest || steps[0].Ticks != ticksPerMeasure || steps[0].Onset != 0 {
		t.Fatalf("rest bar should be one measure-long step on beat 1, got %+v", steps)
	}
	// A rhythm row sets a rest bar's length.
	marked := modelBar("----------------")
	marked.Rhythm = []model.RhythmMark{{Position: 0, Ticks: ticksPerQuarter}, {Position: 8, Ticks: ticksPerQuarter}}
	if got := BarSteps(&model.Tab{Bars: []model.Bar{marked}})[0][0].Ticks; got != 2*ticksPerQuarter {
		t.Fatalf("rhythm rest bar should last its marks (960), got %d", got)
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
