package actor

import "time"

type entry struct {
	value     string
	expiresAt time.Time
}

// Actor owns its key-value state.
type Actor struct {
	clock Clock
	data  map[string]entry
}

// NewActor returns an empty Actor that reads time from clock.
func NewActor(clock Clock) *Actor {
	return &Actor{
		clock: clock,
		data:  make(map[string]entry),
	}
}

// Put stores value under key until ttl has passed. ttl must be positive.
func (a *Actor) Put(key, value string, ttl time.Duration) {
	if ttl <= 0 {
		panic("ttl must be positive")
	}

	a.data[key] = entry{
		value:     value,
		expiresAt: a.clock.Now().Add(ttl),
	}
}

// Get returns the value for key, or false if it is missing or expired.
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
