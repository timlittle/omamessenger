// Package clocktest provides a manually advanced Clock for deterministic
// tests. Callbacks scheduled for the same instant run in registration order.
package clocktest

import (
	"sort"
	"sync"
	"time"
)

type timer struct {
	due      time.Time
	sequence uint64
	f        func()
	active   bool
}

// Clock is a thread-safe manually advanced clock.
type Clock struct {
	mu       sync.Mutex
	now      time.Time
	sequence uint64
	timers   []*timer
}

// New constructs a clock starting at start.
func New(start time.Time) *Clock { return &Clock{now: start} }

func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *Clock) AfterFunc(d time.Duration, f func()) func() bool {
	c.mu.Lock()
	t := &timer{due: c.now.Add(max(d, 0)), sequence: c.sequence, f: f, active: true}
	c.sequence++
	c.timers = append(c.timers, t)
	c.mu.Unlock()
	return func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		if !t.active {
			return false
		}
		t.active = false
		return true
	}
}

// Advance moves the clock forward and runs every due callback in time order.
// Callbacks run without the clock lock and can safely schedule more timers.
func (c *Clock) Advance(d time.Duration) {
	c.mu.Lock()
	target := c.now.Add(max(d, 0))
	c.mu.Unlock()
	for {
		c.mu.Lock()
		var next *timer
		for _, candidate := range c.timers {
			if !candidate.active || candidate.due.After(target) {
				continue
			}
			if next == nil || candidate.due.Before(next.due) ||
				(candidate.due.Equal(next.due) && candidate.sequence < next.sequence) {
				next = candidate
			}
		}
		if next == nil {
			c.now = target
			c.timers = compact(c.timers)
			c.mu.Unlock()
			return
		}
		next.active = false
		c.now = next.due
		callback := next.f
		c.mu.Unlock()
		callback()
	}
}

// Pending reports the active callback count. It is useful when synchronizing
// a test with a goroutine that has just registered its timer.
func (c *Clock) Pending() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	count := 0
	for _, t := range c.timers {
		if t.active {
			count++
		}
	}
	return count
}

func compact(timers []*timer) []*timer {
	active := timers[:0]
	for _, t := range timers {
		if t.active {
			active = append(active, t)
		}
	}
	sort.SliceStable(active, func(i, j int) bool {
		if active[i].due.Equal(active[j].due) {
			return active[i].sequence < active[j].sequence
		}
		return active[i].due.Before(active[j].due)
	})
	return active
}
