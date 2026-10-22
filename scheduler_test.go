package main

import (
	"slices"
	"testing"
	"time"
)

func TestSchedulerOrdersEventsAndBreaksTies(t *testing.T) {
	sim := NewScheduler(time.Unix(0, 0))

	var got []string

	sim.After(100*time.Millisecond, func() {
		got = append(got, "A")

		// Schedule another event at the current time.
		sim.After(0, func() {
			got = append(got, "D")
		})
	})

	sim.After(50*time.Millisecond, func() {
		got = append(got, "B")
	})

	sim.After(100*time.Millisecond, func() {
		got = append(got, "C")
	})

	sim.RunUntil(sim.Now().Add(time.Second))

	want := []string{"B", "A", "C", "D"}

	if !slices.Equal(got, want) {
		t.Fatalf("got %v; want %v", got, want)
	}
}

func TestActorExpiryDrivenByScheduler(t *testing.T) {
	const ttl = 5 * time.Second
	start := time.Unix(0, 0)
	sim := NewScheduler(start)
	actor := NewActor(sim)

	var (
		before   string
		beforeOK bool
		atTTLOK  bool
	)

	sim.After(0, func() {
		actor.Put("session", "active", ttl)
	})

	sim.After(ttl-time.Second, func() {
		before, beforeOK = actor.Get("session")
	})

	sim.After(ttl, func() {
		_, atTTLOK = actor.Get("session")
	})

	deadline := start.Add(ttl)
	n := sim.RunUntil(deadline)

	if n != 3 {
		t.Errorf("ran %d events; want 3", n)
	}

	if !beforeOK || before != "active" {
		t.Errorf("1s before expiry: got (%q, %v); want (active, true)", before, beforeOK)
	}

	if atTTLOK {
		t.Error("at expiry: key still present; want missing")
	}

	if got := sim.Now(); !got.Equal(deadline) {
		t.Errorf("Now() = %s; want %s", got, deadline)
	}
}
