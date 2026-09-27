// Inspection subcommands: `list` (library contents), `show` and `timing`
// (how a tab or chord sheet is parsed and scheduled), and `chords` (print a
// chord's fingering as a text diagram). These need no interactive TUI and
// exist so a tab's timing can be checked and debugged from the command
// line -- the same schedule the player and the viewer's cursor use.
package cli

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"

	"fretboard/internal/library"
	"fretboard/internal/model"
	"fretboard/internal/parser"
	"fretboard/internal/player"
)

// runList prints every tab in the library: id, title, artist, tuning, kind,
// play count, and status/favorite flags, so a specific tab's id (needed by
// `show`/`timing`) can be found without opening the TUI.
func runList(store *library.Store, stdout, stderr io.Writer) int {
	rows, err := store.List()
	if err != nil {
		fmt.Fprintf(stderr, "list: %v\n", err)
		return 1
	}
	if len(rows) == 0 {
		fmt.Fprintln(stdout, "library is empty — import a tab with: fretboard import <file>")
		return 0
	}
	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tTITLE\tARTIST\tTUNING\tPLAYS\tFLAGS")
	for _, r := range rows {
		var flags []string
		if r.Favorite {
			flags = append(flags, "fav")
		}
		if r.Status != "" {
			flags = append(flags, r.Status)
		}
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%d\t%s\n", r.ID, r.Title, r.Artist, library.TuningLabel(r.Tuning), r.PlayCount, strings.Join(flags, ","))
	}
	tw.Flush()
	return 0
}

// resolveTab loads a tab by library id (a bare integer) or by file path,
// trying the library first so a numeric filename is not mistaken for an id.
func resolveTab(store *library.Store, arg string) (*model.Tab, string, error) {
	if id, err := strconv.ParseInt(arg, 10, 64); err == nil {
		if tab, err := store.Get(id); err == nil {
			row, _ := store.GetRow(id)
			label := arg
			if row != nil {
				label = row.Filepath
			}
			return tab, label, nil
		}
	}
	tab, err := parser.ParsePath(arg)
	if err == nil && len(tab.Bars) == 0 && tab.Metadata["kind"] != "chords" {
		// An empty or prose file parses without error but has nothing to
		// show; a chord sheet is legitimately bar-less and keeps going.
		err = fmt.Errorf("%s: no tab bars or chord lines found", arg)
	}
	return tab, arg, err
}

// runShow prints a summary of how a tab was parsed and will be scheduled:
// title/artist, tuning and capo, kind (tab or chord sheet), bar count,
// tempo, and the resulting length -- the same numbers the player and the
// viewer's status line use, without opening the TUI.
func runShow(store *library.Store, args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: fretboard show <id|file>")
		return 1
	}
	tab, label, err := resolveTab(store, args[0])
	if err != nil {
		fmt.Fprintf(stderr, "show: %v\n", err)
		return 1
	}
	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "source\t%s\n", label)
	fmt.Fprintf(tw, "title\t%s\n", tab.Title)
	fmt.Fprintf(tw, "artist\t%s\n", tab.Artist)
	if kind := tab.Metadata["kind"]; kind == "chords" {
		fmt.Fprintf(tw, "kind\tchord sheet (no playable bars)\n")
		lines := strings.Split(tab.Metadata["raw"], "\n")
		fmt.Fprintf(tw, "lines\t%d\n", len(lines))
		tw.Flush()
		return 0
	}
	tuning := "?"
	if tab.Tuning != nil {
		tuning = tab.Tuning.Label()
	}
	if capo, _ := strconv.Atoi(tab.Metadata[model.MetaKeyCapo]); capo > 0 {
		tuning += fmt.Sprintf(" (capo %d)", capo)
	}
	fmt.Fprintf(tw, "tuning\t%s\n", tuning)
	fmt.Fprintf(tw, "bars\t%d\n", len(tab.Bars))
	bpm := player.TabBPM(tab)
	fmt.Fprintf(tw, "tempo\t%d BPM\n", bpm)
	unit, perMeasure := player.ColumnGrid(tab)
	if perMeasure > 0 {
		fmt.Fprintf(tw, "spacing\t%d cols/note, %d cols/measure\n", unit, perMeasure)
	}
	order := player.RepeatOrder(tab)
	fmt.Fprintf(tw, "playback order\t%d bar-visits (from %d bars on the page)\n", len(order), len(tab.Bars))
	secs := player.ScheduleDurationSeconds(tab, bpm)
	fmt.Fprintf(tw, "length\t%d:%02d\n", int(secs)/60, int(secs)%60)
	tw.Flush()
	return 0
}

