package sim

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
