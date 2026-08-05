package parser

import (
	"strings"
	"testing"

	"fretboard/internal/model"
)

// TestInferTuningNamedVariants guards against common tuning labels collapsing
// into truncated or empty tunings via NoteLetters ("Dropped D" → "DD" used to
// produce a 2-string tuning, silently dropping notes on every other string).
func TestInferTuningNamedVariants(t *testing.T) {
	tests := []struct {
		label string
		want  model.Tuning
	}{
		{"Dropped D", model.DropD},
		{"Dropped D Tuning", model.DropD},
		{"Drop D", model.DropD},
		{"Open D", model.OpenD},
		{"Open D Tuning", model.OpenD},
		{"Open G", model.OpenG},
		{"DADGAD", model.DADGAD},
		{"E Flat", model.HalfStepDown},
		{"E Flat Standard", model.HalfStepDown},
		{"Eb Standard", model.HalfStepDown},
		{"Half Step Down", model.HalfStepDown},
		{"D Standard", model.FullStepDown},
	}
	for _, tc := range tests {
		tab, err := Parse(strings.NewReader(`Tune Check
Tuning: ` + tc.label + `

E|----0----|
B|----0----|
G|----0----|
D|----0----|
A|----0----|
E|----0----|
`))
		if err != nil {
			t.Fatalf("%s: parse: %v", tc.label, err)
		}
		if len(tab.Tuning) != len(tc.want) {
			t.Fatalf("%s: tuning has %d strings, want %d: %v", tc.label, len(tab.Tuning), len(tc.want), tab.Tuning)
		}
		for i := range tc.want {
			if tab.Tuning[i] != tc.want[i] {
				t.Fatalf("%s: tuning[%d] = %d, want %d (%v)", tc.label, i, tab.Tuning[i], tc.want[i], tab.Tuning)
			}
		}
	}
}

// TestInferTuningGarbageLabelsFallBackToStandard ensures unrecognizable labels
// never produce a shorter-than-needed tuning (which used to drop notes).
func TestInferTuningGarbageLabelsFallBackToStandard(t *testing.T) {
	for _, label := range []string{"Standard Tuning", "Standard", "Tuning", "Weird Stuff"} {
		tab, err := Parse(strings.NewReader(`Fallback
Tuning: ` + label + `

E|----0----|
B|----0----|
G|----0----|
D|----0----|
A|----0----|
E|----0----|
`))
		if err != nil {
			t.Fatalf("%s: parse: %v", label, err)
		}
		if len(tab.Tuning) != len(model.Standard) {
			t.Fatalf("%s: expected fallback Standard (%d strings), got %v", label, len(model.Standard), tab.Tuning)
		}
	}
}
