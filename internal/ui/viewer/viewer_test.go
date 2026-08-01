package viewer

import (
	"os"
	"path/filepath"
	"testing"

	"fretboard/internal/model"
	"fretboard/internal/player"
	"fretboard/internal/ui/msgs"
	tea "github.com/charmbracelet/bubbletea"
)

func TestAudioFetchedMsgUpdatesCatalogPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "backing.mp3")
	if err := os.WriteFile(path, []byte("fake"), 0644); err != nil {
		t.Fatal(err)
	}

	m := NewViewerModel()
	m.tab = &model.Tab{Title: "Layla", Artist: "Clapton"}
	m.tabID = 42
	m.selectedSourceIdx = 1
	m.audioCatalog = player.AudioCatalog{
		Sources: []player.AudioSource{
			{ID: "midi", Kind: player.SourceMIDI, Label: "MIDI"},
			{ID: "yt:abc", Kind: player.SourceOnline, Label: "YouTube", VideoID: "abc"},
		},
	}

	updated, _ := m.Update(msgs.AudioFetchedMsg{
		Path:    path,
		Artist:  "Clapton",
		Title:   "Layla",
		TabID:   42,
		TabPath: "online://ug/1",
	})
	m = updated

	if got := m.audioCatalog.Sources[1].Path; got != path {
		t.Fatalf("catalog path = %q, want %q", got, path)
	}
	if m.resolvedAudio != path {
		t.Fatalf("resolvedAudio = %q, want %q", m.resolvedAudio, path)
	}
}

func TestTogglePlaybackUsesResolvedOnlineAudio(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "backing.mp3")
	if err := os.WriteFile(path, []byte("fake"), 0644); err != nil {
		t.Fatal(err)
	}

	m := NewViewerModel()
	m.tab = &model.Tab{Title: "Layla", Artist: "Clapton", Bars: []model.Bar{{Strings: []model.StringLine{{}}}}}
	m.selectedSourceIdx = 1
	m.resolvedAudio = path
	m.audioCatalog = player.AudioCatalog{
		Sources: []player.AudioSource{
			{ID: "midi", Kind: player.SourceMIDI, Label: "MIDI"},
			{ID: "yt:abc", Kind: player.SourceOnline, Label: "YouTube", VideoID: "abc"},
		},
	}

	cmd := m.togglePlayback()
	if cmd == nil {
		t.Fatal("expected playback cmd when resolved audio is cached")
	}
}

func TestPlaybackStartIndexFromCursor(t *testing.T) {
	m := NewViewerModel()
	m.tab = &model.Tab{
		Title: "T",
		Bars: []model.Bar{{
			Strings: []model.StringLine{
				{Segments: []model.Segment{{Char: '0', Value: 0, Position: 0, Width: 1}, {Char: '3', Value: 3, Position: 4, Width: 1}}},
				{Segments: []model.Segment{{Char: '-', Value: -1, Position: 0, Width: 1}}},
			},
		}},
	}
	m.cursorBar = 0
	m.cursorCol = 4
	if got := m.playbackStartIndex(); got != 1 {
		t.Fatalf("playbackStartIndex = %d, want 1", got)
	}
}

func TestPickAudioSourceIndexPrefersLocal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "backing.mp3")
	if err := os.WriteFile(path, []byte("fake"), 0644); err != nil {
		t.Fatal(err)
	}
	cat := player.AudioCatalog{Sources: []player.AudioSource{
		{ID: "midi", Kind: player.SourceMIDI, Label: "MIDI"},
		{ID: "local:1", Kind: player.SourceLocal, Label: "Local", Path: path},
		{ID: "yt:abc", Kind: player.SourceOnline, Label: "YouTube", VideoID: "abc", Score: 999},
	}}
	if got := pickAudioSourceIndex(nil, cat); got != 1 {
		t.Fatalf("pickAudioSourceIndex = %d, want local index 1", got)
	}
}

func TestPickAudioSourceIndexHonorsMetadata(t *testing.T) {
	cat := player.AudioCatalog{Sources: []player.AudioSource{
		{ID: "midi", Kind: player.SourceMIDI, Label: "MIDI"},
		{ID: "yt:abc", Kind: player.SourceOnline, Label: "YouTube", VideoID: "abc"},
	}}
	tab := &model.Tab{Metadata: map[string]string{"audio_source": "yt:abc"}}
	if got := pickAudioSourceIndex(tab, cat); got != 1 {
		t.Fatalf("pickAudioSourceIndex = %d, want metadata pick 1", got)
	}
}

