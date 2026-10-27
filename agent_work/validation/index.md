# Validation scripts

Scripts that check whether the `actor/` tests catch real bugs, rather than
passing because they mirror the code. Results of past runs are in
[results.md](results.md).

Every script works on a temp copy of the repo and never modifies the working
tree. Go scripts are ordinary `package main` directories in the module, so
`make check` builds and vets them; they share `vlib/`. Run them from the repo
root. Mutants match on code only, never on comments, so editing a comment
doesn't break them.

The repo has no goroutines (single-threaded simulator) and `make test` already
runs every test 100 times shuffled, so there is no race-detector flakiness
script here.

## Scripts

| Script | What it does | Command |
|---|---|---|
| `mutate/` | Applies one small bug at a time to `actor/mailbox.go`, `kv_mailbox.go`, `worker.go`, or `controller.go`, runs `go test ./actor/`, and prints a killed/survived table. Exits 1 if a mutant survives, 2 if a mutant no longer matches the code exactly once. `-matrix` runs every mutant under every test separately and prints which mutants each test kills uniquely. `-probe` also runs `probe/` against each mutant. | `go run ./agent_work/validation/mutate` (see `-h` for `-run`, `-only`, `-matrix`, `-tests`, `-probe`) |
| `probe/` | Drives `SandboxController`/`SandboxWorker` through edge timings the tests skip (reply exactly at the deadline, reply tied with a retry, late and duplicate acks, zero-delay network, `retryEvery >= timeout`, reused IDs in several orders, determinism) and prints `ok`/`NOTE`/`BUG` lines. Exits 1 on any `BUG`. | `go run ./agent_work/validation/probe` |
| `vlib/` | Shared helpers: `CopyRepo`, `Replace`, `GoTest`, `GoRun`, `ListTests`, `Summarize`. Not run directly. | - |
