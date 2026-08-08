package player

import (
	"testing"

	"fretboard/internal/model"
)

// TestClassifyAudioCandidate covers the performance-type taxonomy that strict
// audio selection is built on: official recordings must be told apart from
// live shows, covers, lessons, and backing tracks.
func TestClassifyAudioCandidate(t *testing.T) {
	cases := []struct {
		artist, song, title, channel, desc string
		want                               AudioCategory
	}{
		{"Dire Straits", "Sultans of Swing", "Sultans of Swing (Official Audio)", "Dire Straits", "", CatOfficial},
		{"Dire Straits", "Sultans of Swing", "Dire Straits - Sultans of Swing", "DireStraitsVEVO", "", CatOfficial},
		{"Dire Straits", "Sultans of Swing", "Sultans of Swing", "Dire Straits", "", CatOfficial},
		{"Dire Straits", "Sultans of Swing", "Sultans of Swing (Live at Wembley)", "Dire Straits", "", CatLive},
		{"Dire Straits", "Sultans of Swing", "Sultans of Swing Live 1985", "SomeUploader", "", CatLive},
		{"Dire Straits", "Sultans of Swing", "Sultans of Swing - Cover", "Beginner Guitarist", "", CatCover},
		{"Dire Straits", "Sultans of Swing", "Sultans of Swing guitar lesson", "Guitar Teacher", "", CatLesson},
		{"Dire Straits", "Sultans of Swing", "How to play Sultans of Swing on guitar", "TabsMaster", "", CatLesson},
		{"Dire Straits", "Sultans of Swing", "Sultans of Swing Backing Track", "Jam Tracks", "", CatBacking},
		{"Dire Straits", "Sultans of Swing", "Sultans of Swing (Karaoke)", "Karaoke World", "", CatBacking},
		{"Dire Straits", "Sultans of Swing", "Morning Coffee Jazz Mix", "Lofi Beats", "", CatOther},
	}
	for _, c := range cases {
		if got := ClassifyAudioCandidate(c.artist, c.song, c.title, c.channel, c.desc); got != c.want {
			t.Errorf("ClassifyAudioCandidate(%q) = %s, want %s", c.title, got, c.want)
		}
	}
}

func TestStrictCompatible(t *testing.T) {
	for cat, want := range map[AudioCategory]bool{
		CatOfficial: true,
		CatBacking:  true,
		CatLocal:    true,
		CatLive:     false,
		CatCover:    false,
		CatLesson:   false,
		CatOther:    false,
	} {
		if got := StrictCompatible(cat); got != want {
			t.Errorf("StrictCompatible(%s) = %v, want %v", cat, got, want)
		}
	}
}

// TestScoreYouTubeResultPrefersStudio guards the strict-scoring priority: an
// official recording must always outrank live/cover/lesson versions of the
// same song, regardless of ordering.
func TestScoreYouTubeResultPrefersStudio(t *testing.T) {
	tab := &model.Tab{Artist: "Dire Straits", Title: "Sultans of Swing"}
	official := ScoreYouTubeResult(tab, "Sultans of Swing (Official Audio)", "Dire Straits", "", 340)
	live := ScoreYouTubeResult(tab, "Sultans of Swing (Live at Wembley)", "Dire Straits", "", 560)
	cover := ScoreYouTubeResult(tab, "Sultans of Swing - Cover", "Some Guitarist", "", 300)
	lesson := ScoreYouTubeResult(tab, "Sultans of Swing guitar lesson", "Guitar Teacher", "", 900)
	if official <= live || official <= cover || official <= lesson {
		t.Fatalf("official (%d) must outrank live (%d), cover (%d), lesson (%d)", official, live, cover, lesson)
	}
	if lesson > 0 {
		t.Fatalf("lesson scoring should be strongly negative, got %d", lesson)
	}
}

// TestAudioCatalogHasStrictRejected guards the "no studio match" signal used
// to warn the user when strict auto-pick fell back to MIDI.
func TestAudioCatalogHasStrictRejected(t *testing.T) {
	cat := AudioCatalog{Sources: []AudioSource{
		{ID: "midi", Kind: SourceMIDI},
		{ID: "yt:1", Kind: SourceOnline, Category: CatLive, StrictOK: false},
	}}
	if !cat.HasStrictRejected() {
		t.Fatal("live candidate should count as strict-rejected")
	}
	cat = AudioCatalog{Sources: []AudioSource{
		{ID: "midi", Kind: SourceMIDI},
		{ID: "yt:1", Kind: SourceOnline, Category: CatOfficial, StrictOK: true},
	}}
	if cat.HasStrictRejected() {
		t.Fatal("official candidate should not be strict-rejected")
	}
}
