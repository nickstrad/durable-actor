# Validation results

Results of runs of the scripts listed in [index.md](index.md). The scripts
print their own tables; this file keeps only what each run found.

## Run 1: Day 6 retries, deadlines, idempotency (2026-10-26)

Scope: the uncommitted Day 6 change. `actor/mailbox.go` gained
`NewMailbox(sched, handle)`; `actor/retries.go` added `SandboxWorker`
(dedups by request ID, counts starts, replies Ack/Conflict) and
`SandboxController` (one deadline event per request, a retry every
`retryEvery` while pending, late/unknown replies ignored); three tests in
`actor/retries_test.go`.

### Probe (`go run ./agent_work/validation/probe`)

17 scenarios, no `BUG` lines: nothing contradicts what the code's comments
promise, and 50/50 seeds of a lossy seeded network reproduce the same trace.
The `NOTE` lines are design questions, not bugs:

| # | Scenario | Observed | Verdict |
|---|---|---|---|
| 1 | Reply lands exactly at the deadline (RTT == timeout) | `timed_out`, 1 attempt | By design: the deadline is exclusive. 1ns of slack (scenario 2) flips it to `acknowledged`. Untested. |
| 3 | Reply lands exactly when a retry is due (RTT == retryEvery) | `acknowledged`, **2 attempts**, 1 start | Surprising: the retry was scheduled first, so it fires before the ack that arrives at the same instant. One spurious resend; harmless because the worker dedups. Untested. |
| 4 | Ack arrives after the deadline | stays `timed_out`; no retries after the deadline | By design and the lesson's point. The controller never learns the start happened and there is no way to re-drive a timed-out request. **Untested** (see mutants C19/C21). |
| 7 | `retryEvery >= timeout` | `timed_out`, 1 attempt | No retry fires at or after the deadline; the deadline event wins the same-instant tie with the retry event. Untested (see C07). |
| 8 | Reused request ID; the second sandbox's request reaches the worker first (per-packet delay) | `b` acknowledged, `a` rejected | The winner is whoever reaches the worker first, not the `Start` order. Reachable today with per-packet delay; reordering proper is Day 7. |
| 9 | Reused request ID after the first request timed out | `b` rejected, `a` started once | Worker memory outlives the controller's deadline. By design. |
| 10 | Two request IDs for the same sandbox | both acknowledged, **starts = 2** | Idempotency is keyed on request ID only. Arguably correct (client-chosen idempotency key), but no test pins it (W12 survives). |
| 11 | Three `Start` calls with the same IDs | 1 request packet | Correct, but the test only checks `attempts == 1` on the stored request, which a mutant that replaces the entry on every `Start` also satisfies (C01 survives). |
| 13 | Stray replies for unknown keys; a forged `Conflict` for a known key | unknown ignored, forged conflict believed | The controller trusts any reply whose key it knows; it does not check `From`. Fine for Day 6. |
| 14 | `Start` before the worker is registered | panics from `SimNetwork.Send` after the request and its deadline are already registered | Construction order matters; a half-registered request is left behind. Minor. |
| 15 | `NewMailbox(sched, nil)` / `NewMailbox(nil, h)` | nil handler panics at construction; nil scheduler only at first `Send` | Pre-existing asymmetry, not from today. |

No real bugs found in the simulated code.

### Mutation testing (`go run ./agent_work/validation/mutate`)

50 mutants over `actor/mailbox.go` (10) and `actor/retries.go` (40), whole
`./actor/` package: **killed 39, survived 11**.

Survivors, with the smallest test that would kill each (verified with
`mutate -probe`, which runs the probe against every mutant):

| Mutant | What it breaks | Minimal killing test | Worth adding? |
|---|---|---|---|
| C01 Start is not a no-op for an existing request | A repeated `Start` replaces the entry and sends again; the orphaned request keeps retrying | After three `Start("r","s")`, count `StartRequest` packets in `net.Events()` (or sends) and require 1 (probe scenario 11 kills it) | **Yes.** `TestReusedRequestIDIsRejected` says it checks this ("sends nothing new") but only checks `attempts == 1` on the stored entry. |
| C19 receive: late replies overwrite TimedOut | A late ack flips `timed_out` to `acknowledged` | Delay the first ack past the deadline, drop the rest; require `timed_out` before and after the ack lands (scenario 4) | **Yes.** This is the "ignore late replies" rule in `receive` and the heart of `TestTimeoutDoesNotProveFailure`, but that test drops every ack so the rule is never exercised. |
| C21 receive: replies only overwrite TimedOut (not Pending) | Same gap as C19, narrower | Same test | Covered by the C19 test. |
| C07 deadline scheduled after the first send | At `retryEvery == timeout` the retry fires before the deadline and sends one more attempt | `retryEvery == timeout` with all acks dropped; require 1 attempt (scenario 7) | **Yes, cheap.** The comment in `Start` explains this ordering; nothing checks it. Note: a *reply* can never tie-break ahead of the deadline whatever the order, because delivery events are scheduled later; only the retry event can. |
| C20 receive: unknown replies dereference nil | Panic on a reply for an unknown key | Send a `StartAck` for an unknown request ID to the controller; require no panic (scenario 13) | Maybe. Only a buggy or foreign worker produces one. |
| W12 StartCount caps at 1 | Equivalent for the current tests: no test ever expects a count above 1 | Two request IDs for one sandbox; require `StartCount == 2` (scenario 10) | Only if the "request ID is the idempotency key, not sandbox ID" semantics are meant to be a lesson point. |
| C24 Result returns (Pending, 0) for unknown request | Drops the panic | `Result` on an unknown key must panic | Low value. |
| C26 constructor allows non-positive intervals | Drops argument validation | Panic test | Low value. |
| C27 Start allows empty IDs | Drops argument validation | Panic test | Low value. |
| MB06 NewMailbox allows nil handler | Drops the nil check | Panic test | Low value. |
| MB08 kv handler: Get reply called even when nil | Nil `Reply` on `GetCommand` panics | Send a `GetCommand` with nil `Reply` | Low value; nothing sends one. |

