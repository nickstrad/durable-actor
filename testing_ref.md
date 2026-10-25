# Testing reading list

Books and other sources on model testing, property-based testing, fuzzing,
deterministic simulation testing (DST), and SQLite-style rigor.

They are in the order the course uses each topic, so each one is useful
around the day it covers. Each one also gives you terms the next one uses.

As far as we know there is no full book on DST, so that topic is covered
by talks, papers, and engineering docs instead.

## Reading order

### 1. *Property-Based Testing with PropEr, Erlang, and Elixir*, Fred Hebert (Pragmatic, 2019)

**Course days:** 4, 11, 12

- The best explanation of stateful property testing: generate random
  commands, run them against the real system and a model, compare. This is
  what `actor/workload_test.go` does.
- Also covers shrinking, which means cutting a failing case down to the
  smallest one that still fails.
- The examples are in Erlang, but the ideas carry over directly to Go.

### 2. *Designing Data-Intensive Applications*, Martin Kleppmann, chapters 8–9

**Course days:** 5–9

- Chapter 8, "The Trouble with Distributed Systems", lists what goes wrong:
  lost and delayed messages, pauses, clocks that disagree, partitions.
- It tells you which faults the simulator should inject, and why each one
  matters.

### 3. Deterministic simulation testing sources (not books)

**Course days:** 5–9, read after item 2

- **Will Wilson, "Testing Distributed Systems w/ Deterministic Simulation"
  (Strange Loop 2014).** The talk that made the idea known. It describes
  FoundationDB's single-threaded simulator, the same design rule as this
  project's `AGENTS.md`.
- **"FoundationDB: A Distributed Unbundled Transactional Key Value Store"
  (SIGMOD 2021)**, especially the section on simulation testing.
- **TigerBeetle's docs and blog posts on VOPR**, their simulator. It's a
  modern example that also injects storage faults.
- **The Antithesis blog.** Antithesis sells DST as a product, and the
  blog explains the ideas well.

### 4. *Database Internals*, Alex Petrov

**Course days:** 8–10

- Part I covers storage engines, write-ahead logs, and crash recovery.
  That's what the persistent operation log and the crash simulation model.

### 5. *The Fuzzing Book*, Andreas Zeller et al. (free at fuzzingbook.org)

**Course day:** 11

- Hands-on, in Python: random fuzzing, coverage-guided fuzzing (the kind
  Go's built-in fuzzer does), and fuzzing driven by grammars.
- It comes after property testing because a fuzzer is "property testing
  where the computer also searches for inputs that reach new code".

### 6. *Why Programs Fail*, Andreas Zeller (or its free successor, *The Debugging Book*)

**Course day:** 12

- Its main subject is delta debugging, the standard way to shrink a failing
  input or trace to the smallest version that still fails. That is what
  Day 12's failure minimization is.

### 7. SQLite-style rigor: *Introduction to Software Testing*, Ammann & Offutt, plus "How SQLite Is Tested"

**When:** after the course

- No book covers SQLite's testing; the main source is
  <https://sqlite.org/testing.html>. It describes:
  - 100% MC/DC branch coverage, the avionics standard
  - several independent test harnesses
  - injecting out-of-memory and I/O errors at every possible point
  - crash simulation
  - fuzzing
- Ammann & Offutt is the textbook that explains the theory behind it:
  coverage criteria, and MC/DC in particular.
- It comes last because it's about being thorough. The earlier items teach
  the techniques, and this one shows how far you can take them.

### 8. *Practical TLA+*, Hillel Wayne (Apress, 2018)

**When:** optional, after the course

- DST tests the code you wrote. TLA+ checks the design before you write any
  code, by exploring every possible interleaving of a small model.
- It's the formal version of the "model" idea from item 1, which is why it
  comes at the end.

## Optional references

- ***Fuzzing for Software Security Testing and Quality Assurance***,
  Takanen, DeMott, and Miller. Fuzzing from the security side.
- ***Practical Model-Based Testing***, Utting and Legeard. The older,
  industrial version of model-based testing. The tooling is dated, but the
  theory is sound.
- **John Hughes, "Experiences with QuickCheck: Testing the Hard Stuff and
  Staying Sane"** (paper and talk). Short, and a good motivator before
  item 1.
- **For Go:** the official fuzzing guide at
  <https://go.dev/doc/security/fuzz>, and the `pgregory.net/rapid`
  library's docs. The course is standard library only, but rapid is still
  worth reading about.

## If you only read three

1. Hebert (item 1)
2. Wilson's talk and the FoundationDB paper (item 3)
3. *The Fuzzing Book* (item 5)

Together they cover model testing, DST, and fuzzing.

---

Editions and availability were not checked when this list was written.
*Designing Data-Intensive Applications* may have a newer edition.
