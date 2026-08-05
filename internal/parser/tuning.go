package parser

import (
	"strings"

	"fretboard/internal/model"
)

func inferTuning(tab *model.Tab, stringCount int) model.Tuning {
	raw := tab.Metadata[model.MetaKeyTuningRaw]
	if raw != "" {
		low := strings.ToLower(raw)
		// Named tunings first.
		switch {
		case strings.Contains(low, "drop d") || strings.Contains(low, "drop-d") || strings.Contains(low, "dropped d"):
			return model.DropD
		case strings.Contains(low, "dadgad"):
			return model.DADGAD
		case strings.Contains(low, "open g"):
			return model.OpenG
		case strings.Contains(low, "open d"):
			return model.OpenD
		case strings.Contains(low, "half step") || strings.Contains(low, "eb standard") || strings.Contains(low, "e flat") || strings.Contains(low, "halfstep"):
			return model.HalfStepDown
		case strings.Contains(low, "full step") || strings.Contains(low, "d standard") || strings.Contains(low, "fullstep"):
			return model.FullStepDown
		}
		// Named but unspecified ("E Standard", "Standard") — try the letters.
		cleaned := model.NoteLetters(raw)
		if len(cleaned) >= 2 && len(cleaned) <= stringCount {
			if t := model.ParseTuning(cleaned); len(t) == len(cleaned) {
				return t
			}
		}
		// Single letter, empty, or unparseable: fall back to default.
	}
	// Default to standard.
	switch stringCount {
	case 7:
		return model.Standard7
	case 6:
		return model.Standard
	case 4:
		return model.Tuning{model.Standard[1], model.Standard[2], model.Standard[3], model.Standard[4]}
	}
	return model.Standard
}
