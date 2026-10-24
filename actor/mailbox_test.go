package actor

import (
	"slices"
	"testing"
	"time"

	"github.com/nickstrad/durable-actor/sim"
)

func TestMailboxIsDeferredAndFIFO(t *testing.T) {
	sched := sim.NewScheduler(time.Unix(0, 0))
	actor := NewMailboxActor(sched)

	var reads []string
	record := func(v string, ok bool) {
		if !ok {
			t.Fatal("missing job")
		}
		reads = append(reads, v)
	}

	actor.Send(PutCommand{
		Key:   "job",
		Value: "queued",
		TTL:   time.Minute,
	})

	actor.Send(GetCommand{
		Key:   "job",
		Reply: record,
	})

	actor.Send(PutCommand{
		Key:   "job",
		Value: "running",
		TTL:   time.Minute,
	})

	actor.Send(GetCommand{
		Key:   "job",
		Reply: record,
	})

	// Sending commands must not execute them.
	if len(reads) != 0 {
		t.Fatal("commands executed before simulation advanced")
	}

	// One scheduler step processes one command.
	if !sched.RunNext() {
		t.Fatal("expected first command")
	}

	if len(reads) != 0 {
		t.Fatal("processed more than one command per step")
	}

	// Execute the remaining commands.
	for sched.RunNext() {
	}

	want := []string{"queued", "running"}

	if !slices.Equal(reads, want) {
		t.Fatalf("reads = %v; want %v", reads, want)
	}
}

func TestSameTimeArrivalsPreserveOrder(t *testing.T) {
	sched := sim.NewScheduler(time.Unix(0, 0))
	actor := NewMailboxActor(sched)

	var got string

	sched.After(0, func() {
		actor.Send(PutCommand{
			Key:   "x",
			Value: "first",
			TTL:   time.Minute,
		})
	})

	sched.After(0, func() {
		actor.Send(PutCommand{
			Key:   "x",
			Value: "second",
			TTL:   time.Minute,
		})
	})

	sched.After(0, func() {
		actor.Send(GetCommand{
			Key: "x",
			Reply: func(v string, ok bool) {
				if !ok {
					t.Fatal("missing x")
				}
				got = v
			},
		})
	})

	for sched.RunNext() {
	}

	if got != "second" {
		t.Fatalf("got %q; want second", got)
	}
}

func TestCallbackEnqueuesAfterExistingWork(t *testing.T) {
	sched := sim.NewScheduler(time.Unix(0, 0))
	actor := NewMailboxActor(sched)

	var got string

	actor.Send(PutCommand{
		Key:   "key",
		Value: "before",
		TTL:   time.Minute,
	})

	actor.Send(GetCommand{
		Key: "key",
		Reply: func(v string, ok bool) {
			actor.Send(PutCommand{
				Key:   "key",
				Value: "after",
				TTL:   time.Minute,
			})
		},
	})

	actor.Send(GetCommand{
		Key: "key",
		Reply: func(v string, ok bool) {
			if !ok {
				t.Fatal("missing key")
			}
			got = v
		},
	})

	for sched.RunNext() {
	}

	if got != "before" {
		t.Fatalf("got %q; want before", got)
	}
}
