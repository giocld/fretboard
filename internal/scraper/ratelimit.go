package scraper

import (
	"sync"
	"time"
)

// rateLimiter enforces a minimum delay between network requests, shared by
// all scraper backends.
type rateLimiter struct {
	delay   time.Duration
	lastReq time.Time
	mu      sync.Mutex
}

func (r *rateLimiter) throttle() {
	if r.delay <= 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	since := time.Since(r.lastReq)
	if since < r.delay {
		time.Sleep(r.delay - since)
	}
	r.lastReq = time.Now()
}
