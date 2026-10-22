package main

import (
	"testing"
	"time"
)

func TestActorTTLAtExactBoundary(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := NewManualClock(start)
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
	clock := NewManualClock(time.Unix(0, 0))
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
