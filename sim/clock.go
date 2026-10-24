// Package sim provides the tools that drive a deterministic simulation:
// a manually advanced clock and a virtual-time event scheduler.
package sim

import "time"

// ManualClock is controlled by the simulation.
type ManualClock struct {
	now time.Time
}

func NewManualClock(start time.Time) *ManualClock {
	return &ManualClock{now: start}
}

func (c *ManualClock) Now() time.Time {
	return c.now
}

func (c *ManualClock) Advance(d time.Duration) {
	if d < 0 {
		panic("cannot move simulated time backwards")
	}
	c.now = c.now.Add(d)
}
