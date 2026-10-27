// Command mutate runs mutation testing against ./actor/ on a temp copy of
// the repo. Each mutant is a small set of old->new string replacements in
// non-test code. A mutant is "killed" when go test fails (an assertion, a
// panic, or a timeout) and "survived" when it passes. A mutant whose old
// string is not found exactly once is an ERROR: the code under test has
// drifted and the mutant list needs updating. Exit status is 1 if any mutant
// survived and 2 if any mutant failed to apply.
//
// Mutants match on code only, never on comments, so a comment edit does not
// break them.
//
//	go run ./agent_work/validation/mutate [-repo .] [-run REGEXP] [-only REGEXP]
//	go run ./agent_work/validation/mutate -matrix [-tests REGEXP]
//
// -matrix runs every mutant under every test separately (go test -run
// '^Name$') and prints a kill matrix plus, per test, the mutants only that
// test kills. A test with no unique kills is a redundancy candidate.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/nickstrad/durable-actor/agent_work/validation/vlib"
)

const pkg = "./actor/"

type mutant struct {
	Name  string
	File  string      // relative to actor/
	Pairs [][2]string // match text is code only, never comments
}

const (
	workerSeen     = "if previous, seen := w.handled[req.RequestID]; seen {"
	workerConflict = "\t\tif previous != req.SandboxID {\n"
	workerStore    = "\t\tw.handled[req.RequestID] = req.SandboxID\n"
	workerStart    = "w.starts[req.SandboxID]++"
	workerReply    = "\tw.net.Send(w.address, req.ReplyTo, reply)\n"
	ctrlNoop       = "\tif _, exists := c.requests[key]; exists {\n\t\treturn\n\t}\n"
	ctrlDeadline   = "\tc.sched.After(c.timeout, func() {\n\t\tif r.status == StartPending {\n\t\t\tr.status = StartTimedOut\n\t\t}\n\t})\n\n\tc.send(r)\n"
	ctrlRetry      = "\tc.sched.After(c.retryEvery, func() {\n\t\tif r.status == StartPending {\n\t\t\tc.send(r)\n\t\t}\n\t})\n"
	ctrlFilter     = "\tif !ok || r.status != StartPending {\n\t\treturn\n\t}\n"
	ctrlResultOK   = "\tif !ok {\n\t\tpanic(\"unknown request\")\n\t}\n"
	mbHandle       = "\ta.handle(cmd)\n"
	mbNext         = "\tif len(a.inbox) > 0 {\n\t\ta.sched.After(0, a.processOne)\n\t} else {\n\t\ta.pumpScheduled = false\n\t}\n"
	mbReply        = "\t\t\tif c.Reply != nil {\n\t\t\t\tc.Reply(value, found)\n\t\t\t}\n"
)

