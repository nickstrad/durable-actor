package actor

import (
	"testing"
	"time"

	"github.com/nickstrad/durable-actor/sim"
)

func TestActorTTLAtExactBoundary(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := sim.NewManualClock(start)
	actor := NewActor(clock)

	actor.Put("session", "active", 10*time.Second)

	// Move to just before expiration.
	clock.Advance(9*time.Second + 999*time.Millisecond)

	if got, ok := actor.Get("session"); !ok || got != "active" {
		t.Fatalf(
			"before expiry: got (%q, %v); want (active, true)",
			got, ok,
		)
	}

	// Advance to the exact expiration boundary.
	clock.Advance(time.Millisecond)

	if got, ok := actor.Get("session"); ok {
		t.Fatalf(
			"at expiry: got (%q, %v); want missing",
			got, ok,
		)
	}
}

func TestActorTTLResetsOnOverwrite(t *testing.T) {
	clock := sim.NewManualClock(time.Unix(0, 0))
	actor := NewActor(clock)

	actor.Put("job", "queued", 5*time.Second)

	clock.Advance(4 * time.Second)

	actor.Put("job", "running", 5*time.Second)

	clock.Advance(2 * time.Second)

	if got, ok := actor.Get("job"); !ok || got != "running" {
		t.Fatalf(
			"overwrite should reset expiry, got (%q, %v)",
			got, ok,
		)
	}
}

func TestActorExpiryDrivenByScheduler(t *testing.T) {
	const ttl = 5 * time.Second
	start := time.Unix(0, 0)
	sched := sim.NewScheduler(start)
	actor := NewActor(sched)

	var (
		before   string
		beforeOK bool
		atTTLOK  bool
	)

	sched.After(0, func() {
		actor.Put("session", "active", ttl)
	})

	sched.After(ttl-time.Second, func() {
		before, beforeOK = actor.Get("session")
	})

	sched.After(ttl, func() {
		_, atTTLOK = actor.Get("session")
	})

	deadline := start.Add(ttl)
	n := sched.RunUntil(deadline)

	if n != 3 {
		t.Errorf("ran %d events; want 3", n)
	}

	if !beforeOK || before != "active" {
		t.Errorf("1s before expiry: got (%q, %v); want (active, true)", before, beforeOK)
	}

	if atTTLOK {
		t.Error("at expiry: key still present; want missing")
	}

	if got := sched.Now(); !got.Equal(deadline) {
		t.Errorf("Now() = %s; want %s", got, deadline)
	}
}
