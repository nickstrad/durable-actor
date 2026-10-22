package main

import "time"

// Clock abstracts the source of time.
type Clock interface {
	Now() time.Time
}

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

type entry struct {
	value     string
	expiresAt time.Time
}

// Actor owns its key-value state.
type Actor struct {
	clock Clock
	data  map[string]entry
}

func NewActor(clock Clock) *Actor {
	return &Actor{
		clock: clock,
		data:  make(map[string]entry),
	}
}

func (a *Actor) Put(key, value string, ttl time.Duration) {
	if ttl <= 0 {
		panic("ttl must be positive")
	}

	a.data[key] = entry{
		value:     value,
		expiresAt: a.clock.Now().Add(ttl),
	}
}

func (a *Actor) Get(key string) (string, bool) {
	e, ok := a.data[key]
	if !ok {
		return "", false
	}

	if !a.clock.Now().Before(e.expiresAt) {
		delete(a.data, key)
		return "", false
	}

	return e.value, true
}
