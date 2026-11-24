package meta

import (
	"alt/scripts/altharness/internal/fsperm"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func initTestRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := newGit(repo, args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v %s", args, err, out)
		}
	}
	git("init", "-q")
	git("config", "user.email", "t@example.com")
	git("config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(repo, "a"), []byte("a"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	git("add", "a")
	git("commit", "-q", "-m", "init")
	return repo
}

func TestParallelFasterAndOrdered(t *testing.T) {
	repo := initTestRepo(t)
	s1Started := make(chan struct{})
	s2Started := make(chan struct{})
	steps := []Step{
		{Name: "s1", Parallel: true, Run: func(ctx context.Context) (string, int, error) {
			close(s1Started)
			select {
			case <-s2Started:
				return "first\n", 0, nil
			case <-time.After(3 * time.Second):
				return "", 1, fmt.Errorf("s1 timed out waiting for concurrent s2")
			}
		}},
		{Name: "s2", Parallel: true, Run: func(ctx context.Context) (string, int, error) {
			close(s2Started)
			select {
			case <-s1Started:
				return "second\n", 0, nil
			case <-time.After(3 * time.Second):
				return "", 1, fmt.Errorf("s2 timed out waiting for concurrent s1")
			}
		}},
		{Name: "s3", Parallel: true, Args: []string{"echo", "third"}},
	}
	var out bytes.Buffer
	code, err := Run(context.Background(), steps, Options{Repo: repo, Workers: 2, Stdout: &out, Stderr: &out, NoCache: true})
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v out=%s", code, err, out.String())
	}
	text := out.String()
	i1 := strings.Index(text, "==> [s1]")
	i2 := strings.Index(text, "==> [s2]")
	i3 := strings.Index(text, "==> [s3]")
	if !(i1 >= 0 && i1 < i2 && i2 < i3) {
		t.Fatalf("order:\n%s", text)
	}
	if !strings.Contains(text, "third") {
		t.Fatalf("missing echo: %s", text)
	}
	if !strings.Contains(text, "==> summary") {
		t.Fatalf("missing summary footer: %s", text)
	}
}

func TestEmptyCommandFailsClosed(t *testing.T) {
	repo := initTestRepo(t)
	var out bytes.Buffer
	code, err := Run(context.Background(), []Step{{Name: "empty"}}, Options{Repo: repo, Stdout: &out, Stderr: &out})
	if err != nil {
		t.Fatal(err)
	}
	if code == 0 || !strings.Contains(out.String(), "FAIL: empty") {
		t.Fatalf("code=%d out=%s", code, out.String())
	}
}

