package actor

import (
	"fmt"
	"math/rand"
	"slices"
	"time"
)

// Packet is a command in transit between two actors.
type Packet struct {
	ID      uint64
	From    string
	To      string
	Command Command
}

// Delivery decides what happens to one packet.
type Delivery struct {
	Drop  bool
	Delay time.Duration
}

// DeliveryPolicy picks the Delivery for each packet.
type DeliveryPolicy func(Packet) Delivery

// NetworkEventKind says what happened to a packet.
type NetworkEventKind int

const (
	NetworkSend NetworkEventKind = iota
	NetworkDrop
	NetworkDeliver
)

func (k NetworkEventKind) String() string {
	return [...]string{"send", "drop", "deliver"}[k]
}

// NetworkEvent records one observable network action.
type NetworkEvent struct {
	At   time.Time
	Kind NetworkEventKind
	ID   uint64
}

// SimNetwork delivers commands to registered actors.
// Each delivery is an event on the scheduler, so latency
// and loss are under the simulation's control.
type SimNetwork struct {
	sched  Scheduler
	actors map[string]*Mailbox
	policy DeliveryPolicy
	nextID uint64
	events []NetworkEvent
}

// NewSimNetwork returns a network that uses policy for every packet.
// A nil policy delivers every packet with no delay.
func NewSimNetwork(sched Scheduler, policy DeliveryPolicy) *SimNetwork {
	if sched == nil {
		panic("nil scheduler")
	}

	if policy == nil {
		policy = func(Packet) Delivery {
			return Delivery{}
		}
	}

	return &SimNetwork{
		sched:  sched,
		actors: make(map[string]*Mailbox),
		policy: policy,
	}
}

// Register connects an actor to the network under name.
func (n *SimNetwork) Register(name string, a *Mailbox) {
	if a == nil {
		panic("nil actor")
	}

	if a.sched != n.sched {
		panic("actor must use the network's scheduler")
	}

	if _, exists := n.actors[name]; exists {
		panic(fmt.Sprintf("duplicate actor: %s", name))
	}

	n.actors[name] = a
}

// Send schedules delivery of cmd to the actor named to,
// or drops it, as the policy decides. It returns the packet ID.
func (n *SimNetwork) Send(from, to string, cmd Command) uint64 {
	if cmd == nil {
		panic("nil command")
	}

	dest, exists := n.actors[to]
	if !exists {
		panic(fmt.Sprintf("unknown actor: %s", to))
	}

	n.nextID++

	packet := Packet{
		ID:      n.nextID,
		From:    from,
		To:      to,
		Command: cmd,
	}

	decision := n.policy(packet)

	n.record(NetworkSend, packet.ID)

	if decision.Drop {
		n.record(NetworkDrop, packet.ID)
		return packet.ID
	}

	// Network delivery becomes a scheduled event.
	n.sched.After(decision.Delay, func() {
		n.record(NetworkDeliver, packet.ID)
		dest.Send(cmd)
	})

	return packet.ID
}

func (n *SimNetwork) record(kind NetworkEventKind, id uint64) {
	n.events = append(n.events, NetworkEvent{
		At:   n.sched.Now(),
		Kind: kind,
		ID:   id,
	})
}

// Events returns a copy of the network events so far.
func (n *SimNetwork) Events() []NetworkEvent {
	return slices.Clone(n.events)
}

// SeededDeliveryPolicy drops dropPercent of packets and delays the
// rest by a random duration less than maxDelay. It draws from rng, so
// the caller's one seed drives the network along with everything else.
func SeededDeliveryPolicy(rng *rand.Rand, dropPercent int, maxDelay time.Duration) DeliveryPolicy {
	if rng == nil {
		panic("nil rng")
	}

	if dropPercent < 0 || dropPercent > 100 {
		panic("invalid drop percentage")
	}

	if maxDelay < 0 {
		panic("negative max delay")
	}

	return func(Packet) Delivery {
		drop := rng.Intn(100) < dropPercent

		var delay time.Duration
		if maxDelay > 0 {
			delay = time.Duration(rng.Int63n(int64(maxDelay)))
		}

		return Delivery{
			Drop:  drop,
			Delay: delay,
		}
	}
}