var mutants = []mutant{
	// mailbox.go: handler dispatch and pump; kv_mailbox.go: key/value handler.
	{"MB01 mailbox never calls handler", "mailbox.go", [][2]string{{mbHandle, "\t_ = cmd\n"}}},
	{"MB02 mailbox calls handler twice", "mailbox.go", [][2]string{{mbHandle, mbHandle + mbHandle}}},
	{"MB03 mailbox handles in LIFO order", "mailbox.go", [][2]string{{
		"\tcmd := a.inbox[0]\n\ta.inbox = a.inbox[1:]\n",
		"\tcmd := a.inbox[len(a.inbox)-1]\n\ta.inbox = a.inbox[:len(a.inbox)-1]\n"}}},
	{"MB04 mailbox drains whole inbox in one event", "mailbox.go", [][2]string{{mbNext,
		"\tfor len(a.inbox) > 0 {\n\t\tcmd := a.inbox[0]\n\t\ta.inbox = a.inbox[1:]\n\t\ta.handle(cmd)\n\t}\n\ta.pumpScheduled = false\n"}}},
	{"MB05 mailbox runs handler at Send time", "mailbox.go", [][2]string{
		{"\ta.inbox = append(a.inbox, cmd)\n", "\ta.handle(cmd)\n\treturn\n"},
		{"\tif !a.pumpScheduled {\n\t\ta.pumpScheduled = true\n\t\ta.sched.After(0, a.processOne)\n\t}\n", "\n"}}},
	{"MB06 NewMailbox allows nil handler", "mailbox.go", [][2]string{{"\tif handle == nil {\n\t\tpanic(\"nil handler\")\n\t}\n", "\n"}}},
	{"MB07 kv handler: Get reports found=false", "kv_mailbox.go", [][2]string{{"c.Reply(value, found)", "c.Reply(value, found && false)"}}},
	{"MB08 kv handler: Get reply called even when nil", "kv_mailbox.go", [][2]string{{mbReply, "\t\t\tc.Reply(value, found)\n"}}},
	{"MB09 kv handler: Put ignores value", "kv_mailbox.go", [][2]string{{"state.Put(c.Key, c.Value, c.TTL)", "state.Put(c.Key, \"\", c.TTL)"}}},
	{"MB10 kv handler: Put uses a fixed TTL", "kv_mailbox.go", [][2]string{{"state.Put(c.Key, c.Value, c.TTL)", "state.Put(c.Key, c.Value, time.Hour)"}}},

	// worker.go: worker dedup, conflict, start counting, reply.
	{"W01 worker never dedups (seen is ignored)", "worker.go", [][2]string{{workerSeen, "if previous, seen := w.handled[req.RequestID]; seen && false {"}}},
	{"W02 worker never records request ID", "worker.go", [][2]string{{workerStore, "\t\t_ = req.RequestID\n"}}},
	{"W03 worker never reports conflict", "worker.go", [][2]string{{workerConflict, "\t\tif previous != req.SandboxID && false {\n"}}},
	{"W04 worker reports conflict on same-sandbox retry (== not !=)", "worker.go", [][2]string{{workerConflict, "\t\tif previous == req.SandboxID {\n"}}},
	{"W05 worker never counts a start", "worker.go", [][2]string{{workerStart, "w.starts[req.SandboxID] += 0"}}},
	{"W06 worker counts a start on every request", "worker.go", [][2]string{
		{workerStart, "_ = 0"},
		{"\tvar reply Command = StartAck{", "\tw.starts[req.SandboxID]++\n\tvar reply Command = StartAck{"}}},
	{"W07 worker conflict reply names the first sandbox", "worker.go", [][2]string{{
		"reply = StartConflict{RequestID: req.RequestID, SandboxID: req.SandboxID}",
		"reply = StartConflict{RequestID: req.RequestID, SandboxID: previous}"}}},
	{"W08 worker never replies", "worker.go", [][2]string{{workerReply, "\t_ = reply\n"}}},
	{"W09 worker replies to itself", "worker.go", [][2]string{{workerReply, "\tw.net.Send(w.address, w.address, reply)\n"}}},
	{"W10 worker dedups on sandbox ID instead of request ID", "worker.go", [][2]string{
		{workerSeen, "if previous, seen := w.handled[req.SandboxID]; seen {"},
		{workerStore, "\t\tw.handled[req.SandboxID] = req.SandboxID\n"}}},
	{"W11 worker conflict also starts the sandbox", "worker.go", [][2]string{{workerConflict,
		"\t\tw.starts[req.SandboxID]++\n" + workerConflict}}},
	{"W12 StartCount caps at 1", "worker.go", [][2]string{{"\treturn w.starts[sandboxID]\n", "\treturn min(w.starts[sandboxID], 1)\n"}}},
	{"W13 worker acks a conflicting request with the first sandbox's ID", "worker.go", [][2]string{{
		"reply = StartConflict{RequestID: req.RequestID, SandboxID: req.SandboxID}",
		"reply = StartAck{RequestID: req.RequestID, SandboxID: previous}"}}},

	// controller.go: controller Start, deadline, retry.
	{"C01 Start is not a no-op for an existing request", "controller.go", [][2]string{{ctrlNoop, "\n"}}},
	{"C02 Start never registers the request", "controller.go", [][2]string{{"\tc.requests[key] = r\n", "\n"}}},
	{"C03 deadline never fires (no timeout event)", "controller.go", [][2]string{{ctrlDeadline, "\tc.send(r)\n"}}},
	{"C04 deadline marks TimedOut regardless of status", "controller.go", [][2]string{{ctrlDeadline,
		"\tc.sched.After(c.timeout, func() {\n\t\tr.status = StartTimedOut\n\t})\n\n\tc.send(r)\n"}}},
	{"C05 deadline marks Rejected instead of TimedOut", "controller.go", [][2]string{{"\t\t\tr.status = StartTimedOut\n", "\t\t\tr.status = StartRejected\n"}}},
	{"C06 deadline uses retryEvery", "controller.go", [][2]string{{"\tc.sched.After(c.timeout, func() {\n", "\tc.sched.After(c.retryEvery, func() {\n"}}},
	{"C07 deadline scheduled after the first send (reply at deadline wins)", "controller.go", [][2]string{{ctrlDeadline,
		"\tc.send(r)\n\n\tc.sched.After(c.timeout, func() {\n\t\tif r.status == StartPending {\n\t\t\tr.status = StartTimedOut\n\t\t}\n\t})\n"}}},
	{"C08 deadline is one retry interval longer", "controller.go", [][2]string{{"\tc.sched.After(c.timeout, func() {\n", "\tc.sched.After(c.timeout+c.retryEvery, func() {\n"}}},
	{"C09 never retries", "controller.go", [][2]string{{ctrlRetry, "\n"}}},
	{"C10 retries regardless of status", "controller.go", [][2]string{{ctrlRetry, "\tc.sched.After(c.retryEvery, func() {\n\t\tc.send(r)\n\t})\n"}}},
	{"C11 retries after timeout too (!= Acknowledged)", "controller.go", [][2]string{{ctrlRetry,
		"\tc.sched.After(c.retryEvery, func() {\n\t\tif r.status != StartAcknowledged && r.status != StartRejected {\n\t\t\tc.send(r)\n\t\t}\n\t})\n"}}},
	{"C12 retry interval doubles each attempt", "controller.go", [][2]string{{"\tc.sched.After(c.retryEvery, func() {\n", "\tc.sched.After(c.retryEvery*time.Duration(r.attempts), func() {\n"}}},
	{"C13 attempts not counted", "controller.go", [][2]string{{"\tr.attempts++\n", "\n"}}},
	{"C14 attempts not counted for retries", "controller.go", [][2]string{{"\tr.attempts++\n", "\tif r.attempts == 0 {\n\t\tr.attempts++\n\t}\n"}}},
	{"C15 request sent with ReplyTo = worker", "controller.go", [][2]string{{"\t\tReplyTo:   c.address,\n", "\t\tReplyTo:   c.worker,\n"}}},
	{"C16 request sent with empty sandbox ID", "controller.go", [][2]string{{"\t\tSandboxID: r.key.sandboxID,\n\t\tReplyTo:", "\t\tSandboxID: \"\",\n\t\tReplyTo:"}}},

	// controller.go: controller receive and Result.
	{"C17 receive: Ack and Conflict statuses swapped", "controller.go", [][2]string{
		{"key, status = requestKey{m.RequestID, m.SandboxID}, StartAcknowledged", "key, status = requestKey{m.RequestID, m.SandboxID}, StartRejected"},
		{"key, status = requestKey{m.RequestID, m.SandboxID}, StartRejected\n\n\tdefault", "key, status = requestKey{m.RequestID, m.SandboxID}, StartAcknowledged\n\n\tdefault"}}},
	{"C18 receive: Conflict treated as Ack", "controller.go", [][2]string{{"key, status = requestKey{m.RequestID, m.SandboxID}, StartRejected", "key, status = requestKey{m.RequestID, m.SandboxID}, StartAcknowledged"}}},
	{"C19 receive: late replies overwrite TimedOut", "controller.go", [][2]string{{ctrlFilter, "\tif !ok {\n\t\treturn\n\t}\n"}}},
	{"C20 receive: unknown replies dereference nil", "controller.go", [][2]string{{ctrlFilter, "\tif (!ok && false) || r.status != StartPending {\n\t\treturn\n\t}\n"}}},
	{"C21 receive: replies only overwrite TimedOut", "controller.go", [][2]string{{ctrlFilter, "\tif !ok || r.status != StartPending && r.status != StartTimedOut {\n\t\treturn\n\t}\n"}}},
	{"C22 receive: status never set", "controller.go", [][2]string{{"\tr.status = status\n", "\t_ = status\n"}}},
	{"C23 receive: Conflict ignored (no case)", "controller.go", [][2]string{{"\tcase StartConflict:\n\t\tkey, status = requestKey{m.RequestID, m.SandboxID}, StartRejected\n", "\tcase StartConflict:\n\t\treturn\n"}}},
	{"C24 Result returns (Pending, 0) for unknown request", "controller.go", [][2]string{{ctrlResultOK, "\tif !ok {\n\t\treturn StartPending, 0\n\t}\n"}}},
	{"C25 Result ignores sandbox ID", "controller.go", [][2]string{{"\tr, ok := c.requests[requestKey{requestID, sandboxID}]\n",
		"\tvar r *startRequest\n\tvar ok bool\n\tfor k, v := range c.requests {\n\t\tif k.requestID == requestID && (!ok || k.sandboxID > r.key.sandboxID) {\n\t\t\tr, ok = v, true\n\t\t}\n\t}\n\t_ = sandboxID\n"}}},
	{"C26 constructor allows non-positive intervals", "controller.go", [][2]string{{"\tif retryEvery <= 0 || timeout <= 0 {\n", "\tif false {\n"}}},
	{"C27 Start allows empty IDs", "controller.go", [][2]string{{"\tif requestID == \"\" || sandboxID == \"\" {\n", "\tif false {\n"}}},
}

