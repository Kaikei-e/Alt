package meta

import (
	"alt/scripts/altharness/internal/fsperm"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParallelFasterAndOrdered(t *testing.T) {
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

	steps := []Step{
		{Name: "s1", Parallel: true, Args: []string{"sleep", "0.3"}},
		{Name: "s2", Parallel: true, Args: []string{"sleep", "0.3"}},
		{Name: "s3", Parallel: true, Args: []string{"echo", "third"}},
	}
	var out bytes.Buffer
	start := time.Now()
	code, err := Run(context.Background(), steps, Options{Repo: repo, Workers: 2, Stdout: &out, Stderr: &out, NoCache: true})
	elapsed := time.Since(start)
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v out=%s", code, err, out.String())
	}
	if elapsed > 550*time.Millisecond {
		t.Fatalf("parallel steps took %s, want well under 600ms", elapsed)
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
}

func TestEmptyCommandFailsClosed(t *testing.T) {
	repo := t.TempDir()
	cmd := newGit(repo, "init", "-q")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	cmd = newGit(repo, "config", "user.email", "t@example.com")
	_, _ = cmd.CombinedOutput()
	cmd = newGit(repo, "config", "user.name", "t")
	_, _ = cmd.CombinedOutput()
	if err := os.WriteFile(filepath.Join(repo, "a"), []byte("a"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	cmd = newGit(repo, "add", "a")
	_, _ = cmd.CombinedOutput()
	cmd = newGit(repo, "commit", "-q", "-m", "init")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	var out bytes.Buffer
	code, err := Run(context.Background(), []Step{{Name: "empty"}}, Options{Repo: repo, Stdout: &out, Stderr: &out})
	if err != nil {
		t.Fatal(err)
	}
	if code == 0 || !strings.Contains(out.String(), "FAIL: empty") {
		t.Fatalf("code=%d out=%s", code, out.String())
	}
}

func TestCacheHit(t *testing.T) {
	repo := t.TempDir()
	cmd := newGit(repo, "init", "-q")
	_, _ = cmd.CombinedOutput()
	cmd = newGit(repo, "config", "user.email", "t@example.com")
	_, _ = cmd.CombinedOutput()
	cmd = newGit(repo, "config", "user.name", "t")
	_, _ = cmd.CombinedOutput()
	if err := os.WriteFile(filepath.Join(repo, "a"), []byte("a"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	cmd = newGit(repo, "add", "a")
	_, _ = cmd.CombinedOutput()
	cmd = newGit(repo, "commit", "-q", "-m", "init")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(string(out), err)
	}
	cache := t.TempDir()
	t.Setenv("ALT_HARNESS_CACHE_DIR", cache)
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
}
