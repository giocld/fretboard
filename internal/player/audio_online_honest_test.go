//go:build !noytdlp

package player

import (
	"strings"
	"testing"

	"fretboard/internal/model"
)

// TestSearchOnlineCandidatesReportsFailureWhenAllQueriesFail guards US-9:
// when every search query fails (fake yt-dlp exits 1), the caller must learn
// the real cause instead of a misleading empty success.
func TestSearchOnlineCandidatesReportsFailureWhenAllQueriesFail(t *testing.T) {
	writeFailingYtDlp(t)
	cands, err := SearchOnlineCandidates(&model.Tab{Artist: "Test", Title: "Track"}, 5)
	if err == nil {
		t.Fatal("expected an error when all yt-dlp queries fail")
	}
	if cands != nil {
		t.Fatalf("expected no candidates, got %+v", cands)
	}
}

// TestSearchOnlineCandidatesEmptyWithoutErrorStaysEmpty guards the
// complementary case: a successful search with zero hits stays a plain empty
// result (err == nil), which is what distinguishes "no matching recording"
// from "the search tool is broken".
func TestSearchOnlineCandidatesEmptyWithoutErrorStaysEmpty(t *testing.T) {
	writeEmptyYtDlp(t)
	cands, err := SearchOnlineCandidates(&model.Tab{Artist: "Test", Title: "Track"}, 5)
	if err != nil {
		t.Fatalf("empty playlist must not error, got %v", err)
	}
	if cands != nil {
		t.Fatalf("expected no candidates, got %+v", cands)
	}
}

// TestBuildAudioCatalogSurfacesOnlineSearchError guards US-9 end to end: the
// catalog keeps its local/MIDI sources and carries the online failure.
func TestBuildAudioCatalogSurfacesOnlineSearchError(t *testing.T) {
	writeFailingYtDlp(t)
	tab := &model.Tab{Artist: "Dire Straits", Title: "Sultans of Swing"}
	cat, err := BuildAudioCatalog(tab, "", nil, true)
	if err == nil {
		t.Fatal("BuildAudioCatalog should surface the online search failure")
	}
	if len(cat.Sources) == 0 {
		t.Fatal("catalog must still offer its non-online sources")
	}
	if cat.Sources[0].Kind != SourceMIDI {
		t.Fatalf("first source should be MIDI, got %s", cat.Sources[0].Kind)
	}
	if !strings.Contains(err.Error(), "exit status") && !strings.Contains(err.Error(), "1") {
		t.Fatalf("error should carry the underlying failure, got %v", err)
	}
}
