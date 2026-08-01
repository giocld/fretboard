### WHAT THIS PACKAGE DOES
All the Bubble Tea UI code: the tab viewer, the library browser, search,
status bar, and styling. This is the "frontend" of the app.

### BUBBLE TEA MENTAL MODEL

Every screen in Bubble Tea follows the same pattern:

```
type Model struct { ... }        // Holds state (cursor, data, viewport, etc.)

func (m Model) Init() tea.Cmd    // Runs once on startup. Returns initial commands.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd)
                                 // Called on every event (keypress, tick, resize).
                                 // Returns updated model + optional side-effect commands.
func (m Model) View() string     // Renders the screen as a string. Called after Update.
```

Events flow like this:

```
User presses 'j' → tea.KeyMsg → Update(msg) → model.cursor++
                                              → View() re-renders
                                              → terminal shows new state
```

Commands (tea.Cmd) are how you do async work:

```
Init() returns tea.Batch(loadTabCmd, startPlayerCmd)
     → Both run in goroutines
     → Each sends a Msg back when done
     → Update() handles the result
```

### FILE: app.go

#### WHAT IT DOES
The top-level model. Holds the current view (viewer, browser, or help).
Routes keypresses to the active sub-model.

#### HOW TO THINK ABOUT IT
Think of this as a router. It doesn't render anything itself — it delegates
View() and Update() to whichever sub-model is active. Only handles top-level
keys like `Ctrl+C` (quit) and switching between views.

#### STEP-BY-STEP
1. Define `Model` with a `view int` field (constants: `viewLibrary, viewViewer, viewHelp`).
2. Store sub-models as fields: `library *LibraryModel`, `viewer *ViewerModel`.
3. `Update()`: switch on msg type. `tea.KeyMsg{String: "ctrl+c"}` → `tea.Quit`.
   Pass other keys to the active sub-model's Update().
4. `View()`: return the active sub-model's View().
5. Window resize events: forward `tea.WindowSizeMsg` to both sub-models.
6. View switching: on `:open` command in viewer → switch to library. On `Enter` in
   library → load that tab → switch to viewer.

#### GO CONCEPTS
- `tea.Model` interface (Init, Update, View).
- Type switch: `switch msg := msg.(type) { case tea.KeyMsg: ... }`.
- Embedding/composition: Model has-a ViewerModel, not is-a ViewerModel.

#### GOTCHAS
- Bubble Tea resizes the terminal on start. Your first WindowSizeMsg may come
  AFTER Init(). Initialize your viewport with a default size (80x24) and resize on
  the first WindowSizeMsg.
- Don't store `*tea.Program` in your model. Use `tea.Quit()` command to exit.
- Centralize keybinding constants so you can show them in the help screen.

#### SKELETON

type Model struct {
    view    viewType
    library LibraryModel
    viewer  ViewerModel
    width   int
    height  int
}

func New() Model {
    return Model{
        view:   viewLibrary,
        library: NewLibraryModel(...),
        viewer:  ViewerModel{Ready: false},
    }
}

func (m Model) Init() tea.Cmd {
    return m.library.Init()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyMsg:
        switch msg.String() {
        case "ctrl+c", "q":
            return m, tea.Quit
        }
    }
    switch m.view {
    case viewLibrary:
        newLib, cmd := m.library.Update(msg)
        m.library = newLib
        return m, cmd
    case viewViewer:
        newV, cmd := m.viewer.Update(msg)
        m.viewer = newV
        return m, cmd
    }
    return m, nil
}

func (m Model) View() string {
    switch m.view {
    case viewLibrary: return m.library.View()
    case viewViewer:  return m.viewer.View()
    default:          return "loading..."
    }
}

### FILE: viewer.go

#### WHAT IT DOES
The main tab display: shows string lines with fret numbers, bar delimiters,
cursor highlight during playback, and navigation controls.

#### HOW TO THINK ABOUT IT
You're rendering an ASCII tab as styled lipgloss text inside a viewport.
The tab data is already parsed ([]Bar, []StringLine, []Segment). Your job
is to turn that structured data back into display text, with color.

