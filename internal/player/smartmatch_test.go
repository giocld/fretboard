//go:build !noytdlp

package player

import (
	"strings"
	"testing"
	"time"
)

// TestSmartMatchKeywordPenaltyFlipsRanking guards the transformed-audio
// keyword terms: a candidate whose title carries a penalty keyword must rank
// below an otherwise identical clean candidate. The 8D case is the sharpest
// proof — the base scorer only catches the "8d audio" phrase, so the flip is
// attributable to smartMatchScore's bare "8d" term.
func TestSmartMatchKeywordPenaltyFlipsRanking(t *testing.T) {
	tab := songTab("Dire Straits", "Sultans of Swing")
	clean := AudioSource{Score: ScoreYouTubeResult(tab, "Dire Straits - Sultans of Swing", "Some Channel", "", 240), Duration: 4 * time.Minute}
	eightD := AudioSource{Score: ScoreYouTubeResult(tab, "Dire Straits - Sultans of Swing (8D)", "Some Channel", "", 240), Duration: 4 * time.Minute}
	if clean.Score != eightD.Score {
		t.Fatalf("base scores must match for the flip to be attributable: clean=%d 8d=%d", clean.Score, eightD.Score)
	}
	smartMatchScore(tab, &clean, "Dire Straits - Sultans of Swing", "Some Channel")
	smartMatchScore(tab, &eightD, "Dire Straits - Sultans of Swing (8D)", "Some Channel")
	if eightD.Score >= clean.Score {
		t.Fatalf("8D keyword must flip ranking: clean=%d eightD=%d", clean.Score, eightD.Score)
	}
}

// TestSmartMatchKeywordPenalties guards each penalty keyword's weight and
// the cover rule: "cover" is penalized unless the performing channel is the
// tab's own artist. Expected durations land on 240 s, so every candidate
// also collects the full +60 closeness boost; artist channels add +30.
func TestSmartMatchKeywordPenalties(t *testing.T) {
	tab := songTab("Dire Straits", "Sultans of Swing")
	base := AudioSource{Score: 100, Duration: 4 * time.Minute}
	cases := []struct {
		title, channel string
		want           int
	}{
		{"Sultans of Swing", "Some Channel", 100 + 60},
		{"Sultans of Swing (Nightcore)", "Some Channel", 100 - 70 + 60},
		{"Sultans of Swing (Sped Up)", "Some Channel", 100 - 70 + 60},
		{"Sultans of Swing (Slowed)", "Some Channel", 100 - 70 + 60},
		{"Sultans of Swing (8D)", "Some Channel", 100 - 70 + 60},
		{"Sultans of Swing Karaoke", "Some Channel", 100 - 70 + 60},
		{"Sultans of Swing REACTION", "Some Channel", 100 - 70 + 60},
		{"Sultans of Swing (Remix)", "Some Channel", 100 - 70 + 60},
		{"Sultans of Swing - Cover", "Some Guitarist", 100 - 50 + 60},
		{"Sultans of Swing - Cover", "Dire Straits", 100 + 60 + 30}, // artist channel: no cover penalty
		{"Sultans of Swing - Cover (Nightcore)", "Some Guitarist", 100 - 50 - 70 + 60},
	}
	for _, tc := range cases {
		src := base
		smartMatchScore(tab, &src, tc.title, tc.channel)
		if src.Score != tc.want {
			t.Fatalf("%q on %q: score = %d, want %d", tc.title, tc.channel, src.Score, tc.want)
		}
	}
}

