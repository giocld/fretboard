// Online subcommands: `search` (query the tab sites from the shell) and
// `fetch` (save a result into the library, exactly as the TUI search screen
// does, so a re-fetch updates the same entry).
package cli

import (
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"text/tabwriter"

	"fretboard/internal/library"
	"fretboard/internal/model"
	"fretboard/internal/scraper"
)

// onlineClient is the subset of scraper.Client the online subcommands need,
// so tests can substitute a fake without a network round trip.
type onlineClient interface {
	Search(query string) ([]scraper.SearchResult, error)
	Fetch(result scraper.SearchResult) (*model.Tab, error)
}

// runSearch prints the merged search results, best-first, with the ids that
// `fretboard fetch` accepts.
func runSearch(args []string, client onlineClient, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: fretboard search <query...>")
		return 1
	}
	results, err := client.Search(strings.Join(args, " "))
	if err != nil {
		fmt.Fprintf(stderr, "search: %v\n", err)
		return 1
	}
	if len(results) == 0 {
		fmt.Fprintln(stdout, "no results")
		return 0
	}
	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "SOURCE\tID\tTYPE\tRATING\tSONG\tARTIST")
	for _, r := range results {
		fmt.Fprintf(tw, "%s\t%d\t%s\t%.1f\t%s\t%s\n", r.Source, r.ID, clip(r.Type, 12), r.Rating, clip(r.SongName, 40), clip(r.ArtistName, 24))
	}
	tw.Flush()
	fmt.Fprintln(stdout, "\nSave an Ultimate Guitar result with: fretboard fetch <id>")
	return 0
}

// ugURLID pulls the tab id off the end of an Ultimate Guitar URL
// (…/tabs/24697/enter-sandman or …/24697).
var ugURLID = regexp.MustCompile(`(\d+)/?$`)

// runFetch retrieves an Ultimate Guitar tab or chord sheet by id or URL and
// imports it under the same library path the TUI uses, so fetching again
// updates the entry.
func runFetch(store *library.Store, args []string, client onlineClient, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: fretboard fetch <ug-id|ug-url>")
		return 1
	}
	r := scraper.SearchResult{Source: scraper.SourceUG}
	if strings.HasPrefix(args[0], "http") {
		// A pasted URL can carry a query string or fragment; the tab id is
		// the last path segment, not whatever digits trail the parameters.
		u := args[0]
		if i := strings.IndexAny(u, "?#"); i >= 0 {
			u = u[:i]
		}
		m := ugURLID.FindStringSubmatch(u)
		if m == nil {
			fmt.Fprintf(stderr, "fetch: no tab id at the end of %s\n", args[0])
			return 1
		}
		r.TabURL = args[0]
		r.ID, _ = strconv.ParseInt(m[1], 10, 64)
	} else {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			fmt.Fprintln(stderr, "usage: fretboard fetch <ug-id|ug-url>")
			return 1
		}
		r.ID = id
	}
	tab, err := client.Fetch(r)
	if err != nil {
		if r.TabURL == "" {
			fmt.Fprintf(stderr, "fetch: %v (if the id is right, pass the tab's full UG url instead)\n", err)
		} else {
			fmt.Fprintf(stderr, "fetch: %v\n", err)
		}
		return 1
	}
	before, _ := store.List()
	id, err := store.Import(scraper.LibraryPath(r), tab)
	if err != nil {
		fmt.Fprintf(stderr, "fetch: %v\n", err)
		return 1
	}
	verb := "saved"
	for _, row := range before {
		if row.ID == id {
			verb = "updated"
			break
		}
	}
	kind := "tab"
	if tab.Metadata["kind"] == "chords" {
		kind = "chord sheet"
	}
	fmt.Fprintf(stdout, "%s #%d %s (%s, %d bars)\n", verb, id, tabTitle(tab), kind, len(tab.Bars))
	return 0
}

// tabTitle renders "Title — Artist" for a fetch report.
func tabTitle(tab *model.Tab) string {
	if tab == nil {
		return "?"
	}
	if tab.Artist != "" {
		return tab.Title + " — " + tab.Artist
	}
	return tab.Title
}

// clip shortens a table cell to n runes, marking the cut with an ellipsis.
func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}
