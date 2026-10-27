// Command probe drives SandboxController (actor/controller.go) and
// SandboxWorker (actor/worker.go) through edge-case timings and orderings that the Day 6 tests do not cover:
// same-instant ties between the deadline, a retry, and a reply; a reply
// exactly at the deadline; late and duplicate acks; a zero-delay network;
// retryEvery >= timeout; request IDs reused in different orders; two request
// IDs for one sandbox; and determinism across runs.
//
// It is a report, not a test: each scenario prints what happened and whether
// that matches the behavior the code's comments promise. Lines marked NOTE are
// surprising but arguably by design; lines marked BUG contradict the code's
// own documentation. Exit status is 1 if any BUG line was printed.
//
//	go run ./agent_work/validation/probe
package main

import (
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/nickstrad/durable-actor/actor"
	"github.com/nickstrad/durable-actor/sim"
)

const ms = time.Millisecond

var bugs int

func check(ok bool, format string, args ...any) {
	tag := "ok  "
	if !ok {
		tag = "BUG "
		bugs++
	}
	fmt.Printf("    %s %s\n", tag, fmt.Sprintf(format, args...))
}

func note(format string, args ...any) {
	fmt.Printf("    NOTE %s\n", fmt.Sprintf(format, args...))
}

type world struct {
	start  time.Time
	sched  *sim.Scheduler
	net    *actor.SimNetwork
	worker *actor.SandboxWorker
	ctrl   *actor.SandboxController
}

func newWorld(policy actor.DeliveryPolicy, retryEvery, timeout time.Duration) *world {
	w := &world{start: time.Unix(0, 0)}
	w.sched = sim.NewScheduler(w.start)
	w.net = actor.NewSimNetwork(w.sched, policy)
	w.worker = actor.NewSandboxWorker(w.sched, w.net, "worker")
	w.ctrl = actor.NewSandboxController(w.sched, w.net, "controller", "worker", retryEvery, timeout)
	return w
}

func fixed(d time.Duration) actor.DeliveryPolicy {
	return func(actor.Packet) actor.Delivery { return actor.Delivery{Delay: d} }
}

func (w *world) run(until time.Duration) { w.sched.RunUntil(w.start.Add(until)) }

func (w *world) result(req, sb string) (actor.StartStatus, int) { return w.ctrl.Result(req, sb) }

// sends counts StartRequest packets handed to the network.
func (w *world) sends() int {
	n := 0
	for _, e := range w.net.Events() {
		if e.Kind == actor.NetworkSend {
			n++
		}
	}
	return n
}

func scenario(name string, f func()) {
	fmt.Printf("\n== %s\n", name)
	f()
}