#### STEP-BY-STEP
1. Create a `ViewerModel` struct containing:
   - `tab *model.Tab`
   - `viewport viewport.Model` — from `bubbles/viewport`, handles scrolling
   - `cursorBar, cursorCol int` — current position (editing/playback)
   - `playing bool`
   - `width, height int`

2. **Render function (`renderTab(tab) string`):**
   For each bar in the visible range:
   - Write bar number above: `  [12]`
   - For each string line in the bar: render `stringName│segments│`
     - Walk Segments, for each: style the Char with lipgloss
     - Fret digits → bold white/colored
     - Hyphens → dim gray
     - Technique marks → yellow/italic
   - Add a blank line between bars for readability

3. **Color per string — use Lipgloss Adaptive Colors:**
   ```
   var stringColors = []lipgloss.AdaptiveColor{
       {Light: "#333333", Dark: "#AAAAAA"},  // low E
       {Light: "#444444", Dark: "#BBBBBB"},  // A
       {Light: "#555555", Dark: "#CCCCCC"},  // D
       {Light: "#666666", Dark: "#DDDDDD"},  // G
       {Light: "#777777", Dark: "#EEEEEE"},  // B
       {Light: "#888888", Dark: "#FFFFFF"},  // high e
   }
   ```
   Each string line uses its color as the foreground for the whole line.

4. **Cursor highlight:** During playback, when `cursorBar == bar.Number`
   and `cursorCol == seg.Position`, render that character with inverted colors
   (use `lipgloss.NewStyle().Reverse(true)`).

5. **Viewport update loop:** Set the viewport content to `renderTab(tab)`.
   The viewport handles vertical/horizontal scrolling automatically.

6. **Status bar (bottom):** Show filename, tuning, BPM, loop markers.
   Rendered as a separate lipgloss line that's placed below the viewport.

#### KEY CHALLENGE — Multi-digit frets
Fret "10" takes two character columns in the raw text but is ONE fret.
In the current Segment model (one rune), this is ambiguous. Approach:
- For MVP, flag multi-digit frets and display them as "?".
- Later, extend Segment with `Value int` and `Width int` fields.
  Char='1', Value=10, Width=2. Then render Value and pad with spaces.

#### GO CONCEPTS
- `strings.Builder` — efficient string concatenation in loops.
- `viewport.Model` from `bubbles/viewport` — handles scrolling out of the box.
- Lipgloss `Style.Render(string) string` — apply styles.
- Lipgloss `Style.Width(int)`, `Style.MaxWidth(int)` — constrain widths.
- Composition: `lipgloss.JoinVertical(lipgloss.Left, header, viewport.View(), statusbar)`.

#### GOTCHAS
- The viewport Model has its own keybindings (up/down/pageup/pagedown).
  You need to let those pass through. In Update(), check if the viewport
  handled the key first; if not, handle it yourself.
- Viewport content must be set AFTER the model knows its size.
  In Update(), on WindowSizeMsg, recalculate: `viewport.SetContent(renderTab(tab))`
  and `viewport.Width = w; viewport.Height = h - statusBarHeight`.
- Tab characters in source data will mess up column alignment.
  Replace `\t` with four spaces during parsing or rendering.
- Terminal width: if a bar is wider than the terminal, lipgloss wraps it
  (ugly). Use viewport's built-in horizontal scrolling + `Style.MaxWidth()`.

#### IF STUCK
- "bubbletea viewport example" — ship with bubbles library
- "lipgloss adaptive colors" — for terminal-theme-aware styling
- "golang strings builder tutorial"
- Look at: https://github.com/charmbracelet/bubbles/tree/master/viewport

### FILE: browser.go

#### WHAT IT DOES
The library browser: a scrollable list of tabs with search, sort, and metadata display.

#### HOW TO THINK ABOUT IT
It's a list component. Like the viewer, it's a viewport + rendered content.
Each row is a tab entry. Up/down keys move a highlight. Enter opens the tab.

#### STEP-BY-STEP
1. `BrowserModel` struct:
   - `store *library.Store`
   - `tabs []TabRow` — all loaded tabs from DB
   - `cursor int` — currently highlighted row index
   - `viewport viewport.Model`
   - `filters` — search query, sort order
   - `width, height int`