// runTiming prints, per bar, how BuildSchedule times it: total ticks in
// measures, and each note's onset as a beat.sub count (1.1 = beat 1, 1.2 =
// its second sixteenth), the same breakdown used to debug the ASCII
// spacing-to-measures quantization. An optional "from-to" bar range
// (1-based, inclusive) limits the output.
func runTiming(store *library.Store, args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 || len(args) > 2 {
		fmt.Fprintln(stderr, "usage: fretboard timing <id|file> [from-to]")
		return 1
	}
	tab, _, err := resolveTab(store, args[0])
	if err != nil {
		fmt.Fprintf(stderr, "timing: %v\n", err)
		return 1
	}
	if tab.Metadata["kind"] == "chords" {
		fmt.Fprintln(stderr, "timing: chord sheet has no playable bars")
		return 1
	}
	from, to := 1, len(tab.Bars)
	if len(args) == 2 {
		if from, to, err = parseBarRange(args[1], len(tab.Bars)); err != nil {
			fmt.Fprintf(stderr, "timing: %v\n", err)
			return 1
		}
	}
	perBar := player.BarSteps(tab)
	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "BAR\tMEASURES\tONSETS (beat.sub)")
	const ticksPerMeasure = 4 * 480 // 4/4 at 480 ticks per quarter
	for b := from - 1; b < to && b < len(perBar); b++ {
		steps := perBar[b]
		var total int64
		var onsets []string
		var acc int64
		for _, s := range steps {
			if !s.Rest {
				onsets = append(onsets, beatLabel(acc))
			}
			acc += int64(s.Ticks)
			total += int64(s.Ticks)
		}
		measures := float64(total) / ticksPerMeasure
		fmt.Fprintf(tw, "%d\t%.2fm\t%s\n", b+1, measures, strings.Join(onsets, " "))
	}
	tw.Flush()
	return 0
}

// beatLabel renders a tick offset within a measure as "beat.sixteenth",
// e.g. tick 0 -> "1.1", tick 120 -> "1.2" (the second sixteenth of beat 1).
func beatLabel(tick int64) string {
	const sixteenth = 120
	beat := tick/480 + 1
	sub := (tick%480)/sixteenth + 1
	return fmt.Sprintf("%d.%d", beat, sub)
}

// parseBarRange parses "N" or "N-M" (1-based, inclusive) against a tab of
// barCount bars.
func parseBarRange(s string, barCount int) (from, to int, err error) {
	parts := strings.SplitN(s, "-", 2)
	from, err = strconv.Atoi(parts[0])
	if err != nil || from < 1 {
		return 0, 0, fmt.Errorf("invalid bar range %q", s)
	}
	to = from
	if len(parts) == 2 {
		to, err = strconv.Atoi(parts[1])
		if err != nil || to < from {
			return 0, 0, fmt.Errorf("invalid bar range %q", s)
		}
	}
	if to > barCount {
		to = barCount
	}
	return from, to, nil
}

// runChords prints a text fretboard diagram for each chord name given,
// e.g. "fretboard chords Am7 C G/B". Unrecognized chord names are skipped
// with a note instead of failing the whole command.
func runChords(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: fretboard chords <name...>")
		return 1
	}
	ok := true
	for i, name := range args {
		if i > 0 {
			fmt.Fprintln(stdout)
		}
		c, parsed := parser.ParseChord(name)
		if !parsed {
			fmt.Fprintf(stderr, "%s: not a recognized chord name\n", name)
			ok = false
			continue
		}
		shape := c.FretShape()
		if shape == ([6]int{-1, -1, -1, -1, -1, -1}) {
			fmt.Fprintf(stderr, "%s: no diagram available for this quality\n", c.String())
			ok = false
			continue
		}
		fmt.Fprintln(stdout, chordDiagram(c.String(), shape))
	}
	if !ok {
		return 1
	}
	return 0
}

// chordDiagram renders a fret-shape as a text diagram, low string (E) on
// the left, high string (e) on the right, muted strings marked "x", open
// strings marked "0". Shapes that fit in the first four frets are drawn from
// fret 1 like a chord book; higher barre shapes are drawn from their lowest
// fretted fret, with that fret number labeled on the left.
func chordDiagram(name string, shape [6]int) string {
	lowest, highest := 25, 0
	for _, f := range shape {
		if f > 0 {
			if f < lowest {
				lowest = f
			}
			if f > highest {
				highest = f
			}
		}
	}
	base := 1
	if highest > 4 {
		base = lowest
	}
	rows := 4
	if highest-base+1 > rows {
		rows = highest - base + 1 // e.g. sus2 barres span five frets
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", name)
	fmt.Fprintln(&b, "  "+markerRow(shape))
	for fret := base; fret < base+rows; fret++ {
		row := make([]byte, 6)
		for i, f := range shape {
			if f == fret {
				row[i] = '#'
			} else {
				row[i] = '-'
			}
		}
		label := "  "
		if fret == base && base > 1 {
			label = fmt.Sprintf("%2d", base)
		}
		fmt.Fprintf(&b, "%s|%s|\n", label, joinFrets(row))
	}
	return strings.TrimRight(b.String(), "\n")
}

func markerRow(shape [6]int) string {
	marks := make([]byte, 6)
	for i, f := range shape {
		switch {
		case f < 0:
			marks[i] = 'x'
		case f == 0:
			marks[i] = 'o'
		default:
			marks[i] = ' '
		}
	}
	return joinFrets(marks)
}

func joinFrets(b []byte) string {
	out := make([]byte, 0, len(b)*2-1)
	for i, c := range b {
		if i > 0 {
			out = append(out, ' ')
		}
		out = append(out, c)
	}
	return string(out)
}
