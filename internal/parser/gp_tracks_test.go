package parser

import (
	"testing"
)

// Hand-written --all envelope: song-level title/artist plus one track per
// GP track (name/instrument/strings/tuning/key/bars).
const allTracksJSON = `{
	"title": "Test Song",
	"artist": "Test Artist",
	"tracks": [{
		"name": "Guitar 1",
		"instrument": "Steel String Guitar",
		"strings": 6,
		"tuning": [64, 59, 55, 50, 45, 40],
		"key": "C major",
		"bars": [{
			"number": 1,
			"column_ticks": [480, 240],
			"strings": [{
				"segments": [
					{"char": "0", "value": 0, "position": 0, "width": 1},
					{"char": "3", "value": 3, "position": 4, "width": 1}
				]
			}]
		}]
	}, {
		"name": "Bass",
		"instrument": "Fingered Bass",
		"strings": 4,
		"tuning": [43, 38, 33, 28],
		"key": "C major",
		"bars": []
	}]
}`

// The first track of --all output must decode into a full Tab (same shape
// as the single-track path) while the remaining tracks stay metadata-only.
func TestDecodeGPTracksJSON(t *testing.T) {
	tracks, err := decodeGPTracksJSON([]byte(allTracksJSON))
	if err != nil {
		t.Fatalf("decodeGPTracksJSON: %v", err)
	}
	if len(tracks) != 2 {
		t.Fatalf("expected 2 tracks, got %d", len(tracks))
	}

	first := tracks[0]
	if first.Name != "Guitar 1" || first.Instrument != "Steel String Guitar" {
		t.Fatalf("unexpected first track metadata: %+v", first)
	}
	if first.Strings != 6 {
		t.Fatalf("Strings = %d, want 6", first.Strings)
	}
	if first.Tuning != "EBGDAE" {
		t.Fatalf("Tuning = %q, want EBGDAE", first.Tuning)
	}
	if first.Tab == nil {
		t.Fatal("first track Tab is nil; want full tab")
	}
	if first.Tab.Title != "Test Song" || first.Tab.Artist != "Test Artist" {
		t.Fatalf("tab metadata: %+v", first.Tab)
	}
	if len(first.Tab.Bars) != 1 || len(first.Tab.Bars[0].ColumnTicks) != 2 {
		t.Fatalf("tab bars not decoded: %+v", first.Tab.Bars)
	}
	if first.Tab.Metadata == nil {
		t.Fatal("tab Metadata is nil; want non-nil map")
	}

	second := tracks[1]
	if second.Name != "Bass" || second.Instrument != "Fingered Bass" {
		t.Fatalf("unexpected second track metadata: %+v", second)
	}
	if second.Strings != 4 || second.Tuning != "GDAE" {
		t.Fatalf("second track: %+v", second)
	}
	if second.Tab != nil {
		t.Fatal("second track Tab is not nil; want metadata only")
	}
}

func TestDecodeGPTracksJSONNoTracks(t *testing.T) {
	if _, err := decodeGPTracksJSON([]byte(`{"title":"T","artist":"A","tracks":[]}`)); err == nil {
		t.Fatal("expected error for empty tracks array")
	}
}

// A single-track --all payload must still produce a usable Tab.
func TestDecodeGPTracksJSONSingleTrack(t *testing.T) {
	const raw = `{"title":"Solo","artist":"A","tracks":[{"name":"Gtr","instrument":"Clean Guitar","strings":6,"tuning":[64,59,55,50,45,40],"key":"G major","bars":[]}]}`
	tracks, err := decodeGPTracksJSON([]byte(raw))
	if err != nil {
		t.Fatalf("decodeGPTracksJSON: %v", err)
	}
	if len(tracks) != 1 {
		t.Fatalf("expected 1 track, got %d", len(tracks))
	}
	if tracks[0].Tab == nil || tracks[0].Tab.Title != "Solo" {
		t.Fatalf("single track not decoded: %+v", tracks[0])
	}
}

// GPTrack.Tuning must degrade to an empty string for tunings with 0 MIDI
// values (e.g. percussion tracks) rather than producing garbage.
func TestDecodeGPTracksJSONPercussionTuning(t *testing.T) {
	const raw = `{"title":"T","artist":"A","tracks":[{"name":"Drums","instrument":"Fingered Bass","strings":7,"tuning":[0,0,0,0,0,0,0],"key":"C major","bars":[]}]}`
	tracks, err := decodeGPTracksJSON([]byte(raw))
	if err != nil {
		t.Fatalf("decodeGPTracksJSON: %v", err)
	}
	if tracks[0].Tuning != "" {
		t.Fatalf("Tuning = %q, want empty for percussion", tracks[0].Tuning)
	}
	// Zero MIDI tunings are preserved as-is (same as the single-track path);
	// only a fully empty tuning list falls back to standard.
	if len(tracks[0].Tab.Tuning) != 7 {
		t.Fatalf("tab tuning = %v, want 7 zero values preserved", tracks[0].Tab.Tuning)
	}
}
