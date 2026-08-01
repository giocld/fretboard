### WHAT THIS PACKAGE DOES
Reads a raw text file and produces a structured `model.Tab`. The parser is
the hardest piece — it has to be forgiving because ASCII tabs are messy.

### FILE: ascii.go

#### WHAT IT DOES
Implements `Parse(reader io.Reader) (*model.Tab, error)`.

#### HOW TO THINK ABOUT IT
This is a two-pass algorithm:
- Pass 1: Identify WHERE the tablature region is in the file. Separate metadata from tab.
- Pass 2: Parse the tab region line by line, character by character, extracting notes.

Don't try to parse everything perfectly. Guitar tabs are user-generated and wildly
inconsistent. Start with the commonest format (artist → title → tuning → bars with | delimiters)
and add tolerance gradually.

#### STEP-BY-STEP — PASS 1 (Structural)

1. Read all lines with `bufio.Scanner`.
2. Filter out empty lines and comment lines (starting with `//` or `#`).
3. For the first few lines, try to detect:
   - Artist line (often line 1)
   - Title line (often line 2)
   - Tuning line (contains note names or "Tuning:" prefix)
   - Capo line (contains "Capo" followed by a number)
4. Find where the tab starts. Heuristic: the first line that has mostly
   hyphens and digits (i.e., count non-whitespace chars; if >50% are 0-9 or '-',
   it's probably a tab string line).
5. The tab section continues until lines stop looking like string lines.

#### STEP-BY-STEP — PASS 2 (Note Extraction)

For each group of N consecutive string lines (N = len(tuning), typically 6):

1. **Detect bar boundaries:** Find all `|` characters. Each pair of `|`s
   defines a bar. The content between them is the bar's tab data.

2. **Split into bars:** For each `|` to `|` span, slice the substring.
   That's one bar's worth of raw text for that string line.

3. **Extract segments per bar per string:** Walk the substring character by
   character. Track column position. For each char:
   - '-' → Segment{'-', pos} (rest)
   - '0'-'9' → Segment{digit, pos} (fret number — NOTE: "10" is two chars!)
   - 'h','p','b','/','\\','~','x' → Segment{char, pos} (technique)
   - ' ' → skip (spacing)
   - '|' → skip (already handled)

4. **Multi-digit fret numbers:** Track state. When you hit a digit after
   another digit, combine them: "1" + "0" = 10. Store the combined value
   somehow (you could use Position for the first digit only and a special
   marker, or store the full number as a string in Char temporarily).

   SIMPLER APPROACH: Ignore multi-digit frets for MVP. Most tabs are single-digit.
   Multi-digit is advanced.

5. **Chord detection:** Not needed for parsing. You'll detect chords at render
   time by checking which Segments share the same Position across strings.

#### PSEUDO-CODE

func Parse(r io.Reader) (*model.Tab, error) {
    lines := readAllLines(r)
    lines = removeEmpty(lines)

    meta := extractMetadata(lines)
    tuning := parseTuning(meta["tuning"])
    tabLines := extractTabLines(lines)  // the N string lines

    var bars []model.Bar
    barNum := 1

    // For each set of N lines (one per string)
    for i := 0; i < len(tabLines); i += len(tuning) {
        batch := tabLines[i : i+len(tuning)]
        // All N lines in batch have the same bar structure (same | positions)
        barDelimiters := findPipePositions(batch[0])

        for j := 0; j < len(barDelimiters)-1; j++ {
            start := barDelimiters[j] + 1
            end := barDelimiters[j+1]
            bar := model.Bar{Number: barNum, Strings: make([]model.StringLine, len(tuning))}
            for s := 0; s < len(tuning); s++ {
                substring := batch[s][start:end]  // content between two |s
                bar.Strings[s] = parseBarContent(substring)
            }
            bars = append(bars, bar)
            barNum++
        }
    }

    return &model.Tab{...}, nil
}

func parseBarContent(s string) model.StringLine {
    var segs []model.Segment
    pos := 0
    for _, ch := range s {
        switch {
        case ch == '-':
            segs = append(segs, model.Segment{Char: '-', Position: pos})
        case ch >= '0' && ch <= '9':
            segs = append(segs, model.Segment{Char: ch, Position: pos})
        case strings.ContainsRune("hpb/\\~x", ch):
            segs = append(segs, model.Segment{Char: ch, Position: pos})
        }
        pos++
    }
    return model.StringLine{Segments: segs}
}

#### GO CONCEPTS
- `bufio.Scanner` for line-by-line reading.
- `strings.Builder` if you need to accumulate chars.
- `bytes` package if you're working with byte slices instead of strings.
- Rune iteration: `for i, r := range s` — i is byte index, r is rune.
  Be careful: `s[i]` gives a byte, not a rune! Always use `r` from the range.

#### GOTCHAS
- **Multi-byte characters:** Some tabs use Unicode chars (e.g., fancy bar lines,
  rhythm markers). A bar like `│` is 3 bytes in UTF-8. Iterate by rune, not byte index.
- **Inconsistent bar count:** Some tabs have bar numbers written inline.
  Others have no bars at all. For no-bar tabs, treat the whole line as one bar.
- **Extra whitespace:** Leading spaces, trailing spaces, tabs vs spaces.
  Trim lines before processing.
- **Wrapped lines:** Tabs wider than ~80 chars may be hard-wrapped by the author.
  Lines that start mid-bar. Detect consecutive string lines with no `|` at column 0.
- **Empty bars:** Two consecutive `||` with nothing between. Create an empty Bar with
  no segments (or all rests).

#### IF STUCK
- Go by Example: "Reading Files" and "String Functions"
- "golang bufio scanner example" — essential for line-by-line reading
- "golang rune iteration utf8" — critical for handling non-ASCII chars
- Read: https://go.dev/blog/strings — understanding strings, bytes, runes
- Test on real tabs from Ultimate-Guitar. Open 5 different tabs and compare
  your parsed output to what you see visually.

### FILE: ascii_test.go

#### WHAT IT DOES
Table-driven tests against real-world tab files stored in testdata/.

#### HOW TO BUILD IT
1. Download 5 tabs from Ultimate-Guitar: standard format, drop D, capo, no-bar-lines, multi-page.
2. Save them in `testdata/`.
3. For each, write a test that calls `Parse`, checks:
   - Number of bars (ballpark — don't assert exact, tabs are messy)
   - Tuning detected correctly
   - First bar has the right fret numbers in the right places
4. Test edge cases: empty file, no tab region found, corrupted tab.

#### GO CONCEPTS
- `testing` package basics.
- Table-driven tests pattern: `var tests = []struct{name, input, want}{...}`.
- `t.Run()` for sub-tests.
- `os.ReadFile` in tests.
- `t.Fatal` vs `t.Error` — Fatal stops the test, Error continues.

#### SKELETON

func TestParse(t *testing.T) {
    tests := []struct {
        name     string
        file     string
        wantBars int // minimum expected bars
    }{
        {"standard", "testdata/sultans.txt", 8},
        {"drop_d", "testdata/everlong.txt", 12},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            data, err := os.ReadFile(tt.file)
            if err != nil { t.Fatal(err) }
            tab, err := Parse(bytes.NewReader(data))
            if err != nil { t.Fatal(err) }
            if len(tab.Bars) < tt.wantBars {
                t.Errorf("got %d bars, want at least %d", len(tab.Bars), tt.wantBars)
            }
        })
    }
}