Two mutants were initially "killed" by a build failure (an unused variable)
and one iterated a map so its result depended on map order; they were
rewritten so that every kill is a real test failure. One (the first W13) was
an equivalent mutant and was replaced.

### Test value (`go run ./agent_work/validation/mutate -matrix`)

Every mutant run under every `actor/` test separately (`go test -run '^Name$'`).
A mutant killed by exactly one test is a "unique" kill for that test; a test
with no unique kills is a redundancy candidate. Same 11 survivors as the
whole-package run.

| Test | Kills | Unique | Unique kills | Verdict |
|---|---|---|---|---|
| TestLostAckRetriesWithoutDoubleStarting | 21 | 0 | - | **No unique kills.** Everything it catches is also caught by `TestTimeoutDoesNotProveFailure` (retry/attempt mutants C06, C09, C14) or `TestReusedRequestIDIsRejected` (reply-path mutants W08, W09, C04, C15, C17, C22) or `TestMailboxIsDeferredAndFIFO` (MB02). It is still the only test of the happy path "lost ack, retry, one start", which is the lesson's headline, so keep it, but give it an assertion the others lack: e.g. count `StartRequest`/`StartAck` packets in `net.Events()` (2 sends, 1 drop, 1 delivered ack), or add the late/duplicate-ack check (kills C19/C21). |
| TestTimeoutDoesNotProveFailure | 19 | 5 | C03 no deadline, C05 wrong timeout status, C08 deadline off by one interval, C11 retries after timeout, C12 retry interval drifts | **Helpful.** Owns the deadline and retry-cadence behavior. Its one blind spot is that it drops every ack, so "late replies are ignored" (C19/C21) is never exercised; a delayed-not-dropped ack would fix that. |
| TestReusedRequestIDIsRejected | 25 | 7 | W03 no conflict, W07 conflict names wrong sandbox, W10 dedup by sandbox ID, W13 ack instead of conflict, C18/C23 conflict handling, C25 Result ignores sandbox ID | **Helpful**, highest unique count. Its third `Start` ("sends nothing new") is not actually verified (C01 survives). |
| TestMailboxIsDeferredAndFIFO | 7 | 1 | MB04 drain whole inbox in one event | Helpful; the only test that pins one-command-per-step. Covers today's handler dispatch (MB01/MB02/MB03/MB07/MB09). |
| TestSameTimeArrivalsPreserveOrder | 4 | 0 | - | Redundant for mutation purposes (subset of FIFO test's kills), but it reads as a distinct lesson (tie-breaking). |
| TestCallbackEnqueuesAfterExistingWork | 5 | 0 | - | Redundant by this metric; MB05 (handler at Send time) is also killed by the FIFO test. |
| TestActorRandomizedAgainstModel | 4 | 1 | MB10 Put uses a fixed TTL | Helpful; the only test that checks the TTL argument flows through the new handler. |
| TestNetwork* (3 of 4), TestWorkload* | 0-3 | 0 | - | Not aimed at today's code; they kill the mailbox-dispatch mutants incidentally. |
| TestActorTTL* (3), TestSeededNetworkIsReproducible, TestWorkloadReproducible | 0 | 0 | - | Do not touch today's code. |

Totals: 15 tests, 50 mutants, 39 killed. The three Day 6 tests together kill
every retries.go mutant that any test kills; the older tests add nothing
there. For the mailbox change, `TestMailboxIsDeferredAndFIFO` and
`TestActorRandomizedAgainstModel` do the work.

### Suggested test additions (not applied; actor/ was not edited)

1. Late ack after the deadline stays `timed_out` (kills C19, C21). Smallest
   change: in `TestTimeoutDoesNotProveFailure`, delay the first ack by more than
   the timeout instead of dropping it, run past its arrival, and check the
   status did not change.
2. Repeated `Start` sends no packet (kills C01): count `NetworkSend` events
   with a `StartRequest` after three identical `Start` calls, or compare
   `len(net.Events())` before and after the repeats.
3. `retryEvery == timeout` gives exactly one attempt (kills C07): the deadline
   wins the same-instant tie with the retry; this is what the ordering comment
   in `Start` promises.
4. Optional, design-pinning: two request IDs for one sandbox start it twice
   (kills W12); a stray ack for an unknown request does not panic (kills C20).

### Commands

    go run ./agent_work/validation/probe
    go run ./agent_work/validation/mutate
    go run ./agent_work/validation/mutate -probe
    go run ./agent_work/validation/mutate -matrix
    go run ./agent_work/validation/mutate -matrix -tests 'Retries|Timeout|Reused'

## Follow-up (2026-10-26, commit 6b84766)

The run above predates two test changes made from these findings:

- `TestReusedRequestIDIsRejected` now counts `StartRequest` packets (kills C01).
- New `TestLateAckDoesNotOverrideTimeout`: acks arrive after the deadline and
  `retryEvery == timeout` (kills C07, C19, C21).

Rerun: **killed 43, survived 7**. Survivors: MB06, MB08, C24, C26, C27
(panic-on-bad-input guards), C20 (unknown reply; unreachable today), and W12
(two request IDs for one sandbox start it twice; dedup is by request ID by
design). The `-matrix` numbers above were not rerun.
