// Package meta runs the repo and harness gates on a bounded worker pool.
// Successful read-only steps are cached by git HEAD plus a worktree content
// fingerprint so a second gate does not repeat a check the first gate already
// proved. A cache hit is printed as such. Failures are never cached. A step
// with no command fails closed.
package meta

import (
	"alt/scripts/altharness/internal/fsperm"
	"alt/scripts/altharness/internal/gate"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const maxCacheEntries = 512

var cacheEnvVars = []string{
	"LINT_POLICY",
	"LINT_BASE",
	"PYREFLY_BIN",
	"AST_GREP_BIN",
	"GITHUB_STEP_SUMMARY",
}

// Step is one catalog entry. Dir is the working directory. Env is extra KEY=VAL.
// Key distinguishes in-process Run steps in cache keys and unionSteps dedup.
// When Run is set it replaces Args.
type Step struct {
	Name     string
	Key      string
	Parallel bool
	Args     []string
	Dir      string
	Env      []string
	// Cache records a zero exit so a later gate can reuse it. Never set this
	// on a step that writes the repository or an external output file.
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
// that step's parallel group finishes, followed by a consolidated PASS/FAIL
// summary footer (including any CORPUS CAPTURE RATE line).
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
	corpusLine := ""
	for i, step := range steps {
		fmt.Fprintf(opt.Stdout, "==> [%s]\n%s", step.Name, outs[i])
		label := "PASS"
		if rep.Results[i].Code != 0 || rep.Results[i].Err != nil {
			label = "FAIL"
		}
		fmt.Fprintf(opt.Stdout, "%s: %s\n", label, step.Name)
		if found := extractCorpusRate(outs[i]); found != "" {
			corpusLine = found
		}
	}
	fmt.Fprintln(opt.Stdout, "\n==> summary")
	gate.FormatSummary(opt.Stdout, rep)
	if corpusLine != "" {
		fmt.Fprintln(opt.Stdout, corpusLine)
	}
	if rep.Failed > 0 {
		return 1, nil
	}
	return 0, nil
}

func extractCorpusRate(text string) string {
	found := ""
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "CORPUS CAPTURE RATE:") {
			found = trimmed
		}
	}
	return found
}

func runStep(ctx context.Context, opt Options, step Step, key string) (string, int, error) {
	if step.Run == nil && len(step.Args) == 0 {
		return "", 2, fmt.Errorf("step %s has no command", step.Name)
	}
	useCache := step.Cache && !opt.NoCache && os.Getenv("ALT_HARNESS_NO_CACHE") == ""
	if useCache {
		if text, ok := readCache(key); ok {
			return "CACHE HIT: " + step.Name + "\n" + text, 0, nil
		}
	}
	if step.Run != nil {
		text, code, err := step.Run(ctx)
		if err == nil && code == 0 && useCache {
			writeCache(key, text)
		}
		if code == 0 && err == nil {
			return text + "EXIT 0\n", 0, nil
		}
		return text, code, err
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
		if useCache {
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
	status, err := exec.CommandContext(ctx, "git", "-C", repo, "status", "--porcelain=v1", "-z", "-uall").Output()
	if err != nil {
		return "", err
	}
	h := sha256.New()
	_, _ = h.Write(bytesTrim(head))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write(status)
	_, _ = h.Write([]byte{0})

	// Hash the content/symlink targets of every dirty, staged, or untracked path
	// so editing an already-modified file always invalidates the cache.
	entries := bytes.Split(status, []byte{0})
	for i := 0; i < len(entries); i++ {
		entry := entries[i]
		if len(entry) < 4 {
			continue
		}
		x, y := entry[0], entry[1]
		rel := string(entry[3:])
		if x == 'R' || x == 'C' || y == 'R' || y == 'C' {
			// Skip the original path entry following a rename/copy.
			i++
		}
		hashWorktreePath(h, repo, rel)
	}

	// Hash the running executable so rebuilding alt-harness invalidates step caches.
	hashSelfExecutable(h)

	return hex.EncodeToString(h.Sum(nil)), nil
}

func hashWorktreePath(h io.Writer, repo, rel string) {
	_, _ = io.WriteString(h, "path:")
	_, _ = io.WriteString(h, rel)
	_, _ = h.Write([]byte{0})
	full := filepath.Join(repo, filepath.FromSlash(rel))
	fi, err := os.Lstat(full)
	if err != nil {
		_, _ = io.WriteString(h, "missing")
		_, _ = h.Write([]byte{0})
		return
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		target, lerr := os.Readlink(full)
		if lerr != nil {
			_, _ = io.WriteString(h, "symlink-err")
		} else {
			_, _ = io.WriteString(h, "symlink:")
			_, _ = io.WriteString(h, target)
		}
		_, _ = h.Write([]byte{0})
		return
	}
	if fi.Mode().IsRegular() {
		fmt.Fprintf(h, "file:%o:%d:", fi.Mode().Perm(), fi.Size())
		if digest, derr := sha256File(full); derr == nil {
			_, _ = io.WriteString(h, digest)
		} else {
			_, _ = io.WriteString(h, "read-err")
		}
		_, _ = h.Write([]byte{0})
		return
	}
	fmt.Fprintf(h, "other:%o", fi.Mode())
	_, _ = h.Write([]byte{0})
}

func hashSelfExecutable(h io.Writer) {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	if digest, err := sha256File(exe); err == nil {
		_, _ = io.WriteString(h, "exe:")
		_, _ = io.WriteString(h, digest)
		_, _ = h.Write([]byte{0})
	}
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	fh := sha256.New()
	if _, err := io.Copy(fh, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(fh.Sum(nil)), nil
}

func bytesTrim(b []byte) []byte { return []byte(strings.TrimSpace(string(b))) }

func cacheKey(fp string, step Step) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\n%s\n%s\n%s\n%s\n", fp, step.Name, step.Key, step.Dir, strings.Join(step.Args, "\x00"))
	fmt.Fprintf(h, "%s\n", strings.Join(step.Env, "\x00"))
	for _, k := range cacheEnvVars {
		v, ok := os.LookupEnv(k)
		if ok {
			fmt.Fprintf(h, "env:%s=%s\x00", k, v)
		} else {
			fmt.Fprintf(h, "env:%s=<unset>\x00", k)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

func cacheDir() string {
	if d := os.Getenv("ALT_HARNESS_CACHE_DIR"); d != "" {
		return d
	}
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return filepath.Join(os.TempDir(), "alt-harness-steps")
		}
		base = filepath.Join(home, ".cache")
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
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return
	}
	tmpName := tmp.Name()
	if _, err := tmp.WriteString(text); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return
	}
	if err := tmp.Chmod(fsperm.File); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return
	}
	if err := os.Rename(tmpName, filepath.Join(dir, key)); err != nil {
		_ = os.Remove(tmpName)
		return
	}
	pruneCache(dir, maxCacheEntries)
}

func pruneCache(dir string, limit int) {
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) <= limit {
		return
	}
	type item struct {
		name  string
		mtime time.Time
	}
	var files []item
	for _, ent := range entries {
		if ent.IsDir() || strings.HasPrefix(ent.Name(), ".tmp-") {
			continue
		}
		info, err := ent.Info()
		if err != nil {
			continue
		}
		files = append(files, item{name: ent.Name(), mtime: info.ModTime()})
	}
	if len(files) <= limit {
		return
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].mtime.Before(files[j].mtime)
	})
	excess := len(files) - limit
	for i := 0; i < excess; i++ {
		_ = os.Remove(filepath.Join(dir, files[i].name))
	}
}
