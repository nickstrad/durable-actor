package actor

import (
	"testing"
	"time"

	"github.com/nickstrad/durable-actor/sim"
)

func TestLostAckRetriesWithoutDoubleStarting(t *testing.T) {
	start := time.Unix(0, 0)
	sched := sim.NewScheduler(start)

	// Drop the first ack only.
	acks := 0
	net := NewSimNetwork(sched, func(p Packet) Delivery {
		if _, ack := p.Command.(StartAck); ack {
			acks++
			if acks == 1 {
				return Delivery{Drop: true}
			}
		}
		return Delivery{Delay: 10 * time.Millisecond}
	})

	worker := NewSandboxWorker(sched, net, "worker")
	controller := NewSandboxController(sched, net, "controller", "worker",
		50*time.Millisecond, 200*time.Millisecond)

	controller.Start("req-123", "sandbox-123")

	sched.RunUntil(start.Add(200 * time.Millisecond))

	status, attempts := controller.Result("req-123", "sandbox-123")
	if status != Acknowledged || attempts != 2 {
		t.Fatalf("got (%s, %d attempts); want (acknowledged, 2)", status, attempts)
	}

	if n := worker.StartCount("sandbox-123"); n != 1 {
		t.Fatalf("sandbox started %d times; want 1", n)
	}
}

func TestTimeoutDoesNotProveFailure(t *testing.T) {
	start := time.Unix(0, 0)
	sched := sim.NewScheduler(start)

	// Every ack is lost, so the controller never hears back.
	net := NewSimNetwork(sched, func(p Packet) Delivery {
		_, ack := p.Command.(StartAck)
		return Delivery{Drop: ack}
	})

	worker := NewSandboxWorker(sched, net, "worker")
	controller := NewSandboxController(sched, net, "controller", "worker",
		50*time.Millisecond, 130*time.Millisecond)

	controller.Start("req-123", "sandbox-123")

	// Attempts at 0, 50, and 100ms; the deadline fires at 130ms.
	sched.RunUntil(start.Add(200 * time.Millisecond))

	status, attempts := controller.Result("req-123", "sandbox-123")
	if status != TimedOut || attempts != 3 {
		t.Fatalf("got (%s, %d attempts); want (timed_out, 3)", status, attempts)
	}

	// The caller timed out, but the work still happened.
	if n := worker.StartCount("sandbox-123"); n != 1 {
		t.Fatalf("sandbox started %d times; want 1", n)
	}
}

func TestReusedRequestIDIsRejected(t *testing.T) {
	start := time.Unix(0, 0)
	sched := sim.NewScheduler(start)

	requests := 0
	net := NewSimNetwork(sched, func(p Packet) Delivery {
		if _, ok := p.Command.(StartRequest); ok {
			requests++
		}
		return Delivery{Delay: 10 * time.Millisecond}
	})

	worker := NewSandboxWorker(sched, net, "worker")
	controller := NewSandboxController(sched, net, "controller", "worker",
		50*time.Millisecond, 200*time.Millisecond)

	// The same request ID for two sandboxes. The worker sees "a" first.
	controller.Start("req-1", "sandbox-a")
	controller.Start("req-1", "sandbox-b")

	// Repeating a start with the same IDs sends nothing new.
	controller.Start("req-1", "sandbox-a")

	sched.RunUntil(start.Add(200 * time.Millisecond))

	if requests != 2 {
		t.Fatalf("sent %d start requests; want 2", requests)
	}

	if status, attempts := controller.Result("req-1", "sandbox-a"); status != Acknowledged || attempts != 1 {
		t.Fatalf("sandbox-a: got (%s, %d attempts); want (acknowledged, 1)", status, attempts)
	}

	if status, attempts := controller.Result("req-1", "sandbox-b"); status != Rejected || attempts != 1 {
		t.Fatalf("sandbox-b: got (%s, %d attempts); want (rejected, 1)", status, attempts)
	}

	if n := worker.StartCount("sandbox-a"); n != 1 {
		t.Fatalf("sandbox-a started %d times; want 1", n)
	}

	if n := worker.StartCount("sandbox-b"); n != 0 {
		t.Fatalf("sandbox-b started %d times; want 0", n)
	}
}

func TestLateAckDoesNotOverrideTimeout(t *testing.T) {
	start := time.Unix(0, 0)
	sched := sim.NewScheduler(start)

	// Requests are fast; acks take longer than the timeout.
	net := NewSimNetwork(sched, func(p Packet) Delivery {
		if _, ack := p.Command.(StartAck); ack {
			return Delivery{Delay: 200 * time.Millisecond}
		}
		return Delivery{Delay: 10 * time.Millisecond}
	})

	worker := NewSandboxWorker(sched, net, "worker")

	// The retry and the deadline fall at the same instant. The deadline
	// was scheduled first, so it wins and no retry is sent.
	controller := NewSandboxController(sched, net, "controller", "worker",
		100*time.Millisecond, 100*time.Millisecond)

	controller.Start("req-123", "sandbox-123")

	// Past the deadline (100ms) and past the ack's arrival (210ms).
	sched.RunUntil(start.Add(300 * time.Millisecond))

	status, attempts := controller.Result("req-123", "sandbox-123")
	if status != TimedOut || attempts != 1 {
		t.Fatalf("got (%s, %d attempts); want (timed_out, 1)", status, attempts)
	}

	if n := worker.StartCount("sandbox-123"); n != 1 {
		t.Fatalf("sandbox started %d times; want 1", n)
	}
}