type cell struct {
	killed bool
	detail string
}

func main() {
	repo := flag.String("repo", ".", "path to the repo root (never modified)")
	run := flag.String("run", "", "go test -run filter for the single-pass mode (default: whole package)")
	only := flag.String("only", "", "regexp selecting mutants by name")
	matrix := flag.Bool("matrix", false, "run each mutant under each test separately and report unique kills")
	tests := flag.String("tests", "", "matrix mode: regexp selecting tests (default: all tests in "+pkg+")")
	timeout := flag.Duration("timeout", 60*time.Second, "per-run go test timeout")
	probe := flag.Bool("probe", false, "table mode: also run agent_work/validation/probe against each mutant (a BUG line kills)")
	flag.Parse()

	dir, cleanup, err := vlib.CopyRepo(*repo)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	defer cleanup()

	// The probe is not part of the copy (agent_work is skipped), so copy
	// just its source in when asked for.
	const probePkg = "./agent_work/validation/probe"
	if *probe {
		src := filepath.Join(*repo, probePkg, "main.go")
		dst := filepath.Join(dir, probePkg, "main.go")
		b, err := os.ReadFile(src)
		if err == nil {
			err = os.MkdirAll(filepath.Dir(dst), 0o755)
		}
		if err == nil {
			err = os.WriteFile(dst, b, 0o644)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "copy probe:", err)
			os.Exit(2)
		}
	}

	var sel *regexp.Regexp
	if *only != "" {
		sel = regexp.MustCompile(*only)
	}
	var selected []mutant
	for _, m := range mutants {
		if sel == nil || sel.MatchString(m.Name) {
			selected = append(selected, m)
		}
	}

	base := []string{"-count=1", "-timeout", timeout.String()}
	fmt.Printf("repo copy: %s\n", dir)

	var runs []string // -run values; "" means the whole package
	if *matrix {
		names, err := vlib.ListTests(dir, pkg)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		var tsel *regexp.Regexp
		if *tests != "" {
			tsel = regexp.MustCompile(*tests)
		}
		for _, n := range names {
			if tsel == nil || tsel.MatchString(n) {
				runs = append(runs, "^"+n+"$")
			}
		}
	} else {
		runs = []string{*run}
	}

	argsFor := func(run string) []string {
		args := append([]string{}, base...)
		if run != "" {
			args = append(args, "-run", run)
		}
		return append(args, pkg)
	}

	for _, r := range runs {
		args := argsFor(r)
		if out, ok := vlib.GoTest(dir, *timeout+10*time.Second, args...); !ok {
			fmt.Printf("BASELINE FAILED for go test %s; not mutating:\n%s", strings.Join(args, " "), out)
			os.Exit(1)
		}
	}
	if *probe {
		if out, ok := vlib.GoRun(dir, *timeout, probePkg); !ok {
			fmt.Printf("BASELINE FAILED for go run %s; not mutating:\n%s", probePkg, out)
			os.Exit(1)
		}
	}
	fmt.Println("baseline: ok")

	// results[mutant][run]; probes[mutant] is the probe column in table mode.
	results := make([][]cell, len(selected))
	probes := make([]string, len(selected))
	errored := 0
	for i, m := range selected {
		path := filepath.Join(dir, "actor", m.File)
		orig, err := vlib.Replace(path, m.Pairs)
		if err != nil { // Replace writes nothing unless every pair matches.
			results[i] = nil
			fmt.Printf("ERROR %s: %v\n", m.Name, err)
			errored++
			continue
		}
		results[i] = make([]cell, len(runs))
		for j, r := range runs {
			out, ok := vlib.GoTest(dir, *timeout+10*time.Second, argsFor(r)...)
			results[i][j] = cell{killed: !ok, detail: strings.Join(vlib.Summarize(out, 3), " / ")}
		}
		if *probe {
			out, ok := vlib.GoRun(dir, *timeout, probePkg)
			switch {
			case ok:
				probes[i] = "survived"
			case strings.Contains(out, "BUG "):
				probes[i] = "KILLED: " + strings.Join(bugLines(out, 2), " / ")
			default:
				probes[i] = "KILLED: " + strings.Join(vlib.Summarize(out, 2), " / ")
			}
		}
		if err := os.WriteFile(path, orig, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "restore failed:", err)
			os.Exit(2)
		}
	}

	survived := 0
	if *matrix {
		survived = printMatrix(selected, runs, results)
	} else {
		survived = printTable(selected, results, probes, *probe)
	}

	switch {
	case errored > 0:
		fmt.Println("ERROR: some mutants no longer match the code; update agent_work/validation/mutate/main.go")
		os.Exit(2)
	case survived > 0:
		os.Exit(1)
	}
}

