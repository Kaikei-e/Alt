package codegen

import (
	"alt/scripts/altharness/internal/fsperm"
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncMissingThenCleanAndOrphan(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	src := filepath.Join(mustRepo(t), "scripts", "codegen", "go_canonical")
	dst := filepath.Join(root, "scripts", "codegen", "go_canonical")
	if err := os.MkdirAll(filepath.Dir(dst), fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("cp", "-a", src, dst).CombinedOutput(); err != nil {
		t.Fatalf("cp: %v %s", err, out)
	}
	var out, errb bytes.Buffer
	if code := Sync(context.Background(), root, true, &out, &errb); code != 1 || !strings.Contains(errb.String(), "MISSING:") {
		t.Fatalf("check missing code=%d err=%s", code, errb.String())
	}
	out.Reset()
	errb.Reset()
	if code := Sync(context.Background(), root, false, &out, &errb); code != 0 {
		t.Fatalf("sync code=%d out=%s err=%s", code, out.String(), errb.String())
	}
	gen := filepath.Join(root, "auth-hub", "internal", "httpserver", "httpserver_gen.go")
	body, err := os.ReadFile(gen)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(body), headerPrefix) {
		t.Fatalf("header missing: %q", body[:min(80, len(body))])
	}
	out.Reset()
	errb.Reset()
	if code := Sync(context.Background(), root, true, &out, &errb); code != 0 {
		t.Fatalf("recheck code=%d err=%s", code, errb.String())
	}
	orphan := filepath.Join(root, "orphan-service", "internal", "fsperm", "fsperm_gen.go")
	if err := os.MkdirAll(filepath.Dir(orphan), fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(orphan, []byte(generatedHdr+"package fsperm\n"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errb.Reset()
	if code := Sync(context.Background(), root, true, &out, &errb); code != 1 || !strings.Contains(errb.String(), "ORPHAN:") {
		t.Fatalf("orphan code=%d err=%s", code, errb.String())
	}
}

func TestGitLsFilesIgnoresNodeModules(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = root
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	git("init", "-q")
	git("config", "user.email", "t@example.com")
	git("config", "user.name", "t")
	src := filepath.Join(mustRepo(t), "scripts", "codegen", "go_canonical")
	dst := filepath.Join(root, "scripts", "codegen", "go_canonical")
	if err := os.MkdirAll(filepath.Dir(dst), fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("cp", "-a", src, dst).CombinedOutput(); err != nil {
		t.Fatalf("cp: %v %s", err, out)
	}
	if code := Sync(context.Background(), root, false, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatal(code)
	}
	git("add", ".")
	git("commit", "-q", "-m", "base")
	noise := filepath.Join(root, "node_modules", "pkg", "internal", "fsperm", "fsperm_gen.go")
	if err := os.MkdirAll(filepath.Dir(noise), fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(noise, []byte(generatedHdr+"package fsperm\n"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	var errb bytes.Buffer
	if code := Sync(context.Background(), root, true, &bytes.Buffer{}, &errb); code != 0 {
		t.Fatalf("node_modules orphan must be ignored, code=%d err=%s", code, errb.String())
	}
}

func TestSyncDetectsTamperedFile(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	src := filepath.Join(mustRepo(t), "scripts", "codegen", "go_canonical")
	dst := filepath.Join(root, "scripts", "codegen", "go_canonical")
	if err := os.MkdirAll(filepath.Dir(dst), fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("cp", "-a", src, dst).CombinedOutput(); err != nil {
		t.Fatalf("cp: %v %s", err, out)
	}
	if code := Sync(context.Background(), root, false, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("initial sync code=%d", code)
	}
	target := filepath.Join(root, "search-indexer", "app", "internal", "connectguard", "connectguard_gen.go")
	body, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, append(body, []byte("\n// tampered\n")...), fsperm.File); err != nil {
		t.Fatal(err)
	}
	var errb bytes.Buffer
	if code := Sync(context.Background(), root, true, &bytes.Buffer{}, &errb); code != 1 || !strings.Contains(errb.String(), "DRIFT:") {
		t.Fatalf("tampered code=%d err=%s", code, errb.String())
	}
}

func TestRealRepoInSync(t *testing.T) {
	t.Parallel()
	var out, errb bytes.Buffer
	code := Sync(context.Background(), mustRepo(t), true, &out, &errb)
	if code != 0 {
		t.Fatalf("code=%d out=%s err=%s", code, out.String(), errb.String())
	}
}

func mustRepo(t *testing.T) string {
	t.Helper()
	c := exec.Command("git", "rev-parse", "--show-toplevel")
	c.Dir = filepath.Join("..", "..", "..")
	out, err := c.Output()
	if err != nil {
		wd, _ := os.Getwd()
		c = exec.Command("git", "rev-parse", "--show-toplevel")
		c.Dir = wd
		out, err = c.Output()
		if err != nil {
			t.Fatal(err)
		}
	}
	return strings.TrimSpace(string(out))
}
