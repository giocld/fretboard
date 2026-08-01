package player

// Cursor tracks the playhead position for the TUI.
type Cursor struct {
	Bar     int
	Col     int
	Playing bool
}

// Advance moves the cursor one step; if it exceeds colsPerBar it wraps to the
// next bar.
func (c *Cursor) Advance(colsPerBar int) {
	if c == nil {
		return
	}
	c.Col++
	if colsPerBar > 0 && c.Col >= colsPerBar {
		c.Bar++
		c.Col = 0
	}
}

// Reset returns the cursor to the start of the piece.
func (c *Cursor) Reset() {
	if c == nil {
		return
	}
	c.Bar = 0
	c.Col = 0
	c.Playing = false
}
