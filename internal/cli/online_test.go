package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"fretboard/internal/model"
	"fretboard/internal/parser"
	"fretboard/internal/scraper"
)

// fakeOnline stands in for scraper.Client: no network, canned results, and a
// record of what was fetched.
type fakeOnline struct {
	results []scraper.SearchResult
	tab     *model.Tab
	err     error
	fetched []scraper.SearchResult
}

func (f *fakeOnline) Search(string) ([]scraper.SearchResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.results, nil
}

func (f *fakeOnline) Fetch(r scraper.SearchResult) (*model.Tab, error) {
	f.fetched = append(f.fetched, r)
	if f.err != nil {
		return nil, f.err
	}
	return f.tab, nil
}

// TestRunSearchPrintsResults: the search table carries the ids and source
// names `fretboard fetch` needs.
func TestRunSearchPrintsResults(t *testing.T) {
	client := &fakeOnline{results: []scraper.SearchResult{
		{Source: scraper.SourceUG, ID: 24697, Type: "Tabs", Rating: 4.9, SongName: "Enter Sandman", ArtistName: "Metallica"},
		{Source: scraper.SourceSongsterr, ID: 12, Type: "Chords", SongName: "Enter Sandman", ArtistName: "Metallica"},
	}}
	var out, errBuf bytes.Buffer
	if code := runSearch([]string{"enter", "sandman"}, client, &out, &errBuf); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, errBuf.String())
	}
	stdout := out.String()
	for _, want := range []string{"SOURCE", "24697", "ug", "songsterr", "Metallica", "fretboard fetch <id>"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("search output missing %q:\n%s", want, stdout)
		}
	}

	out.Reset()
	if code := runSearch(nil, client, &out, &errBuf); code != 1 || !strings.Contains(errBuf.String(), "usage: fretboard search") {
		t.Fatalf("no-args search: code=%d stderr=%q", code, errBuf.String())
	}
}

// TestRunFetchSavesAndUpdates: fetching imports under the TUI's online path;
// a second fetch of the same tab (here by URL) updates that entry instead of
// duplicating it.
func TestRunFetchSavesAndUpdates(t *testing.T) {
	withConfigDir(t, func(dir string) {
		store, err := openStore()
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		sheet, err := parser.Parse(strings.NewReader("Am  C\nsome lyric line\n"))
		if err != nil {
			t.Fatal(err)
		}
		client := &fakeOnline{tab: sheet}

		var out, errBuf bytes.Buffer
		if code := runFetch(store, []string{"24697"}, client, &out, &errBuf); code != 0 {
			t.Fatalf("code=%d stderr=%q", code, errBuf.String())
		}
		if got := out.String(); !strings.Contains(got, "saved #1") || !strings.Contains(got, "chord sheet, 2 bars") {
			t.Fatalf("fetch report = %q", got)
		}

		out.Reset()
		url := "https://tabs.ultimate-guitar.com/tab/metallica/enter-sandman-24697"
		if code := runFetch(store, []string{url}, client, &out, &errBuf); code != 0 {
			t.Fatalf("url fetch: code=%d stderr=%q", code, errBuf.String())
		}
		if got := out.String(); !strings.Contains(got, "updated #1") {
			t.Fatalf("second fetch should update: %q", got)
		}
		if len(client.fetched) != 2 || client.fetched[1].ID != 24697 || client.fetched[1].TabURL != url {
			t.Fatalf("client saw %+v", client.fetched)
		}
		rows, err := store.List()
		if err != nil || len(rows) != 1 {
			t.Fatalf("library rows = %d (%v), want the one updated entry", len(rows), err)
		}
		if rows[0].Filepath != "online://ug/24697" {
			t.Fatalf("stored path = %q, want the online key", rows[0].Filepath)
		}
	})
}

// TestRunFetchErrors: bad invocations fail before any network call.
func TestRunFetchErrors(t *testing.T) {
	withConfigDir(t, func(dir string) {
		store, err := openStore()
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		client := &fakeOnline{}

		var out, errBuf bytes.Buffer
		if code := runFetch(store, nil, client, &out, &errBuf); code != 1 || !strings.Contains(errBuf.String(), "usage: fretboard fetch") {
			t.Fatalf("no-args fetch: code=%d stderr=%q", code, errBuf.String())
		}
		errBuf.Reset()
		if code := runFetch(store, []string{"not-an-id"}, client, &out, &errBuf); code != 1 || !strings.Contains(errBuf.String(), "usage: fretboard fetch") {
			t.Fatalf("bad id: code=%d stderr=%q", code, errBuf.String())
		}
		errBuf.Reset()
		if code := runFetch(store, []string{"https://example.com/no-id-here/"}, client, &out, &errBuf); code != 1 || !strings.Contains(errBuf.String(), "no tab id") {
			t.Fatalf("bad url: code=%d stderr=%q", code, errBuf.String())
		}
		if len(client.fetched) != 0 {
			t.Fatalf("bad invocations must not fetch: %+v", client.fetched)
		}

		// A backend failure is reported as fetch:, not a raw error.
		client.err = errFake
		errBuf.Reset()
		if code := runFetch(store, []string{"1"}, client, &out, &errBuf); code != 1 || !strings.Contains(errBuf.String(), "fetch: boom") {
			t.Fatalf("backend error: code=%d stderr=%q", code, errBuf.String())
		}
	})
}

var errFake = errors.New("boom")

// TestRunFetchURLIgnoresQueryDigits: the tab id is the last path segment, not
// a number trailing the query string.
func TestRunFetchURLIgnoresQueryDigits(t *testing.T) {
	withConfigDir(t, func(dir string) {
		store, err := openStore()
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		tab, err := parser.Parse(strings.NewReader("Am  C\nsome lyric line\n"))
		if err != nil {
			t.Fatal(err)
		}
		client := &fakeOnline{tab: tab}
		var out, errBuf bytes.Buffer
		url := "https://tabs.ultimate-guitar.com/tab/metallica/enter-sandman-24697?utm_source=copy&utm_id=2"
		if code := runFetch(store, []string{url}, client, &out, &errBuf); code != 0 {
			t.Fatalf("code=%d stderr=%q", code, errBuf.String())
		}
		if len(client.fetched) != 1 || client.fetched[0].ID != 24697 {
			t.Fatalf("client saw %+v, want id 24697", client.fetched)
		}
	})
}
