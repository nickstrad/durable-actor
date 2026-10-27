# durable-actor

A 12-day learning course on deterministic simulation testing (DST) in Go. Each day adds one
implementation step and one DST capability. The code is meant to grow one lesson at a time, so
the history stays readable as a course.

## Course plan

| Day | Implementation                         | DST capability gained             |
|-----|----------------------------------------|-----------------------------------|
| 1   | Injectable clock and expiring keys     | Virtual time                      |
| 2   | Deterministic event scheduler          | Control execution order           |
| 3   | Actor mailbox and command processing   | Model sequential execution        |
| 4   | Seeded random command generation       | Reproducible test sequences       |
| 5   | Simulated network messages             | Inject latency and message loss   |
| 6   | Retries, deadlines, idempotency        | Test unreliable delivery          |
| 7   | Network partitions and reordering      | Simulate distributed failures     |
| 8   | Persistent operation log               | Model durability and recovery     |
| 9   | Simulated disk I/O and crashes         | Inject storage failures           |
| 10  | SQLite storage implementation          | Validate against a real database  |
| 11  | Property-based tests and Go fuzzing    | Discover invariant violations     |
| 12  | Trace replay and failure minimization  | Reproduce and debug failures      |

**Current position:** Days 1–6 are done (`sim/clock.go`: `ManualClock`; `actor/actor.go`: `Clock`, TTL `Actor`;
`sim/scheduler.go`: virtual-time `Scheduler`; `actor/mailbox.go`: `MailboxActor`; `actor/workload_test.go`:
seeded workload against a reference model; `actor/network.go`: `SimNetwork` with latency and loss;
`actor/retries.go`: `SandboxController` retries with deadlines, idempotent `SandboxWorker`).
Update this line as each day lands.

## Core design rule: single-threaded simulator

The simulator is single-threaded on purpose. Deterministic simulation is far easier when
concurrency is modeled as scheduled events rather than left to uncontrolled goroutines.

- No goroutines, channels, `sync` primitives, `time.Sleep`, timers, or tickers in simulated code.
  Concurrency is represented as events on the `Scheduler`.
- No wall-clock time (`time.Now`) in simulated code. Time comes from an injected `Clock`.
- No unseeded randomness. Once Day 4 lands, all randomness flows from one explicit seed.
- Do not iterate over maps where the order affects behavior (Go randomizes map order).
- The same seed and inputs must always produce the same sequence of events and results.

## Staying on pace with the course

- Work only on the current day's step. Do not build features from later days early, even when
  a review suggests them. For example, background expiry sweeps, a mailbox, networking,
  persistence, or a real database each belong to their own day.
- Keep earlier days' constructs (such as `ManualClock`) unless the current day's lesson
  replaces them. Folding them together is a lesson decision, not a cleanup.
- Prefer the simplest code that teaches the day's concept. Avoid performance work
  (custom heaps, allocation tuning) unless it serves the lesson.
- Each day's change ships with tests that show the new DST capability working.
- If a suggested change looks like it belongs to a later day or goes against the
  single-threaded rule, say so and ask before doing it.

## Git

- Work directly on `main`. Do not create feature branches or worktrees for this project; this
  overrides any global "work in a branch" rule.
- Commit only when asked. One lesson step per commit, subject `<area>: <imperative summary>`.

## Go conventions

- Run `make check` (gofmt, `go vet`, and `make test`, which runs every test 100 times in
  shuffled order) before finishing any change.
- Standard library only, unless a day calls for a dependency (e.g. SQLite on Day 10).

## Ad hoc scripts

- Before writing a script to validate, test, debug, or investigate a change, read
  `agent_work/index.md` and reuse what is there.
- Put every one-off script in `agent_work/` and add it to the table in `agent_work/index.md`
  (what it does, exact command, date). Follow the rules at the top of that file.
- Go for anything non-trivial, bash for basic terminal work. No Python.
