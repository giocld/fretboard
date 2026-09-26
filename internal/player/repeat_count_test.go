package player

import (
	"fmt"
	"testing"

	"fretboard/internal/model"
)

// Play counts expand in RepeatOrder, beyond the 3x safety cap that guards
// against broken "|:" chains: a riff marked x12 plays twelve times.
func TestRepeatOrderPlaysCounts(t *testing.T) {
	tab := &model.Tab{Bars: []model.Bar{
		{Times: 12}, // bar 1 x12
		{},          // bar 2
		{},          // bar 3
		{Times: 2, TimesFrom: 2},
	}}
	got := fmt.Sprint(RepeatOrder(tab))
	want := "[0 0 0 0 0 0 0 0 0 0 0 0 1 2 3 2 3]"
	if got != want {
		t.Fatalf("RepeatOrder = %s, want %s", got, want)
	}
	// Counts and "|: :|" sections combine: the section plays twice, then
	// the counted bar after it.
	tab = &model.Tab{Bars: []model.Bar{{RepeatStart: true}, {RepeatEnd: true}, {Times: 3, TimesFrom: 2}}}
	if got := fmt.Sprint(RepeatOrder(tab)); got != "[0 1 0 1 2 2 2]" {
		t.Fatalf("section + count = %s, want [0 1 0 1 2 2 2]", got)
	}
}
