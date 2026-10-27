# agent_work

Ad hoc scripts written while working on this repo: validation, debugging, investigation,
benchmarks, data or trace inspection, and other one-off checks. Check here before writing a
new script; reuse or extend an existing one when it fits.

## Rules

- **Languages:** Go for anything non-trivial; bash for basic terminal work (copying files,
  looping over `go test`). No Python.
- **Go scripts:** one `package main` directory per script, e.g. `agent_work/<topic>/<name>/main.go`,
  run with `go run ./agent_work/<topic>/<name>`. They are part of the module, so `make check`
  (gofmt, `go vet`) covers them and they must keep it passing.
- **Bash scripts:** start with `#!/usr/bin/env bash` and `set -euo pipefail`, and are executable.
- **Never mutate the working tree.** Scripts that change code (mutation testing, experiments)
  copy the repo to `mktemp -d` and work there.
- **Group by topic** in subfolders (e.g. `validation/`, `debug/`, `bench/`) once there is more
  than one script on a topic.
- **Record every script below:** what it does, the exact command to run it, and the date added.
  If a script produced findings worth keeping, add a short `index.md` in its subfolder with the
  results and link it here.
- Scripts are tools, not simulator code: the single-threaded simulation rules in `AGENTS.md`
  apply to `actor/` and `sim/`, not here. A script that drives the simulator must still use it
  through its seeded, deterministic API.

## Scripts

| Script | What it does | Command | Added |
|--------|--------------|---------|-------|
| `validation/mutate/` | Mutation testing for the `actor/` mailbox, key/value handler, worker, and controller on a temp copy: killed/survived table; `-matrix` for per-test unique kills; `-probe` to also run the probe against each mutant. Results in [validation/results.md](validation/results.md). | `go run ./agent_work/validation/mutate` / `go run ./agent_work/validation/mutate -matrix` | 2026-10-26 |
| `validation/probe/` | Edge-timing probe for Day 6 retries (reply at deadline, tie with retry, late/duplicate acks, zero delay, `retryEvery >= timeout`, reused IDs, determinism). Prints ok/NOTE/BUG lines. | `go run ./agent_work/validation/probe` | 2026-10-26 |
| `validation/vlib/` | Shared helpers for the validation scripts (copy repo, replace, go test, list tests). Not run directly. | - | 2026-10-26 |
