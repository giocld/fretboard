package tui

const (
	KeyQuit   = "q"
	KeyQuit2  = "ctrl+c"
	KeyUp     = "k"
	KeyDown   = "j"
	KeyHelp   = "?"
	KeySearch = "/"
)

var viewerKeys = map[string]string{
	"j/k":   "scroll bars",
	"h/l":   "pan left/right",
	"gg":    "first bar",
	"G":     "last bar",
	"Space/p": "play/pause",
	"+/-":     "BPM up/down",
	"Esc":     "clear/back/stop",
	"0":     "jump to bar",
	"q":     "quit",
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