// bugLines returns the scenario headers and BUG lines from probe output.
func bugLines(out string, max int) []string {
	var lines []string
	header := ""
	for _, l := range strings.Split(out, "\n") {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "== ") {
			header = strings.SplitN(strings.TrimPrefix(t, "== "), ".", 2)[0]
			continue
		}
		if strings.HasPrefix(t, "BUG ") {
			lines = append(lines, "scenario "+header+": "+strings.TrimPrefix(t, "BUG "))
			if len(lines) == max {
				break
			}
		}
	}
	return lines
}

func printTable(selected []mutant, results [][]cell, probes []string, withProbe bool) (survived int) {
	killed := 0
	if withProbe {
		fmt.Println("\n| mutant | tests | detail | probe |\n|---|---|---|---|")
	} else {
		fmt.Println("\n| mutant | result | detail |\n|---|---|---|")
	}
	esc := func(s string) string { return strings.ReplaceAll(s, "|", "\\|") }
	for i, m := range selected {
		if results[i] == nil {
			fmt.Printf("| %s | ERROR | pattern not found exactly once |\n", m.Name)
			continue
		}
		c := results[i][0]
		res := "SURVIVED"
		if c.killed {
			res = "KILLED"
			killed++
		} else {
			survived++
		}
		if withProbe {
			fmt.Printf("| %s | %s | %s | %s |\n", m.Name, res, esc(c.detail), esc(probes[i]))
		} else {
			fmt.Printf("| %s | %s | %s |\n", m.Name, res, esc(c.detail))
		}
	}
	fmt.Printf("\nkilled %d, survived %d\n", killed, survived)
	return survived
}