// TestSmartMatchDurationClosenessBoost guards the expected-duration term: a
// candidate within ±30% of the tab's expected length gets a boost
// proportional to closeness (up to 60 for an exact match), and candidates
// outside the window get nothing.
func TestSmartMatchDurationClosenessBoost(t *testing.T) {
	tab := songTab("Dire Straits", "Sultans of Swing") // expected 240 s
	exact := AudioSource{Score: 50, Duration: 240 * time.Second}
	close := AudioSource{Score: 50, Duration: 200 * time.Second}  // ratio 0.83 → boost ≈ 50
	far := AudioSource{Score: 50, Duration: 600 * time.Second}    // ratio 2.5 → no boost
	clipped := AudioSource{Score: 50, Duration: 60 * time.Second} // ratio 0.25 → no boost
	smartMatchScore(tab, &exact, "Sultans of Swing", "Some Channel")
	smartMatchScore(tab, &close, "Sultans of Swing", "Some Channel")
	smartMatchScore(tab, &far, "Sultans of Swing", "Some Channel")
	smartMatchScore(tab, &clipped, "Sultans of Swing", "Some Channel")
	if exact.Score != 50+60 {
		t.Fatalf("exact-length candidate should get the full boost, got %d", exact.Score)
	}
	if far.Score != 50 || clipped.Score != 50 {
		t.Fatalf("out-of-window durations must not be boosted: far=%d clipped=%d", far.Score, clipped.Score)
	}
	if !(close.Score > 50 && close.Score < exact.Score) {
		t.Fatalf("closeness boost should be proportional: close=%d exact=%d", close.Score, exact.Score)
	}
}

// TestSmartMatchChannelReputation guards the channel terms: VEVO, Topic, and
// artist-name channels earn the reputation boost; a plain channel does not.
func TestSmartMatchChannelReputation(t *testing.T) {
	tab := songTab("Dire Straits", "Sultans of Swing")
	vevo := AudioSource{Score: 50, Duration: 240 * time.Second}
	topic := AudioSource{Score: 50, Duration: 240 * time.Second}
	artist := AudioSource{Score: 50, Duration: 240 * time.Second}
	other := AudioSource{Score: 50, Duration: 240 * time.Second}
	smartMatchScore(tab, &vevo, "Sultans of Swing", "DireStraitsVEVO")
	smartMatchScore(tab, &topic, "Sultans of Swing", "Dire Straits - Topic")
	smartMatchScore(tab, &artist, "Sultans of Swing", "Dire Straits Official")
	smartMatchScore(tab, &other, "Sultans of Swing", "Random Uploader")
	for name, src := range map[string]AudioSource{"vevo": vevo, "topic": topic, "artist": artist} {
		if src.Score != 50+60+30 {
			t.Fatalf("%s channel should get the reputation boost: %d, want %d", name, src.Score, 50+60+30)
		}
	}
	if other.Score != 50+60 {
		t.Fatalf("plain channel must not be boosted: %d", other.Score)
	}
}

// TestSearchOnlineCandidatesSetsPickReason guards the pick-reason contract
// end to end: the top candidate carries a human-readable reason describing
// the duration match and the official channel.
func TestSearchOnlineCandidatesSetsPickReason(t *testing.T) {
	writeJSONYtDlp(t, `{"entries":[
		{"id":"official1","title":"Dire Straits - Sultans of Swing (Official Audio)","channel":"DireStraitsVEVO","description":"","duration":240},
		{"id":"guitar2","title":"Sultans of Swing - Guitar Lesson","channel":"Some Teacher","description":"","duration":300}
	]}`)
	tab := songTab("Dire Straits", "Sultans of Swing")
	cands, err := SearchOnlineCandidates(tab, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) < 2 {
		t.Fatalf("expected candidates, got %+v", cands)
	}
	top := cands[0]
	if top.ID != "yt:official1" {
		t.Fatalf("official candidate should lead, got %s (%q, score %d)", top.ID, top.Label, top.Score)
	}
	if top.PickReason == "" {
		t.Fatal("top candidate should carry a pick reason")
	}
	if !strings.Contains(top.PickReason, "duration") || !strings.Contains(top.PickReason, "official channel") {
		t.Fatalf("PickReason = %q, want duration + official-channel fragments", top.PickReason)
	}
}
