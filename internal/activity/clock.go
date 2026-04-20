package activity

import (
	"sync"
	"time"
)

// Clock abstracts time so the registry's cleanup deadline can be driven
// forward synchronously in tests.
type Clock interface {
	Now() time.Time
}

// RealClock is the production Clock backed by time.Now.
type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now() }

// FakeClock is a mutable Clock for tests. Advance with Set or Advance.
type FakeClock struct {
	mu  sync.Mutex
	now time.Time
}

// NewFakeClock returns a FakeClock initialized to t.
func NewFakeClock(t time.Time) *FakeClock {
	return &FakeClock{now: t}
}

func (c *FakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *FakeClock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = t
}

func (c *FakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}