func printMatrix(selected []mutant, runs []string, results [][]cell) (survived int) {
	short := func(run string) string { return strings.TrimSuffix(strings.TrimPrefix(run, "^"), "$") }

	// Matrix: rows are mutants, columns are tests.
	fmt.Print("\n| mutant |")
	for j := range runs {
		fmt.Printf(" T%d |", j+1)
	}
	fmt.Println(" killed by |")
	fmt.Print("|---|")
	for range runs {
		fmt.Print("---|")
	}
	fmt.Println("---|")

	killsByTest := make([][]string, len(runs))
	unique := make([][]string, len(runs))
	for i, m := range selected {
		if results[i] == nil {
			fmt.Printf("| %s | ERROR |\n", m.Name)
			continue
		}
		var killers []int
		fmt.Printf("| %s |", m.Name)
		for j, c := range results[i] {
			if c.killed {
				fmt.Print(" X |")
				killers = append(killers, j)
				killsByTest[j] = append(killsByTest[j], m.Name)
			} else {
				fmt.Print(" . |")
			}
		}
		switch len(killers) {
		case 0:
			fmt.Println(" SURVIVED |")
			survived++
		case 1:
			unique[killers[0]] = append(unique[killers[0]], m.Name)
			fmt.Printf(" %s only |\n", short(runs[killers[0]]))
		default:
			fmt.Printf(" %d tests |\n", len(killers))
		}
	}

	fmt.Println("\nTests:")
	for j, r := range runs {
		fmt.Printf("  T%d %s: kills %d, unique %d\n", j+1, short(r), len(killsByTest[j]), len(unique[j]))
		sort.Strings(unique[j])
		for _, n := range unique[j] {
			fmt.Printf("      unique: %s\n", n)
		}
	}
	fmt.Printf("\nmutants %d, survived %d\n", len(selected), survived)
	return survived
}
