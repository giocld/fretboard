package player

import "time"

// StepClock schedules playback steps against absolute wall deadlines.
// A tea.Tick chain re-bases its timer after every message, so render and
// processing time accumulates as drift; StepClock instead rolls an absolute
// deadline forward by each step's duration, and tea.Tick only waits
// time.Until(deadline). Lateness is measurable and catch-up-able.
type StepClock struct {
	deadline time.Time
}

// Start sets the deadline for the first upcoming step, delay from now.
func (c *StepClock) Start(delay time.Duration) {
	c.deadline = time.Now().Add(delay)
}

// Next rolls the deadline forward by one step's duration.
func (c *StepClock) Next(d time.Duration) {
	c.deadline = c.deadline.Add(d)
}

// Rebase restarts the clock from now with the given delay — used when the
// BPM changes mid-song (the current step gets a fresh duration; the session
// itself never restarts).
func (c *StepClock) Rebase(delay time.Duration) {
	c.deadline = time.Now().Add(delay)
}

// Deadline returns the next step's absolute start time.
func (c *StepClock) Deadline() time.Time { return c.deadline }

// Until returns how long until the next deadline (negative = late).
func (c *StepClock) Until() time.Duration { return time.Until(c.deadline) }

// Late reports how far past the deadline the clock is at now; positive
// means the next step is already due (lateness).
func (c *StepClock) Late(now time.Time) time.Duration { return now.Sub(c.deadline) }
