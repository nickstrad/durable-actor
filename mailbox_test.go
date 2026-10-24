package main

import (
	"reflect"
	"testing"
	"time"
)

func TestMailboxIsDeferredAndFIFO(t *testing.T) {
	sim := NewScheduler(time.Unix(0, 0))
	actor := NewMailboxActor(sim)

	var reads []string

	actor.Send(PutCommand{
		Key:   "job",
		Value: "queued",
		TTL:   time.Minute,
	})

	actor.Send(GetCommand{
		Key: "job",
		Reply: func(v string, ok bool) {
			if !ok {
				t.Fatal("missing job")
			}
			reads = append(reads, v)
		},
	})

	actor.Send(PutCommand{
		Key:   "job",
		Value: "running",
		TTL:   time.Minute,
	})

	actor.Send(GetCommand{
		Key: "job",
		Reply: func(v string, ok bool) {
			if !ok {
				t.Fatal("missing job")
			}
			reads = append(reads, v)
		},
	})

	// Sending commands must not execute them.
	if len(reads) != 0 {
		t.Fatal("commands executed before simulation advanced")
	}

	// One scheduler step processes one command.
	if !sim.RunNext() {
		t.Fatal("expected first command")
	}

	if len(reads) != 0 {
		t.Fatal("processed more than one command per step")
	}

	// Execute the remaining commands.
	for sim.RunNext() {
	}

	want := []string{"queued", "running"}

	if !reflect.DeepEqual(reads, want) {
		t.Fatalf("reads = %v; want %v", reads, want)
	}
}

func TestSameTimeArrivalsPreserveOrder(t *testing.T) {
    sim := NewScheduler(time.Unix(0, 0))
    actor := NewMailboxActor(sim)

    var got string

    sim.After(0, func() {
        actor.Send(PutCommand{
            Key:   "x",
            Value: "first",
            TTL:   time.Minute,
        })
    })

    sim.After(0, func() {
        actor.Send(PutCommand{
            Key:   "x",
            Value: "second",
            TTL:   time.Minute,
        })
    })

    sim.After(0, func() {
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

    for sim.RunNext() {}

    if got != "second" {
        t.Fatalf("got %q; want second", got)
    }
}

func TestCallbackEnqueuesAfterExistingWork(t *testing.T) {
    sim := NewScheduler(time.Unix(0, 0))
    actor := NewMailboxActor(sim)

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

    for sim.RunNext() {}

    if got != "before" {
        t.Fatalf("got %q; want before", got)
    }
}
