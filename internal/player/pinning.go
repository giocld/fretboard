package player

import (
	"fmt"
	"strings"

	"fretboard/internal/model"
)

// MetaKeyPinnedVideo is the tab.Metadata key holding the user's permanent
// choice of YouTube video for a tab. A pinned id outranks every search
// heuristic: it is returned directly and re-ranked first, so the pin keeps
// winning even when the search engine stops surfacing the video.
const MetaKeyPinnedVideo = "pinned_video"

// PinnedVideoFor returns the permanently pinned YouTube video id for the
// tab, if one is set. A whitespace-only value counts as no pin.
func PinnedVideoFor(tab *model.Tab) (videoID string, ok bool) {
	if tab == nil || tab.Metadata == nil {
		return "", false
	}
	id := strings.TrimSpace(tab.Metadata[MetaKeyPinnedVideo])
	return id, id != ""
}

// PinVideoFor permanently pins a YouTube video id to the tab, replacing any
// earlier pin. The id is stored trimmed; empty ids and nil tabs are errors.
func PinVideoFor(tab *model.Tab, videoID string) error {
	if tab == nil {
		return fmt.Errorf("pin video: nil tab")
	}
	id := strings.TrimSpace(videoID)
	if id == "" {
		return fmt.Errorf("pin video: empty video id")
	}
	if tab.Metadata == nil {
		tab.Metadata = map[string]string{}
	}
	tab.Metadata[MetaKeyPinnedVideo] = id
	return nil
}
