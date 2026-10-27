package actor

import (
	"fmt"
	"math/rand"
	"slices"
	"testing"
	"time"

	"github.com/nickstrad/durable-actor/sim"
)

// readNow sends a Get straight to the actor's mailbox, skipping the
// network, and runs the scheduler without advancing time.
func readNow(t *testing.T, sched *sim.Scheduler, a *Mailbox, key string) (string, bool) {
	t.Helper()

	var value string
	var found, replied bool

	a.Send(GetCommand{
		Key: key,
		Reply: func(v string, ok bool) {
			value, found, replied = v, ok, true
		},
	})

	sched.RunUntil(sched.Now())

	if !replied {
		t.Fatal("missing read reply")
	}

	return value, found
}

func TestNetworkDeliversAtVirtualDeadline(t *testing.T) {
	sched := sim.NewScheduler(time.Unix(0, 0))
	actor := NewKVMailbox(sched)

	net := NewSimNetwork(sched, func(Packet) Delivery {
		return Delivery{Delay: 250 * time.Millisecond}
	})
	net.Register("jobs", actor)

	net.Send("client", "jobs", PutCommand{
		Key:   "j1",
		Value: "queued",
		TTL:   time.Hour,
	})

	// Immediately before delivery.
	sched.RunUntil(sched.Now().Add(249 * time.Millisecond))

	if _, ok := readNow(t, sched, actor, "j1"); ok {
		t.Fatal("message arrived too early")
	}

	// Advance to the exact delivery time.
	sched.RunUntil(sched.Now().Add(time.Millisecond))

	value, ok := readNow(t, sched, actor, "j1")
	if !ok || value != "queued" {
		t.Fatalf("at 250ms: got (%q, %v); want (\"queued\", true)", value, ok)
	}
}

func TestNetworkDropsPackets(t *testing.T) {
	start := time.Unix(0, 0)
	sched := sim.NewScheduler(start)
	actor := NewKVMailbox(sched)

	// Drop the first packet only.
	net := NewSimNetwork(sched, func(p Packet) Delivery {
		return Delivery{Drop: p.ID == 1}
	})
	net.Register("jobs", actor)

	net.Send("client", "jobs", PutCommand{Key: "lost", Value: "x", TTL: time.Hour})
	net.Send("client", "jobs", PutCommand{Key: "kept", Value: "y", TTL: time.Hour})

	sched.RunUntil(sched.Now().Add(time.Second))

	if _, ok := readNow(t, sched, actor, "lost"); ok {
		t.Fatal("dropped packet was delivered")
	}

	if v, ok := readNow(t, sched, actor, "kept"); !ok || v != "y" {
		t.Fatalf("kept: got (%q, %v); want (\"y\", true)", v, ok)
	}

	want := []NetworkEvent{
		{At: start, Kind: NetworkSend, ID: 1},
		{At: start, Kind: NetworkDrop, ID: 1},
		{At: start, Kind: NetworkSend, ID: 2},
		{At: start, Kind: NetworkDeliver, ID: 2},
	}

	if got := net.Events(); !slices.Equal(got, want) {
		t.Fatalf("events:\ngot  %v\nwant %v", got, want)
	}
}

// runSeededNetwork sends a fixed set of messages through a seeded
// lossy, slow network and returns the network events.
func runSeededNetwork(seed int64) []NetworkEvent {
	sched := sim.NewScheduler(time.Unix(0, 0))
	actor := NewKVMailbox(sched)
	rng := rand.New(rand.NewSource(seed))

	net := NewSimNetwork(sched, SeededDeliveryPolicy(rng, 30, 500*time.Millisecond))
	net.Register("jobs", actor)

	for i := range 20 {
		net.Send("client", "jobs", PutCommand{
			Key:   keyName(i % numKeys),
			Value: fmt.Sprintf("v%d", i),
			TTL:   time.Hour,
		})
	}

	sched.RunUntil(sched.Now().Add(time.Second))

	return net.Events()
}

func TestSeededNetworkIsReproducible(t *testing.T) {
	const seed = 42

	first := runSeededNetwork(seed)
	second := runSeededNetwork(seed)

	if !slices.Equal(first, second) {
		t.Fatalf("seed %d gave different events:\n%v\n%v", seed, first, second)
	}

	// Check that the policy injected both loss and latency, so the
	// test is not passing on a trivial network. All sends happen at
	// the start time, so a later delivery means a delay was applied.
	start := time.Unix(0, 0)
	var dropped, delayed bool

	for _, e := range first {
		switch e.Kind {
		case NetworkDrop:
			dropped = true
		case NetworkDeliver:
			delayed = delayed || e.At.After(start)
		}
	}

	if !dropped {
		t.Fatalf("seed %d dropped no packets", seed)
	}

	if !delayed {
		t.Fatalf("seed %d delayed no packets", seed)
	}
}

func TestPerPacketDelayChangesDeliveryOrder(t *testing.T) {
	start := time.Unix(0, 0)
	sched := sim.NewScheduler(start)
	actor := NewKVMailbox(sched)

	// Slow down the first packet so it arrives after the second.
	net := NewSimNetwork(sched, func(p Packet) Delivery {
		delay := 10 * time.Millisecond
		if p.ID == 1 {
			delay = 100 * time.Millisecond
		}
		return Delivery{Delay: delay}
	})
	net.Register("jobs", actor)

	net.Send("client", "jobs", PutCommand{Key: "state", Value: "first", TTL: time.Hour})
	net.Send("client", "jobs", PutCommand{Key: "state", Value: "second", TTL: time.Hour})

	sched.RunUntil(start.Add(100 * time.Millisecond))

	want := []NetworkEvent{
		{At: start, Kind: NetworkSend, ID: 1},
		{At: start, Kind: NetworkSend, ID: 2},
		{At: start.Add(10 * time.Millisecond), Kind: NetworkDeliver, ID: 2},
		{At: start.Add(100 * time.Millisecond), Kind: NetworkDeliver, ID: 1},
	}

	if got := net.Events(); !slices.Equal(got, want) {
		t.Fatalf("events:\ngot  %v\nwant %v", got, want)
	}

	// The write sent first lands last, so it wins.
	if v, ok := readNow(t, sched, actor, "state"); !ok || v != "first" {
		t.Fatalf("state: got (%q, %v); want (\"first\", true)", v, ok)
	}
}
