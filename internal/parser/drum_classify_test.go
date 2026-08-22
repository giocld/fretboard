package parser

import (
	"strings"
	"testing"
)

func TestDrumTabClassifiesAsTab(t *testing.T) {
	src := `Song
Tuning: E Standard

HH|--x---x---x---x-|
SD|x-------x-------|
BD|----x-------x---|
`
	if Classify(strings.Split(src, "\n")) != SheetTab {
		t.Fatal("drum tab must classify as tab, not chord sheet")
	}
	tab, err := Parse(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(tab.Bars) == 0 {
		t.Fatal("drum tab should parse bars")
	}
}
