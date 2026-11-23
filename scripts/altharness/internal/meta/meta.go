// Package meta runs the repo and harness gates on a bounded worker pool.
// Successful read-only steps are cached by git HEAD plus a worktree fingerprint
// so a second gate does not repeat a check the first gate already proved.
// A cache hit is printed as such. Failures are never cached. A step with no
// command fails closed.
package meta

import (
	"alt/scripts/altharness/internal/fsperm"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"alt/scripts/altharness/internal/gate"
)

// Step is one catalog entry. Dir is the working directory. Env is extra KEY=VAL.
// When Run is set it replaces Args.
type Step struct {
	Name     string
	Parallel bool
	Args     []string
	Dir      string
	Env      []string
	// Cache records a zero exit so a later gate can reuse it. Never set this
	// on a step that writes the repository.
	Cache bool
	Run   func(ctx context.Context) (output string, code int, err error)
}

// Options configures a gate run.
type Options struct {
	Workers int
	Timeout time.Duration
	Repo    string
	Stdout  io.Writer
	Stderr  io.Writer
	NoCache bool
}

// Run executes steps. Output of each step is emitted in catalog order after
// that step's parallel group finishes, followed by a PASS/FAIL summary.
func Run(ctx context.Context, steps []Step, opt Options) (int, error) {
	if ctx == nil {
		return 2, fmt.Errorf("meta: nil context")
	}
	if opt.Repo == "" {
		opt.Repo = "."
	}
	if opt.Stdout == nil {
		opt.Stdout = os.Stdout
	}
	if opt.Stderr == nil {
		opt.Stderr = os.Stderr
	}
	workers := opt.Workers
	if workers == 0 {
		if n, err := strconv.Atoi(os.Getenv("ALT_HARNESS_WORKERS")); err == nil && n > 0 {
			workers = n
		}
	}
	timeout := opt.Timeout
	if timeout == 0 {
		if n, err := strconv.Atoi(os.Getenv("ALT_HARNESS_STEP_TIMEOUT")); err == nil && n > 0 {
			timeout = time.Duration(n) * time.Second
		}
	}
	fp, err := fingerprint(ctx, opt.Repo)
	if err != nil {
		fmt.Fprintf(opt.Stderr, "ERROR: worktree fingerprint: %v\n", err)
		return 1, err
	}
	gsteps := make([]gate.Step, len(steps))
	outs := make([]string, len(steps))
	var mu sync.Mutex
	for i, step := range steps {
		i, step := i, step
		key := cacheKey(fp, step)
		gsteps[i] = gate.Step{
			Name:     step.Name,
			Parallel: step.Parallel,
			Timeout:  timeout,
			Run: func(ctx context.Context) (int, error) {
				text, code, err := runStep(ctx, opt, step, key)
				mu.Lock()
				outs[i] = text
				mu.Unlock()
				return code, err
			},
		}
	}
	rep, err := gate.Run(ctx, gsteps, gate.Options{Workers: workers, DefaultTimeout: timeout, Stdout: opt.Stdout, Stderr: opt.Stderr})
	if err != nil {
		return 2, err
	}
	for i, step := range steps {
		fmt.Fprintf(opt.Stdout, "==> [%s]\n%s", step.Name, outs[i])
		label := "PASS"
		if rep.Results[i].Code != 0 || rep.Results[i].Err != nil {
			label = "FAIL"
		}
		fmt.Fprintf(opt.Stdout, "%s: %s\n", label, step.Name)
	}
	if rep.Failed > 0 {
		return 1, nil
	}
	return 0, nil
}

func runStep(ctx context.Context, opt Options, step Step, key string) (string, int, error) {
	if step.Run == nil && len(step.Args) == 0 {
		return "", 2, fmt.Errorf("step %s has no command", step.Name)
	}
	if step.Run != nil {
		text, code, err := step.Run(ctx)
		if err == nil && code == 0 && step.Cache && !opt.NoCache && os.Getenv("ALT_HARNESS_NO_CACHE") == "" {
			writeCache(key, text)
		}
		if code == 0 && err == nil {
			return text + "EXIT 0\n", 0, nil
		}
		return text, code, err
	}
	if step.Cache && !opt.NoCache && os.Getenv("ALT_HARNESS_NO_CACHE") == "" {
		if text, ok := readCache(key); ok {
			return "CACHE HIT: " + step.Name + "\n" + text, 0, nil
		}
	}
	cmd := exec.CommandContext(ctx, step.Args[0], step.Args[1:]...)
	cmd.Dir = step.Dir
	if cmd.Dir == "" {
		cmd.Dir = opt.Repo
	}
	if len(step.Env) > 0 {
		cmd.Env = append(os.Environ(), step.Env...)
	}
	out, err := cmd.CombinedOutput()
	text := string(out)
	if err == nil {
		if step.Cache && !opt.NoCache && os.Getenv("ALT_HARNESS_NO_CACHE") == "" {
			writeCache(key, text)
		}
		return text + "EXIT 0\n", 0, nil
	}
	if ctx.Err() != nil {
		return text, 1, ctx.Err()
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return text + fmt.Sprintf("EXIT %d\n", ee.ExitCode()), ee.ExitCode(), nil
	}
	return text, 1, err
}

func fingerprint(ctx context.Context, repo string) (string, error) {
	head, err := exec.CommandContext(ctx, "git", "-C", repo, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", err
	}
	status, err := exec.CommandContext(ctx, "git", "-C", repo, "status", "--porcelain=v1", "-z").Output()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(append(append(bytesTrim(head), 0), status...))
	return hex.EncodeToString(sum[:]), nil
}

func bytesTrim(b []byte) []byte { return []byte(strings.TrimSpace(string(b))) }

func cacheKey(fp string, step Step) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\n%s\n%s\n", fp, step.Name, strings.Join(step.Args, "\x00"))
	fmt.Fprintf(h, "%s", strings.Join(step.Env, "\x00"))
	return hex.EncodeToString(h.Sum(nil))
}

func cacheDir() string {
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return filepath.Join(os.TempDir(), "alt-harness-steps")
		}
		base = filepath.Join(home, ".cache")
	}
	if d := os.Getenv("ALT_HARNESS_CACHE_DIR"); d != "" {
		return d
	}
	return filepath.Join(base, "alt-harness", "steps")
}

func readCache(key string) (string, bool) {
	b, err := os.ReadFile(filepath.Join(cacheDir(), key))
	if err != nil {
		return "", false
	}
	return string(b), true
}

func writeCache(key, text string) {
	dir := cacheDir()
	if err := os.MkdirAll(dir, fsperm.Dir); err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, key), []byte(text), fsperm.File)
}
