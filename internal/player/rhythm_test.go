package player

import (
	"strings"
	"testing"

	"fretboard/internal/model"
	"fretboard/internal/parser"
)

func TestRhythmSpacingInfersDurations(t *testing.T) {
	tab, err := parser.Parse(strings.NewReader(`Uneven

E|--0---0-------3---|
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(tab.Bars) == 0 {
		t.Fatal("expected bars")
	}
	bar := tab.Bars[0]
	cols := NoteColumns(bar)
	if len(cols) < 3 {
		t.Fatalf("expected at least 3 note columns, got %v", cols)
	}
	// Uneven spacing: gaps of 4 vs 8 columns
	sp0 := columnSpacing(bar, cols[0], maxColumns(bar.Strings), cols, 0)
	sp1 := columnSpacing(bar, cols[1], maxColumns(bar.Strings), cols, 1)
	if sp0 == sp1 {
		t.Fatalf("expected different spacing between first two notes, got %d and %d (cols=%v)", sp0, sp1, cols)
	}
}

func TestRhythmMarkersOverrideSpacing(t *testing.T) {
	bar := model.Bar{
		Strings: []model.StringLine{{
			Segments: []model.Segment{
				{Char: '0', Value: 0, Position: 0, Width: 1},
				{Char: '3', Value: 3, Position: 8, Width: 1},
			},
		}},
		Rhythm: []model.RhythmMark{
			{Position: 0, Ticks: 960},
			{Position: 8, Ticks: 240},
		},
	}
	if got := columnTicks(bar, 0, 12, []int{0, 8}, 0); got != 960 {
		t.Fatalf("rhythm tick at 0: got %d want 960", got)
	}
	if got := columnTicks(bar, 8, 12, []int{0, 8}, 1); got != 240 {
		t.Fatalf("rhythm tick at 8: got %d want 240", got)
	}
}

func TestRhythmTicksForNoteUsesNearestMark(t *testing.T) {
	tab, err := parser.Parse(strings.NewReader(`Rhythm Tab
Tuning: E Standard

| q  e  e  q  |
E|----0-----3-----5---|
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	bar := tab.Bars[0]
	cols := NoteColumns(bar)
	// Marks are rebased into the bar's content columns, so the e above the
	// first note and the q above the second note match exactly.
	if got := rhythmTicksForNote(bar, cols[0]); got != 240 {
		t.Fatalf("first note sustain: got %d want 240 (e above it)", got)
	}
	if got := rhythmTicksForNote(bar, cols[1]); got != 480 {
		t.Fatalf("second note sustain: got %d want 480 (q above it)", got)
	}
	if got := rhythmTicksForNote(bar, cols[2]); got != 480 {
		t.Fatalf("third note sustain: got %d want 480 (nearest preceding q)", got)
	}
}

func TestRhythmAwareNoteSustain(t *testing.T) {
	tab, err := parser.Parse(strings.NewReader(`Rhythm Tab
Tuning: E Standard

|    q      e      q  |
E|----0-----3-----5---|
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	evts, err := Events(tab, 120)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}

	// Collect note durations in event order: the q/e/q marks above the three
	// notes must yield quarter (480), eighth (240), and a clamped eighth (240)
	// sustain.
	var durations []int64
	var onTick int64 = -1
	for _, e := range evts {
		if e.Type == NoteOn {
			onTick = e.Tick
		}
		if e.Type == NoteOff && e.Tick > onTick {
			durations = append(durations, e.Tick-onTick)
			onTick = e.Tick
		}
	}
	if len(durations) < 3 {
		t.Fatalf("expected at least 3 note durations, got %v", durations)
	}
	if durations[0] != 480 || durations[1] != 240 || durations[2] != 240 {
		t.Fatalf("unexpected durations: %v (want 480, 240, 240)", durations)
	}
	if durations[0] <= durations[1] {
		t.Fatalf("quarter should last longer than eighth: %v", durations)
	}
}

func TestBuildScheduleSkipsEmptyColumns(t *testing.T) {
	tab, err := parser.Parse(strings.NewReader(`Mono
Tuning: E Standard

E|----0-----3-----5---|
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	steps := BuildSchedule(tab)
	if len(steps) < 3 {
		t.Fatalf("expected at least 3 steps, got %d", len(steps))
	}
	for i := 1; i < len(steps); i++ {
		if steps[i].Bar < steps[i-1].Bar {
			t.Fatalf("steps out of bar order")
		}
	}
}

func TestBuildSchedulePopulatesSustain(t *testing.T) {
	tab := &model.Tab{
		Tuning: model.Standard,
		Bars: []model.Bar{
			{
				Strings: []model.StringLine{{
					Segments: []model.Segment{
						{Char: '0', Value: 0, Position: 0, Width: 1},
						{Char: '3', Value: 3, Position: 4, Width: 1},
						{Char: '5', Value: 5, Position: 8, Width: 1},
					},
				}},
				Rhythm: []model.RhythmMark{
					{Position: 0, Ticks: 960},
					{Position: 1, Ticks: 120},
				},
			},
			{
				Strings: []model.StringLine{{
					Segments: []model.Segment{
						{Char: '0', Value: 0, Position: 2, Width: 1},
						{Char: '3', Value: 3, Position: 6, Width: 1},
					},
				}},
				Rhythm: []model.RhythmMark{
					{Position: 1, Ticks: 960},
				},
			},
		},
	}
	steps := BuildSchedule(tab)
	want := []struct{ ticks, sustain int }{
		{960, 960}, // bar 0 col 0: exact rhythm mark, full quarter
		{480, 120}, // bar 0 col 4: rhythm sustain 120 respected under advance 480
		{120, 120}, // bar 0 col 8: tail advance 120
		{480, 480}, // bar 1 col 2: rhythm sustain 960 clamped to advance 480
		{120, 120}, // bar 1 col 6: tail advance 120, long rhythm clamped
	}
	if len(steps) != len(want) {
		t.Fatalf("expected %d steps, got %d: %+v", len(want), len(steps), steps)
	}
	for i, w := range want {
		st := steps[i]
		if st.Sustain <= 0 {
			t.Fatalf("step %d %+v: sustain must be positive", i, st)
		}
		if st.Sustain > st.Ticks {
			t.Fatalf("step %d %+v: sustain %d exceeds ticks %d", i, st, st.Sustain, st.Ticks)
		}
		if st.Ticks != w.ticks || st.Sustain != w.sustain {
			t.Fatalf("step %d: got ticks=%d sustain=%d want ticks=%d sustain=%d", i, st.Ticks, st.Sustain, w.ticks, w.sustain)
		}
	}
}

func TestBuildScheduleSustainFromParsedRhythm(t *testing.T) {
	tab, err := parser.Parse(strings.NewReader(`Rhythm Tab
Tuning: E Standard

| q  e  e  q  |
E|----0-----3-----5---|
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	steps := BuildSchedule(tab)
	if len(steps) < 3 {
		t.Fatalf("expected at least 3 steps, got %d", len(steps))
	}
	for i, st := range steps {
		if st.Sustain <= 0 || st.Sustain > st.Ticks {
			t.Fatalf("step %d %+v: sustain %d out of range 1..%d", i, st, st.Sustain, st.Ticks)
		}
	}
}

func TestStepIndexAtPosition(t *testing.T) {
	schedule := []PlaybackStep{
		{Bar: 0, Col: 0},
		{Bar: 0, Col: 4},
		{Bar: 1, Col: 0},
	}
	if got := StepIndexAtPosition(schedule, 0, 0); got != 0 {
		t.Fatalf("at start got %d", got)
	}
	if got := StepIndexAtPosition(schedule, 0, 3); got != 1 {
		t.Fatalf("mid bar got %d want 1", got)
	}
	if got := StepIndexAtPosition(schedule, 1, 0); got != 2 {
		t.Fatalf("bar 1 got %d want 2", got)
	}
}