2. **Load tabs on Init:**
   `func (m BrowserModel) Init() tea.Cmd { return loadTabsCmd(m.store) }`
   `loadTabsCmd` queries the DB and returns a `TabsLoadedMsg{[]TabRow}`.

3. **Render list (`renderList(tabs, cursor) string`):**
   For each tab, render one row: `★ Sultans of Swing — Dire Straits     E Std     ★★★★`.
   The highlighted row gets inverted colors. Favorites show a star.

4. **Keybindings:**
   - `j/k` or `↓/↑` → move cursor
   - `/` → enter search mode (focus on search bar)
   - `Enter` → open selected tab (sends a message to the top-level Model)
   - `d` → delete selected tab (with confirmation)
   - `e` → edit metadata modal (or inline)
   - `f` → toggle favorite
   - `s` → cycle sort: alpha / recent / most-played / difficulty
   - `r` → reload from DB (after external file changes)

5. **Fuzzy search:** When user types in the search bar, filter `tabs` by
   matching query against title + artist using `github.com/sahilm/fuzzy`.
   Show only matches. Clear search on Escape.

#### GO CONCEPTS
- `tea.Batch(...tea.Cmd)` — running multiple commands concurrently.
- Custom message types: `type TabsLoadedMsg struct{ Tabs []TabRow }`.
- Tea commands as closures: `func() tea.Msg { ... }`.
- Sorting with `sort.Slice(tabs, func(i,j int) bool { ... })`.

#### GOTCHAS
- Don't re-query the DB in every `View()` call. Load once, filter in memory.
- Fuzzy search can be slow for 1000+ tabs. Use `fuzzy.FindFrom` (pre-filtered).
- The cursor should stay within bounds: `max(0, min(cursor, len(tabs)-1))`.
- On tab change (add/delete), refresh the list.

#### IF STUCK
- "bubble tea list component" — `bubbles/list` package, may be easier than custom rendering.
- "bubble tea custom message type" — how to define and send your own messages.
- Look at: https://github.com/charmbracelet/bubbletea/tree/master/examples/list-simple

### FILE: search.go

#### WHAT IT DOES
A text input component for fuzzy searching the library. Appears when `/` is pressed.

#### HOW TO THINK ABOUT IT
You already get this for free from `bubbles/textinput`. Wrap it in a small model
that shows/hides, and on each keystroke, filters the parent list.

#### STEP-BY-STEP
1. Embed `textinput.Model` from `bubbles/textinput`.
2. When activated (key `/`): `textinput.Focus()`, store in BrowserModel.SearchActive = true.
3. On each Update, call `textinput.Update(msg)`. The text input handles its own cursor and rendering.
4. In the browser's render: if SearchActive, prepend the text input to the viewport content.
5. Filter: on every change in the text input value, run fuzzy search against `tabs`.

#### GO CONCEPTS
- Embedding: `type SearchBar struct { textinput.Model }`.
- Focusing/unfocusing: `textinput.Focus()` / `textinput.Blur()`.
- Styling: `textinput.New()` returns a model you can style with `input.PromptStyle = ...`.

#### SKELETON

type SearchModel struct {
    textinput.Model
    active bool
}

func NewSearch() SearchModel {
    ti := textinput.New()
    ti.Placeholder = "Search tabs..."
    ti.Prompt = "/ "
    return SearchModel{Model: ti}
}

### FILE: statusbar.go

#### WHAT IT DOES
The bottom bar: shows filename, tuning, mode indicator, play status, BPM, and
available commands.

#### HOW TO THINK ABOUT IT
A single lipgloss-rendered line. Left side = context info. Right side = key hints.
Use lipgloss placement to split it: `lipgloss.JoinHorizontal(lipgloss.Top, left, right)`
with `right` aligned via `Style.Align(lipgloss.Right)`.

#### STEP-BY-STEP
1. Define a `StatusBar` function (not a model — just a render helper):
   ```
   func StatusBar(width int, info StatusInfo) string
   ```
2. `StatusInfo` is a struct with fields: Filename, Tuning, BPM, Playing, LoopStart, LoopEnd.
3. Left side: `"sultans-of-swing.txt  │  E Standard  │  BPM: 120"`
4. Right side: `"j/k:scroll  /:search  Space:play  q:quit"`
5. Style: dark background, light text, subtle padding.
6. Full width: `barStyle.Width(width).Render(left + "  " + right)`.