func TestViewerNavigationStopsPlayback(t *testing.T) {
	m := NewViewerModel()
	m.tab = &model.Tab{Title: "T", Bars: []model.Bar{{}, {}}}
	m.playing = true
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.playing {
		t.Fatal("j should stop playback before scrolling")
	}
	if m.cursorBar != 1 {
		t.Fatalf("cursorBar=%d want 1", m.cursorBar)
	}
}

func TestTogglePlaybackIgnoresWhileFetching(t *testing.T) {
	m := NewViewerModel()
	m.tab = &model.Tab{Title: "T", Bars: []model.Bar{{Strings: []model.StringLine{{}}}}}
	m.fetchingAudio = true
	if cmd := m.togglePlayback(); cmd != nil {
		t.Fatal("togglePlayback should ignore while audio is downloading")
	}
}

func TestAudioFetchedMsgIgnoresStaleTabID(t *testing.T) {
	m := NewViewerModel()
	m.tab = &model.Tab{Title: "Layla", Artist: "Clapton"}
	m.tabID = 2
	m.selectedSourceIdx = 1
	m.audioCatalog = player.AudioCatalog{
		Sources: []player.AudioSource{
			{ID: "midi", Kind: player.SourceMIDI, Label: "MIDI"},
			{ID: "yt:abc", Kind: player.SourceOnline, Label: "YouTube", VideoID: "abc"},
		},
	}
	updated, _ := m.Update(msgs.AudioFetchedMsg{
		Path:   "/tmp/stale.mp3",
		Artist: "Clapton",
		Title:  "Layla",
		TabID:  99,
	})
	m = updated
	if m.resolvedAudio != "" {
		t.Fatalf("stale tab id should not update viewer, got %q", m.resolvedAudio)
	}
}

func TestSaveTabPrefsCmdAllowsTabPathWithoutID(t *testing.T) {
	m := NewViewerModel()
	m.tab = &model.Tab{Title: "Layla", Artist: "Clapton", Metadata: map[string]string{"audio_source": "yt:abc"}}
	m.tabPath = "online://ug/2563800"
	if cmd := m.saveTabPrefsCmd(); cmd == nil {
		t.Fatal("expected save cmd when tabPath is set")
	}
}

func TestAdjustAudioOffsetPersistsAndRounds(t *testing.T) {
	tab := &model.Tab{Title: "T", Artist: "A", Metadata: map[string]string{}}
	m := NewViewerModel()
	m.LoadTab(tab, "/tmp/off.txt", 42)

	m, _ = m.adjustAudioOffset("]")
	if m.audioOffset != 0.5 {
		t.Fatalf("nudge up: got %v want 0.5", m.audioOffset)
	}
	if got := tab.Metadata[model.MetaKeyAudioOffset]; got != "0.5" {
		t.Fatalf("metadata: got %q want 0.5", got)
	}
	m, _ = m.adjustAudioOffset("[")
	if m.audioOffset != 0 {
		t.Fatalf("nudge down: got %v want 0", m.audioOffset)
	}
	m, _ = m.adjustAudioOffset("}")
	if m.audioOffset != 5 {
		t.Fatalf("big nudge: got %v want 5", m.audioOffset)
	}
	m, _ = m.adjustAudioOffset("o")
	if m.audioOffset != 0 {
		t.Fatalf("reset: got %v want 0", m.audioOffset)
	}
	if got := tab.Metadata[model.MetaKeyAudioOffset]; got != "0.0" {
		t.Fatalf("metadata after reset: got %q want 0.0", got)
	}
}

func TestLoadTabRestoresAudioOffset(t *testing.T) {
	tab := &model.Tab{Title: "T", Artist: "A", Metadata: map[string]string{model.MetaKeyAudioOffset: "2.5"}}
	m := NewViewerModel()
	m.LoadTab(tab, "/tmp/off2.txt", 7)
	if m.audioOffset != 2.5 {
		t.Fatalf("restored offset: got %v want 2.5", m.audioOffset)
	}
}
