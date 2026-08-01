package player_test

import (
	"strings"
	"testing"

	"fretboard/internal/parser"
	"fretboard/internal/player"
)

func TestLaylaLikeSchedule(t *testing.T) {
	content := `Layla
Eric Clapton
Tuning: EADGBE

e|----0-----|
B|----------|
G|----------|
D|----------|
A|----------|
E|----------|

e|----3-----|
B|----------|
G|----------|
D|----------|
A|----------|
E|----------|
`
	tab, err := parser.Parse(strings.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("bars=%d tuning=%v", len(tab.Bars), tab.Tuning)
	steps := player.BuildSchedule(tab)
	t.Logf("steps=%d", len(steps))
	if len(steps) == 0 {
		t.Fatal("no playback steps")
	}
	evts, err := player.Events(tab, 120)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	t.Logf("events=%d", len(evts))
}

func TestHammerOnEvents(t *testing.T) {
	content := `Layla
Eric Clapton
Tuning: EADGBE

e|--------------------------------|
B|10h12p10------------------------|
G|--------------------------------|
D|--------------------------------|
A|--------------------------------|
E|--------------------------------|
`
	tab, err := parser.Parse(strings.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	if len(tab.Bars) == 0 {
		t.Fatal("no bars parsed")
	}
	evts, err := player.Events(tab, 120)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	steps := player.BuildSchedule(tab)
	if len(evts) == 0 || len(steps) == 0 {
		t.Fatalf("no playable notes: events=%d steps=%d", len(evts), len(steps))
	}
}
