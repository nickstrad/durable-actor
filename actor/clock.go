package actor

import "time"

// Clock abstracts the source of time.
type Clock interface {
	Now() time.Time
}

// Scheduler runs actions at a later point in virtual time.
// sim.Scheduler implements it.
type Scheduler interface {
	Clock
	After(delay time.Duration, action func())
}
