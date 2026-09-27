package player

import (
	"fmt"
	"strings"
	"testing"

	"fretboard/internal/model"
	"fretboard/internal/parser"
)

func sixStrings(lines ...string) string {
	names := []string{"e", "B", "G", "D", "A", "E"}
	var b strings.Builder
	for i, l := range lines {
		b.WriteString(names[i] + "|" + l + "|\n")
	}
	return b.String()
}

func blankStrings(first string) string {
	d := strings.Repeat("-", len(first))
	return sixStrings(first, d, d, d, d, d)
}

func barOnsets(steps []PlaybackStep) (map[int][]int, map[int]int) {
	onsets, length := map[int][]int{}, map[int]int{}
	for _, s := range steps {
		if !s.Rest {
			onsets[s.Bar] = append(onsets[s.Bar], length[s.Bar])
		}
		length[s.Bar] += s.Ticks
	}
	return onsets, length
}

// The same quarter-note rhythm typed tight, loosely padded, and with
// two-digit frets taking a dash's place must last one measure each with the
// notes on the same beats. One-column-per-16th made the padded bar half again
// as long as the tight one.
func TestSpacingBarsAreWholeMeasures(t *testing.T) {
	tab, err := parser.Parse(strings.NewReader("Width\n\n" +
		blankStrings("-0---0---0---0---") + "\n" +
		blankStrings("-0-----0-----0-----0-----") + "\n" +
		blankStrings("-12--5---10--7---")))
	if err != nil || len(tab.Bars) != 3 {
		t.Fatalf("parse: %v, %d bars", err, len(tab.Bars))
	}
	onsets, length := barOnsets(BuildSchedule(tab))
	for b := 0; b < 3; b++ {
		if length[b] != ticksPerMeasure || fmt.Sprint(onsets[b]) != "[0 480 960 1440]" {
			t.Fatalf("bar %d: length %d onsets %v, want one measure of quarters", b+1, length[b], onsets[b])
		}
	}
}

// Enter Sandman's intro riff sits in a 27-column block but ends at column 16.
// The padding must not become a rest on each of its twelve repeats.
func TestPaddedRiffRepeatsWithoutGaps(t *testing.T) {
	tab, err := parser.Parse(strings.NewReader(`e|---------------------------|
B|---------------------------|
G|---------------------------|
D|-------5-------------------|
A|-----7---------7-----------|
E|-0-------6-5---------------| (x12)
`))
	if err != nil {
		t.Fatal(err)
	}
	steps := BuildSchedule(tab)
	total := 0
	for _, s := range steps {
		total += s.Ticks
	}
	if total != 12*ticksPerMeasure {
		t.Fatalf("x12 riff should last 12 measures, got %.2f", float64(total)/ticksPerMeasure)
	}
	// 8th-note riff: E0 on 1, A7 on 2, D5 on 2&, E6 on 3, E5 on 3&, A7 on 4&.
	var first []int
	for _, s := range steps[:6] {
		first = append(first, s.Onset)
	}
	if got := fmt.Sprint(first); got != "[0 480 720 960 1200 1680]" {
		t.Fatalf("riff onsets %s", got)
	}
}

// A line holding more notes than a measure of 16ths is two measures; a line
// padded to the same width with fewer notes is not.
func TestDenseLinesSpanSeveralMeasures(t *testing.T) {
	dense := blankStrings("-0-1-2-3-4-5-6-7-8-9-0-1-2-3-4-5-6-7-8-9-")
	sparse := blankStrings("-0-1-2-3-4-5-6-7-------------------------")
	tab, err := parser.Parse(strings.NewReader("Dense\n\n" + dense + "\n" + dense + "\n" + sparse))
	if err != nil || len(tab.Bars) != 3 {
		t.Fatalf("parse: %v, %d bars", err, len(tab.Bars))
	}
	_, length := barOnsets(BuildSchedule(tab))
	if length[0] != 2*ticksPerMeasure || length[1] != 2*ticksPerMeasure || length[2] != ticksPerMeasure {
		t.Fatalf("dense lines are two measures, the padded line one; got %v", length)
	}
}

// A lone dashed line among six-string bars is a divider, not a measure.
func TestDividerLineTakesNoTime(t *testing.T) {
	note := modelBar("-0--------------")
	six := model.Bar{Strings: []model.StringLine{note.Strings[0], note.Strings[0], note.Strings[0], note.Strings[0], note.Strings[0], note.Strings[0]}}
	divider := modelBar("--------------------------------")
	steps := BarSteps(&model.Tab{Bars: []model.Bar{six, divider, six}})
	if len(steps[1]) != 0 {
		t.Fatalf("divider line got steps: %+v", steps[1])
	}
}

// A MIDI file must play what the cursor shows: note-ons at schedule times.
func TestEventsFollowSchedule(t *testing.T) {
	tab, err := parser.Parse(strings.NewReader("Sync\n\n" + blankStrings("-0---3-----5--")))
	if err != nil {
		t.Fatal(err)
	}
	evts, err := Events(tab, 120)
	if err != nil {
		t.Fatal(err)
	}
	var ons []int64
	for _, e := range evts {
		if e.Type == NoteOn {
			ons = append(ons, e.Tick)
		}
	}
	pos := 0
	for i, s := range BuildSchedule(tab) {
		if ons[i] != int64(pos) {
			t.Fatalf("note %d at tick %d, schedule says %d", i, ons[i], pos)
		}
		pos += s.Ticks
	}
}

// A bar filled to the closing pipe at the tab's own grid must keep that grid:
// stretching a near-grid bar by its content width drifted the tail notes late
// (the Sultans fixture: 8 straight 8ths came out with a 360-tick gap). Every
// uniform 4-column gap must land on a uniform 240-tick step.
func TestFilledBarKeepsItsGridRate(t *testing.T) {
	tab, err := parser.Parse(strings.NewReader("Sultans\n\n" + sixStrings(
		"---------------------------------",
		"---3---3---2---0---0---0---3---0-",
		"---------------------------------",
		"---------------------------------",
		"---------------------------------",
		"---------------------------------",
	)))
	if err != nil {
		t.Fatal(err)
	}
	if len(tab.Bars) != 1 {
		t.Fatalf("want 1 bar, got %d", len(tab.Bars))
	}
	onsets, length := barOnsets(BuildSchedule(tab))
	if length[0] != ticksPerMeasure {
		t.Fatalf("bar lasts %d ticks, want one measure", length[0])
	}
	if got := fmt.Sprint(onsets[0]); got != "[0 240 480 720 960 1200 1440 1680]" {
		t.Fatalf("uniform 8ths quantized unevenly: onsets %s", got)
	}
}