func TestCacheHitAndDirtyContentInvalidation(t *testing.T) {
	repo := initTestRepo(t)
	cache := t.TempDir()
	t.Setenv("ALT_HARNESS_NO_CACHE", "")
	t.Setenv("ALT_HARNESS_CACHE_DIR", cache)

	// Make tracked file dirty with content "dirty1".
	tracked := filepath.Join(repo, "a")
	if err := os.WriteFile(tracked, []byte("dirty1"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	steps := []Step{{Name: "echo", Parallel: true, Cache: true, Args: []string{"echo", "once"}}}

	var out bytes.Buffer
	if code, err := Run(context.Background(), steps, Options{Repo: repo, Stdout: &out, Stderr: &out}); err != nil || code != 0 {
		t.Fatal(code, err, out.String())
	}
	out.Reset()
	if code, err := Run(context.Background(), steps, Options{Repo: repo, Stdout: &out, Stderr: &out}); err != nil || code != 0 {
		t.Fatal(code, err, out.String())
	}
	if !strings.Contains(out.String(), "CACHE HIT: echo") || strings.Count(out.String(), "once") != 1 {
		t.Fatalf("expected cached body once, got %s", out.String())
	}

	// Edit the already-dirty file to 0 bytes -> MUST miss cache!
	if err := os.WriteFile(tracked, nil, fsperm.File); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if code, err := Run(context.Background(), steps, Options{Repo: repo, Stdout: &out, Stderr: &out}); err != nil || code != 0 {
		t.Fatal(code, err, out.String())
	}
	if strings.Contains(out.String(), "CACHE HIT: echo") {
		t.Fatalf("expected cache miss after modifying dirty tracked file, got %s", out.String())
	}

	// Untracked file edit -> MUST miss cache!
	untracked := filepath.Join(repo, "u.txt")
	if err := os.WriteFile(untracked, []byte("v1"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if code, err := Run(context.Background(), steps, Options{Repo: repo, Stdout: &out, Stderr: &out}); err != nil || code != 0 {
		t.Fatal(code, err, out.String())
	}
	if err := os.WriteFile(untracked, []byte("v2"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if code, err := Run(context.Background(), steps, Options{Repo: repo, Stdout: &out, Stderr: &out}); err != nil || code != 0 {
		t.Fatal(code, err, out.String())
	}
	if strings.Contains(out.String(), "CACHE HIT: echo") {
		t.Fatalf("expected cache miss after editing untracked file, got %s", out.String())
	}
}

func TestCacheInvalidatesOnEnvAndHitsForCustomRun(t *testing.T) {
	repo := initTestRepo(t)
	cache := t.TempDir()
	t.Setenv("ALT_HARNESS_NO_CACHE", "")
	t.Setenv("ALT_HARNESS_CACHE_DIR", cache)
	t.Setenv("LINT_POLICY", "block-new")

	calls := 0
	step := Step{
		Name: "custom", Parallel: true, Cache: true,
		Run: func(ctx context.Context) (string, int, error) {
			calls++
			return fmt.Sprintf("run-%d\n", calls), 0, nil
		},
	}
	var out bytes.Buffer
	if code, err := Run(context.Background(), []Step{step}, Options{Repo: repo, Stdout: &out, Stderr: &out}); err != nil || code != 0 {
		t.Fatal(code, err, out.String())
	}
	if calls != 1 {
		t.Fatalf("calls=%d, want 1", calls)
	}
	// Second run with same env must hit cache for step.Run!
	out.Reset()
	if code, err := Run(context.Background(), []Step{step}, Options{Repo: repo, Stdout: &out, Stderr: &out}); err != nil || code != 0 {
		t.Fatal(code, err, out.String())
	}
	if calls != 1 || !strings.Contains(out.String(), "CACHE HIT: custom") {
		t.Fatalf("expected cache hit for step.Run, calls=%d out=%s", calls, out.String())
	}
	// Changing LINT_POLICY must invalidate cache!
	t.Setenv("LINT_POLICY", "fix-existing")
	out.Reset()
	if code, err := Run(context.Background(), []Step{step}, Options{Repo: repo, Stdout: &out, Stderr: &out}); err != nil || code != 0 {
		t.Fatal(code, err, out.String())
	}
	if calls != 2 || strings.Contains(out.String(), "CACHE HIT: custom") {
		t.Fatalf("expected cache miss after LINT_POLICY change, calls=%d out=%s", calls, out.String())
	}
}

func TestCacheDirHonorsOverrideWithoutHome(t *testing.T) {
	custom := filepath.Join(t.TempDir(), "custom-cache")
	t.Setenv("ALT_HARNESS_CACHE_DIR", custom)
	t.Setenv("HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")
	if got := cacheDir(); got != custom {
		t.Fatalf("cacheDir()=%q, want %q", got, custom)
	}
}

func TestPruneCacheBoundsFileCount(t *testing.T) {
	dir := t.TempDir()
	base := time.Now().Add(-10 * time.Minute)
	for i := 0; i < 10; i++ {
		p := filepath.Join(dir, fmt.Sprintf("k-%02d", i))
		if err := os.WriteFile(p, []byte("ok"), fsperm.File); err != nil {
			t.Fatal(err)
		}
		mt := base.Add(time.Duration(i) * time.Second)
		if err := os.Chtimes(p, mt, mt); err != nil {
			t.Fatal(err)
		}
	}
	pruneCache(dir, 4)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 4 {
		t.Fatalf("expected 4 entries after prune, got %d", len(entries))
	}
	if _, err := os.Stat(filepath.Join(dir, "k-09")); err != nil {
		t.Fatalf("expected newest entry k-09 to remain: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "k-00")); !os.IsNotExist(err) {
		t.Fatalf("expected oldest entry k-00 to be pruned")
	}
}

func TestSummarySurfacesCorpusCaptureRate(t *testing.T) {
	repo := initTestRepo(t)
	steps := []Step{
		{Name: "corpus", Parallel: true, Run: func(ctx context.Context) (string, int, error) {
			return "ok\nCORPUS CAPTURE RATE: 22/22 (100.0%) [caught=22 | expected-miss=0 | failures=0]\n", 0, nil
		}},
		{Name: "later", Parallel: true, Args: []string{"echo", "after"}},
	}
	var out bytes.Buffer
	code, err := Run(context.Background(), steps, Options{Repo: repo, Stdout: &out, Stderr: &out, NoCache: true})
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v out=%s", code, err, out.String())
	}
	text := out.String()
	summaryIdx := strings.Index(text, "==> summary")
	if summaryIdx < 0 {
		t.Fatalf("missing summary footer: %s", text)
	}
	footer := text[summaryIdx:]
	if !strings.Contains(footer, "CORPUS CAPTURE RATE: 22/22 (100.0%)") {
		t.Fatalf("expected CORPUS CAPTURE RATE in summary footer, got:\n%s", footer)
	}
}

func TestCatalogPropertiesAndUnionSteps(t *testing.T) {
	t.Setenv("GITHUB_STEP_SUMMARY", filepath.Join(t.TempDir(), "summary.md"))
	rSteps, err := repoSteps("/tmp/fake-repo", "HEAD~1")
	if err != nil {
		t.Fatal(err)
	}
	foundSummary := false
	for _, s := range rSteps {
		if s.Name == "suppression-diff-summary" {
			foundSummary = true
			if s.Cache {
				t.Fatal("suppression-diff-summary must have Cache=false")
			}
		}
	}
	if !foundSummary {
		t.Fatal("missing suppression-diff-summary in repoSteps")
	}

	hSteps := harnessSteps("/tmp/nonexistent-repo", "HEAD~1")
	wantNames := map[string]bool{
		"exec_guards_and_runs_tests": false,
		"altharness_gofmt":           false,
		"altharness_govet":           false,
		"altharness_go_test_race":    false,
	}
	for _, s := range hSteps {
		if _, ok := wantNames[s.Name]; ok {
			wantNames[s.Name] = true
		}
	}
	for name, seen := range wantNames {
		if !seen {
			t.Fatalf("harnessSteps missing required step %q", name)
		}
	}

	// unionSteps must NOT collapse steps with different Dir or Env.
	a := []Step{
		{Name: "s1", Dir: "/a", Args: []string{"echo", "hi"}},
		{Name: "s2", Dir: "/b", Args: []string{"echo", "hi"}},
		{Name: "s3", Dir: "/a", Env: []string{"X=1"}, Args: []string{"echo", "hi"}},
		{Name: "s4-dup", Dir: "/a", Args: []string{"echo", "hi"}},
	}
	got := unionSteps(a)
	if len(got) != 3 {
		t.Fatalf("unionSteps len=%d, want 3 (%#v)", len(got), got)
	}
}

func TestResolveBaseScenarios(t *testing.T) {
	repo := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := newGit(repo, args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "-b", "main", "-q")
	git("config", "user.email", "t@example.com")
	git("config", "user.name", "t")
	writeCommit := func(name, body, msg string) string {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(body), fsperm.File); err != nil {
			t.Fatal(err)
		}
		git("add", name)
		git("commit", "-q", "-m", msg)
		return git("rev-parse", "HEAD")
	}
	c1 := writeCommit("a.txt", "one\n", "c1")
	c2 := writeCommit("a.txt", "two\n", "c2")
	c3 := writeCommit("a.txt", "three\n", "c3")
	git("update-ref", "refs/remotes/origin/main", c3)

	// Unset LINT_BASE with HEAD==origin/main -> HEAD~1 (c2).
	t.Setenv("LINT_BASE", "")
	if got, err := ResolveBase(context.Background(), repo); err != nil || got != c2 {
		t.Fatalf("unset: got=%q err=%v want=%s", got, err, c2)
	}
	// All-zero LINT_BASE -> treated as unset -> c2.
	t.Setenv("LINT_BASE", "0000000000000000000000000000000000000000")
	if got, err := ResolveBase(context.Background(), repo); err != nil || got != c2 {
		t.Fatalf("all-zero: got=%q err=%v want=%s", got, err, c2)
	}
	// Unreachable 40-hex force-push SHA -> falls back to c2.
	t.Setenv("LINT_BASE", "1111111111111111111111111111111111111111")
	if got, err := ResolveBase(context.Background(), repo); err != nil || got != c2 {
		t.Fatalf("force-push: got=%q err=%v want=%s", got, err, c2)
	}
	// LINT_BASE == HEAD -> falls back to HEAD~1 (c2).
	t.Setenv("LINT_BASE", c3)
	if got, err := ResolveBase(context.Background(), repo); err != nil || got != c2 {
		t.Fatalf("equal HEAD: got=%q err=%v want=%s", got, err, c2)
	}
	// Explicit valid ancestor -> c1.
	t.Setenv("LINT_BASE", c1)
	if got, err := ResolveBase(context.Background(), repo); err != nil || got != c1 {
		t.Fatalf("valid override: got=%q err=%v want=%s", got, err, c1)
	}
	// Unresolvable non-SHA ref -> error.
	t.Setenv("LINT_BASE", "does-not-exist-rev")
	if _, err := ResolveBase(context.Background(), repo); err == nil || !strings.Contains(err.Error(), "cannot be resolved") {
		t.Fatalf("unresolvable: err=%v, want cannot be resolved", err)
	}
	// Diverged PR branch: fork at c2 while origin/release is at c3 -> merge-base c2.
	git("checkout", "-q", "-b", "pr-branch", c2)
	writeCommit("pr.txt", "pr\n", "p1")
	git("update-ref", "refs/remotes/origin/release", c3)
	t.Setenv("LINT_BASE", "origin/release")
	if got, err := ResolveBase(context.Background(), repo); err != nil || got != c2 {
		t.Fatalf("diverged: got=%q err=%v want=%s", got, err, c2)
	}
	// Unrelated orphan commit -> error.
	git("checkout", "-q", "--orphan", "orphan-branch")
	git("rm", "-rf", "-q", ".")
	orphan := writeCommit("orphan.txt", "o\n", "orphan")
	git("checkout", "-q", "pr-branch")
	t.Setenv("LINT_BASE", orphan)
	if _, err := ResolveBase(context.Background(), repo); err == nil || !strings.Contains(err.Error(), "cannot compute merge-base") {
		t.Fatalf("orphan: err=%v, want cannot compute merge-base", err)
	}
}
