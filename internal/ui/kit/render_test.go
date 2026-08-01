package kit

import (
	"testing"

	"fretboard/internal/model"
)

func TestBarGridLayoutFitsWidth(t *testing.T) {
	tab := &model.Tab{Tuning: model.ParseTuning("EADGBE")}
	for i := 0; i < 8; i++ {
		tab.Bars = append(tab.Bars, model.Bar{Number: i + 1, Strings: []model.StringLine{
			{Segments: []model.Segment{{Position: 0, Width: 24}}},
		}})
	}
	m := BarGridLayout(tab, 86)
	gridWidth := m.BarsPerRow * m.BarWidth
	if gridWidth > 86 {
		t.Fatalf("grid %d exceeds width 86", gridWidth)
	}
	if m.BarWidth < 24 {
		t.Fatalf("BarWidth %d clips 24-wide content", m.BarWidth)
	}
	if m.BarsPerRow < 2 {
		t.Fatalf("expected multiple bars per row, got %d", m.BarsPerRow)
	}
}

func TestBarGridLayoutWideBarPans(t *testing.T) {
	tab := &model.Tab{Tuning: model.ParseTuning("EADGBE")}
	tab.Bars = append(tab.Bars, model.Bar{Number: 1, Strings: []model.StringLine{
		{Segments: []model.Segment{{Position: 0, Width: 80}}},
	}})
	m := BarGridLayout(tab, 40)
	if m.BarsPerRow != 1 {
		t.Fatalf("wide bar should force one per row, got %d", m.BarsPerRow)
	}
	if m.BarWidth < 80 {
		t.Fatalf("BarWidth %d clips 80-wide content", m.BarWidth)
	}
}