func main() {
	scenario("1. reply arrives exactly at the deadline (RTT == timeout)", func() {
		// 10ms each way, timeout 20ms: the ack lands at t=20ms, the same
		// instant the deadline fires. The code comments promise the deadline
		// runs first, so the reply is late.
		w := newWorld(fixed(10*ms), 50*ms, 20*ms)
		w.ctrl.Start("r", "s")
		w.run(100 * ms)
		st, n := w.result("r", "s")
		fmt.Printf("    status=%s attempts=%d starts=%d\n", st, n, w.worker.StartCount("s"))
		check(st == actor.TimedOut && n == 1, "deadline wins the same-instant tie, as documented")
		note("a reply that lands exactly at the deadline is treated as late; the deadline is exclusive")
	})

	scenario("2. reply arrives one tick before the deadline", func() {
		w := newWorld(fixed(10*ms), 50*ms, 20*ms+time.Nanosecond)
		w.ctrl.Start("r", "s")
		w.run(100 * ms)
		st, n := w.result("r", "s")
		fmt.Printf("    status=%s attempts=%d\n", st, n)
		check(st == actor.Acknowledged && n == 1, "1ns of slack flips the result to acknowledged")
	})

	scenario("3. reply arrives exactly when a retry is due (RTT == retryEvery)", func() {
		// 5ms each way, retry every 10ms: the ack and the retry share t=10ms.
		// The retry was scheduled first, so it fires first and resends.
		w := newWorld(fixed(5*ms), 10*ms, 100*ms)
		w.ctrl.Start("r", "s")
		w.run(100 * ms)
		st, n := w.result("r", "s")
		fmt.Printf("    status=%s attempts=%d starts=%d sends=%d\n", st, n, w.worker.StartCount("s"), w.sends())
		check(st == actor.Acknowledged && w.worker.StartCount("s") == 1, "ack recorded, no double start")
		if n == 2 {
			note("one spurious retry: the retry fires before the ack that lands at the same instant (attempts=2)")
		}
	})

	scenario("4. late ack after the deadline is ignored; status stays timed_out", func() {
		// Requests are instant; the first ack takes 300ms, later acks are dropped.
		acks := 0
		w := newWorld(func(p actor.Packet) actor.Delivery {
			if _, ok := p.Command.(actor.StartAck); ok {
				acks++
				if acks == 1 {
					return actor.Delivery{Delay: 300 * ms}
				}
				return actor.Delivery{Drop: true}
			}
			return actor.Delivery{}
		}, 50*ms, 100*ms)
		w.ctrl.Start("r", "s")
		w.run(100 * ms)
		st, n := w.result("r", "s")
		fmt.Printf("    at deadline: status=%s attempts=%d\n", st, n)
		w.run(1000 * ms)
		st2, n2 := w.result("r", "s")
		fmt.Printf("    after late ack: status=%s attempts=%d starts=%d\n", st2, n2, w.worker.StartCount("s"))
		check(st == actor.TimedOut && st2 == actor.TimedOut && n2 == n, "late ack ignored, no retries after timeout")
		note("the controller never learns the sandbox did start; there is no late-ack state and no way to re-drive the request")
	})

	scenario("5. duplicate acks (first ack delayed, retry's ack arrives first)", func() {
		acks := 0
		w := newWorld(func(p actor.Packet) actor.Delivery {
			if _, ok := p.Command.(actor.StartAck); ok {
				acks++
				if acks == 1 {
					return actor.Delivery{Delay: 80 * ms}
				}
			}
			return actor.Delivery{Delay: 5 * ms}
		}, 50*ms, 500*ms)
		w.ctrl.Start("r", "s")
		w.run(500 * ms)
		st, n := w.result("r", "s")
		fmt.Printf("    status=%s attempts=%d starts=%d acks=%d\n", st, n, w.worker.StartCount("s"), acks)
		check(st == actor.Acknowledged && n == 2 && w.worker.StartCount("s") == 1, "second ack is a no-op")
	})

	scenario("6. zero-delay network: everything resolves at t=0", func() {
		w := newWorld(nil, 50*ms, 100*ms)
		w.ctrl.Start("r", "s")
		st, n := w.result("r", "s")
		fmt.Printf("    before running: status=%s attempts=%d\n", st, n)
		check(st == actor.Pending && n == 1, "Start sends once and nothing runs until the scheduler does")
		w.run(0)
		st, n = w.result("r", "s")
		fmt.Printf("    after RunUntil(t=0): status=%s attempts=%d now=%v\n", st, n, w.sched.Now().Sub(w.start))
		check(st == actor.Acknowledged && n == 1, "acknowledged without advancing time")
	})

	scenario("7. retryEvery == timeout and retryEvery > timeout", func() {
		for _, re := range []time.Duration{100 * ms, 150 * ms} {
			w := newWorld(func(p actor.Packet) actor.Delivery {
				_, ack := p.Command.(actor.StartAck)
				return actor.Delivery{Drop: ack}
			}, re, 100*ms)
			w.ctrl.Start("r", "s")
			w.run(1000 * ms)
			st, n := w.result("r", "s")
			fmt.Printf("    retryEvery=%v timeout=100ms: status=%s attempts=%d\n", re, st, n)
			check(st == actor.TimedOut && n == 1, "no retry when the first retry is not strictly before the deadline")
		}
	})

	scenario("8. reused request ID, second sandbox reaches the worker first", func() {
		// sandbox-a's request is slow (30ms), sandbox-b's is fast (1ms).
		w := newWorld(func(p actor.Packet) actor.Delivery {
			if r, ok := p.Command.(actor.StartRequest); ok && r.SandboxID == "a" {
				return actor.Delivery{Delay: 30 * ms}
			}
			return actor.Delivery{Delay: 1 * ms}
		}, 50*ms, 200*ms)
		w.ctrl.Start("req", "a")
		w.ctrl.Start("req", "b")
		w.run(200 * ms)
		sa, na := w.result("req", "a")
		sb, nb := w.result("req", "b")
		fmt.Printf("    a: %s/%d starts=%d   b: %s/%d starts=%d\n", sa, na, w.worker.StartCount("a"), sb, nb, w.worker.StartCount("b"))
		check(sb == actor.Acknowledged && sa == actor.Rejected && w.worker.StartCount("a") == 0 && w.worker.StartCount("b") == 1,
			"first to reach the worker wins; the controller's Start order does not decide")
		note("the winner depends on network delay, not on Start order (that is Day 7 territory, but it is already reachable with per-packet delay)")
	})

	scenario("9. reused request ID after the first request timed out", func() {
		acks := 0
		w := newWorld(func(p actor.Packet) actor.Delivery {
			if _, ok := p.Command.(actor.StartAck); ok {
				acks++
				return actor.Delivery{Drop: acks <= 3}
			}
			return actor.Delivery{Delay: 1 * ms}
		}, 50*ms, 120*ms)
		w.ctrl.Start("req", "a")
		w.run(150 * ms)
		sa, _ := w.result("req", "a")
		w.ctrl.Start("req", "b")
		w.run(400 * ms)
		sb, nb := w.result("req", "b")
		fmt.Printf("    a: %s starts=%d   b: %s/%d starts=%d\n", sa, w.worker.StartCount("a"), sb, nb, w.worker.StartCount("b"))
		check(sa == actor.TimedOut && sb == actor.Rejected && w.worker.StartCount("a") == 1 && w.worker.StartCount("b") == 0,
			"worker memory outlives the controller's deadline: the ID stays taken")
	})

	scenario("10. two request IDs for the same sandbox", func() {
		w := newWorld(fixed(1*ms), 50*ms, 100*ms)
		w.ctrl.Start("req-1", "s")
		w.ctrl.Start("req-2", "s")
		w.run(100 * ms)
		s1, _ := w.result("req-1", "s")
		s2, _ := w.result("req-2", "s")
		fmt.Printf("    req-1: %s  req-2: %s  starts=%d\n", s1, s2, w.worker.StartCount("s"))
		check(s1 == actor.Acknowledged && s2 == actor.Acknowledged, "both acknowledged")
		if w.worker.StartCount("s") == 2 {
			note("idempotency is keyed on request ID only: a new request ID starts the same sandbox again (starts=2). No test pins this down.")
		}
	})

	scenario("11. repeated Start with the same IDs sends nothing new", func() {
		w := newWorld(fixed(1*ms), 50*ms, 100*ms)
		w.ctrl.Start("r", "s")
		w.ctrl.Start("r", "s")
		w.ctrl.Start("r", "s")
		before := w.sends()
		w.run(100 * ms)
		st, n := w.result("r", "s")
		fmt.Printf("    sends before run=%d total sends=%d status=%s attempts=%d\n", before, w.sends(), st, n)
		check(before == 1 && n == 1, "one request packet for three Starts")
		note("the test only checks attempts==1 on the stored request; a mutant that replaces the entry on every Start passes it (see mutate C01)")
	})

	scenario("12. conflict reply lost, retried, conflict again", func() {
		conflicts := 0
		w := newWorld(func(p actor.Packet) actor.Delivery {
			if _, ok := p.Command.(actor.StartConflict); ok {
				conflicts++
				return actor.Delivery{Drop: conflicts == 1, Delay: 1 * ms}
			}
			return actor.Delivery{Delay: 1 * ms}
		}, 50*ms, 200*ms)
		w.ctrl.Start("req", "a")
		w.ctrl.Start("req", "b")
		w.run(200 * ms)
		sb, nb := w.result("req", "b")
		fmt.Printf("    b: %s attempts=%d conflicts sent=%d starts(b)=%d\n", sb, nb, conflicts, w.worker.StartCount("b"))
		check(sb == actor.Rejected && nb == 2 && w.worker.StartCount("b") == 0, "retry of a conflicting request is rejected again, never started")
	})

	scenario("13. stray reply for an unknown request", func() {
		w := newWorld(nil, 50*ms, 100*ms)
		w.ctrl.Start("r", "s")
		w.net.Send("worker", "controller", actor.StartAck{RequestID: "ghost", SandboxID: "s"})
		w.net.Send("worker", "controller", actor.StartAck{RequestID: "r", SandboxID: "other"})
		w.net.Send("worker", "controller", actor.StartConflict{RequestID: "r", SandboxID: "s"})
		w.run(100 * ms)
		st, n := w.result("r", "s")
		fmt.Printf("    status=%s attempts=%d\n", st, n)
		check(st == actor.Rejected && n == 1, "unknown keys ignored without panic; a forged Conflict for a known key is believed")
		note("the controller trusts any reply whose (requestID, sandboxID) it knows; it does not check the sender")
	})

	scenario("14. Start before the worker is registered", func() {
		sched := sim.NewScheduler(time.Unix(0, 0))
		net := actor.NewSimNetwork(sched, nil)
		ctrl := actor.NewSandboxController(sched, net, "controller", "worker", 50*ms, 100*ms)
		func() {
			defer func() { fmt.Printf("    panic: %v\n", recover()) }()
			ctrl.Start("r", "s")
		}()
		_, n := ctrl.Result("r", "s")
		fmt.Printf("    request registered with attempts=%d and a deadline already scheduled\n", n)
		note("Start panics from SimNetwork.Send after registering the request and its deadline; construction order matters")
	})

	scenario("15. mailbox nil checks", func() {
		func() {
			defer func() { fmt.Printf("    NewMailbox(sched, nil): panic=%v\n", recover()) }()
			actor.NewMailbox(sim.NewScheduler(time.Unix(0, 0)), nil)
		}()
		func() {
			defer func() { fmt.Printf("    NewMailbox(nil, h).Send: panic=%v\n", recover()) }()
			m := actor.NewMailbox(nil, func(actor.Command) {})
			fmt.Println("    NewMailbox(nil, h): no panic at construction")
			m.Send(actor.PutCommand{Key: "k", Value: "v", TTL: time.Second})
		}()
		note("nil handler is rejected at construction, nil scheduler only at first Send (pre-existing)")
	})

	scenario("16. determinism: seeded lossy network, two runs, identical traces", func() {
		trace := func(seed int64) string {
			rng := rand.New(rand.NewSource(seed))
			w := newWorld(actor.SeededDeliveryPolicy(rng, 40, 30*ms), 20*ms, 150*ms)
			ids := []string{"a", "b", "c", "d", "e", "f"}
			for i, id := range ids {
				w.ctrl.Start(fmt.Sprintf("req-%d", i%4), id) // some IDs reused on purpose
			}
			w.run(300 * ms)
			s := ""
			for i, id := range ids {
				st, n := w.result(fmt.Sprintf("req-%d", i%4), id)
				s += fmt.Sprintf("%s:%s/%d/%d ", id, st, n, w.worker.StartCount(id))
			}
			for _, e := range w.net.Events() {
				s += fmt.Sprintf("%d%s@%d ", e.ID, e.Kind, e.At.Sub(w.start)/ms)
			}
			return s
		}
		same := 0
		for seed := int64(1); seed <= 50; seed++ {
			if trace(seed) == trace(seed) {
				same++
			}
		}
		fmt.Printf("    %d/50 seeds reproduce exactly\n", same)
		check(same == 50, "same seed, same trace")
		fmt.Printf("    seed 7: %s\n", trace(7)[:120])
	})

	scenario("17. retry timer fires once after settlement, then stops (no leaked retries)", func() {
		w := newWorld(fixed(1*ms), 10*ms, 1000*ms)
		w.ctrl.Start("r", "s")
		w.run(1000 * ms)
		st, n := w.result("r", "s")
		fmt.Printf("    status=%s attempts=%d sends=%d\n", st, n, w.sends())
		check(st == actor.Acknowledged && n == 1 && w.sends() == 2, "exactly one request and one ack on the wire over 1s")
	})

	if bugs > 0 {
		fmt.Printf("\n%d BUG line(s)\n", bugs)
		os.Exit(1)
	}
	fmt.Println("\nno BUG lines; see NOTE lines for design questions")
}
