// Package vlib holds the shared plumbing for the validation scripts: copying
// the repo to a temp directory, applying string replacements to the copy,
// running go test there, and listing tests. Nothing here touches the source
// repo.
package vlib

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// skip lists top-level entries that are not copied: VCS metadata, built
// binaries, and the agent_work folder itself.
var skip = map[string]bool{".git": true, "bin": true, "agent_work": true}

// CopyRepo copies src into a fresh temp directory and returns its path and a
// cleanup func. Symlinks are not followed; the repo has none.
func CopyRepo(src string) (dst string, cleanup func(), err error) {
	src, err = filepath.Abs(src)
	if err != nil {
		return "", nil, err
	}
	if _, err := os.Stat(filepath.Join(src, "go.mod")); err != nil {
		return "", nil, fmt.Errorf("%s does not look like the repo root: %w", src, err)
	}
	dst, err = os.MkdirTemp("", "durable-actor-validate-")
	if err != nil {
		return "", nil, err
	}
	cleanup = func() { os.RemoveAll(dst) }
	err = filepath.WalkDir(src, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, _ := filepath.Rel(src, p)
		if rel == "." {
			return nil
		}
		if d.IsDir() && skip[rel] {
			return filepath.SkipDir
		}
		out := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(out, b, info.Mode().Perm())
	})
	if err != nil {
		cleanup()
		return "", nil, err
	}
	return dst, cleanup, nil
}

// Replace applies the {old, new} pairs in order to the file at path and
// returns the original contents so the caller can restore them. Each old must
// occur exactly once; otherwise Replace returns an error and writes nothing.
func Replace(path string, pairs [][2]string) (orig []byte, err error) {
	orig, err = os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	s := string(orig)
	for _, p := range pairs {
		if n := strings.Count(s, p[0]); n != 1 {
			return orig, fmt.Errorf("%s: pattern %q occurs %d times, want 1", filepath.Base(path), p[0], n)
		}
		s = strings.Replace(s, p[0], p[1], 1)
	}
	return orig, os.WriteFile(path, []byte(s), 0o644)
}

// GoTest runs `go test` in dir with args and returns combined output and
// whether it passed. A run longer than timeout is killed and counts as failed.
func GoTest(dir string, timeout time.Duration, args ...string) (out string, ok bool) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", append([]string{"test"}, args...)...)
	cmd.Dir = dir
	b, err := cmd.CombinedOutput()
	out = string(b)
	if ctx.Err() != nil {
		out += "\n[validation: killed after " + timeout.String() + "]"
	}
	return out, err == nil
}

// GoRun runs `go run pkg` in dir and returns combined output and whether it
// exited 0. A run longer than timeout is killed and counts as failed.
func GoRun(dir string, timeout time.Duration, pkg string) (out string, ok bool) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "run", pkg)
	cmd.Dir = dir
	b, err := cmd.CombinedOutput()
	out = string(b)
	if ctx.Err() != nil {
		out += "\n[validation: killed after " + timeout.String() + "]"
	}
	return out, err == nil
}

// ListTests returns the Test functions in pkg (a ./path pattern) using
// `go test -list`, in source order.
func ListTests(dir, pkg string) ([]string, error) {
	cmd := exec.Command("go", "test", "-list", "^Test", pkg)
	cmd.Dir = dir
	b, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("go test -list: %v\n%s", err, b)
	}
	var tests []string
	for _, l := range strings.Split(string(b), "\n") {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "Test") {
			tests = append(tests, l)
		}
	}
	return tests, nil
}

// Summarize pulls the interesting lines out of go test output: failing test
// names, assertion messages, panics, and timeouts.
func Summarize(out string, max int) []string {
	var lines []string
	seen := map[string]bool{}
	for _, l := range strings.Split(out, "\n") {
		t := strings.TrimSpace(l)
		switch {
		case strings.HasPrefix(t, "--- FAIL"),
			strings.HasPrefix(t, "panic:"),
			strings.Contains(t, "_test.go:"),
			strings.Contains(t, "[validation:"),
			strings.Contains(t, "build failed"):
		default:
			continue
		}
		if len(t) > 160 {
			t = t[:157] + "..."
		}
		if !seen[t] {
			seen[t] = true
			lines = append(lines, t)
		}
		if len(lines) == max {
			break
		}
	}
	return lines
}
