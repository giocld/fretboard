### WHAT THIS FILE DOES
Entry point. Parses CLI flags, decides which screen to show (library browser
or direct tab viewer), initializes the database, and hands control to Bubble Tea.

### HOW TO THINK ABOUT IT
Your main() function is a traffic cop. It figures out what the user wants
("fretboard" with no args = library, "fretboard song.txt" = viewer) and
delegates to the right TUI model. Keep it thin — real logic lives in internal/.

### STEP-BY-STEP BUILD
1. Use `flag.Parse()` or check `os.Args[1:]` to detect a file path.
2. Open/init the SQLite DB in a known location (e.g. `~/.local/share/fretboard/fretboard.db`).
3. If a file path was given, parse it, create a ViewerModel, pass it to tea.NewProgram().
4. If no args, create a LibraryModel, pass it to tea.NewProgram().
5. Call `p.Run()`. Handle `tea.Quit()` returned error.

### GO CONCEPTS TO UNDERSTAND
- `os.Args`, `flag` package — CLI input handling.
- `tea.NewProgram(model, opts...)` — how Bubble Tea starts.
- `os.UserConfigDir()` / `os.MkdirAll()` — XDG paths.
- Error wrapping with `fmt.Errorf("opening db: %w", err)`.

### GOTCHAS
- The DB connection must be closed on exit. Use `defer db.Close()`.
- Bubble Tea programs block until they quit — any setup (db init) must
  happen BEFORE `p.Run()` or via `tea.Batch(...)` Init commands.
- Don't do file I/O inside the TUI update loop. Parse tabs before the loop
  and pass the parsed `Tab` struct, not the file path, to the model.

### IF STUCK, READ/SEARCH
- Bubble Tea tutorial: https://github.com/charmbracelet/bubbletea/tree/master/tutorials/basics
- "golang cli subcommand pattern" → shows how to branch on os.Args
- "modernc.org/sqlite example" → shows pure-Go SQLite with database/sql API
- Bubble Tea "real world examples" repo: https://github.com/charmbracelet/bubbletea/tree/master/examples

### SKELETON

func main() {
    dbPath := filepath.Join(mustConfigDir(), "fretboard.db")
    db, err := sql.Open("sqlite", dbPath)
    if err != nil { /* ... */ }
    defer db.Close()

    var filePath string
    if len(os.Args) > 1 {
        filePath = os.Args[1]
    }

    var m tea.Model
    if filePath != "" {
        tab, err := parser.ParseFile(filePath)
        if err != nil { /* ... */ }
        m = tui.NewViewer(tab)
    } else {
        m = tui.NewLibrary(db)
    }

    p := tea.NewProgram(m)
    if _, err := p.Run(); err != nil { /* ... */ }
}