#### GOTCHAS
- Make sure the bar is always the full terminal width. Use WindowSizeMsg width.
- The status bar height is fixed (1 row). Subtract from viewport height.
- Refresh the status bar on every View() call — don't cache it.

### FILE: styles.go

#### WHAT IT DOES
Centralized lipgloss style definitions. Every color, border, padding, margin
is defined here so the app has a consistent look.

#### HOW TO THINK ABOUT IT
Define styles as package-level variables. Compose them. Name them by purpose,
not by appearance: `barNumberStyle`, not `boldWhiteStyle`.

#### STEP-BY-STEP
1. Define a color palette:
   ```
   var (
       primary   = lipgloss.AdaptiveColor{Light: "#222222", Dark: "#DDDDDD"}
       secondary = lipgloss.AdaptiveColor{Light: "#555555", Dark: "#999999"}
       dimmed    = lipgloss.AdaptiveColor{Light: "#AAAAAA", Dark: "#555555"}
       accent    = lipgloss.AdaptiveColor{Light: "#0000FF", Dark: "#6699FF"}
       highlight = lipgloss.AdaptiveColor{Light: "#FFFF00", Dark: "#FFAA00"}
   )
   ```
2. Define semantic styles:
   ```
   var (
       barNumber   = lipgloss.NewStyle().Foreground(secondary).Bold(true).Width(5)
       fretDigit   = lipgloss.NewStyle().Foreground(primary).Bold(true)
       rest        = lipgloss.NewStyle().Foreground(dimmed)
       technique   = lipgloss.NewStyle().Foreground(accent).Italic(true)
       cursorStyle = lipgloss.NewStyle().Reverse(true)
       statusBar   = lipgloss.NewStyle().Background(lipgloss.Color("#333333")).Padding(0, 1)
       listNormal  = lipgloss.NewStyle().Padding(0, 1)
       listSelected = listNormal.Copy().Reverse(true)
   )
   ```
3. Use `Copy()` to derive styles: `style.Copy().Bold(true)` preserves the base style.

#### GO CONCEPTS
- `lipgloss.Style` — immutable style builder. Every method returns a new Style.
- `AdaptiveColor` — picks light or dark variant based on terminal background.
- Style inheritance: always `Copy()` before modifying to avoid mutating the shared var.

#### GOTCHAS
- Don't modify shared style variables in-place. Always `Copy()`.
- Test on both light and dark terminals. AdaptiveColor helps but verify.
- Some terminals don't support italic. Fall back to a different dim style.
- Width constraints: `Style.Width(n)` truncates or pads. Use `Style.MaxWidth(n)` to allow wrapping.

#### IF STUCK
- Lipgloss readme: https://github.com/charmbracelet/lipgloss
- "lipgloss adaptive color example"
- "lipgloss style composition"

### FILE: keymap.go

#### WHAT IT DOES
Defines all keybindings as constants. Used for both handling keys in Update()
and displaying them in the help/status bar.

#### HOW TO THINK ABOUT IT
Centralize key strings so they're easy to change and document. Use maps
or structs grouped by context (viewer keys, browser keys, global keys).

#### SKELETON

package tui

const (
    KeyQuit  = "q"
    KeyQuit2 = "ctrl+c"
    KeyUp    = "k"
    KeyDown  = "j"
    KeyHelp  = "?"
    KeySearch = "/"
)

var viewerKeys = map[string]string{
    "j/k":       "scroll bars",
    "h/l":       "pan left/right",
    "gg":        "first bar",
    "G":         "last bar",
    "Space":     "play/pause",
    "+/-":       "BPM up/down",
    "[ / ]":     "loop start/end",
    "/":         "search bars",
    "0":         "jump to bar",
    "q":         "quit",
}

var libraryKeys = map[string]string{
    "j/k":   "move cursor",
    "Enter": "open tab",
    "/":     "search",
    "s":     "sort",
    "f":     "favorite",
    "d":     "delete",
    "r":     "refresh",
    "q":     "quit",
}

