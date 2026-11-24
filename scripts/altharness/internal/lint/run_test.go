package lint

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestListExcludesHarnessModule(t *testing.T) {
	repo := repoRoot(t)
	var out, errb bytes.Buffer
	code := Run(context.Background(), repo, []string{"--list"}, &out, &errb)
	if code != 0 {
		t.Fatalf("code=%d err=%s", code, errb.String())
	}
	if strings.Contains(out.String(), "scripts/altharness") {
		t.Fatalf("harness module listed:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "repo\n") || !strings.Contains(out.String(), "harness\n") {
		t.Fatalf("missing named targets:\n%s", out.String())
	}
}

func TestDiskFloor(t *testing.T) {
	t.Setenv("ALT_LINT_FAKE_FREE_KB", "1024")
	t.Setenv("ALT_LINT_MIN_FREE_KB", "2048")
	repo := t.TempDir()
	var out, errb bytes.Buffer
	code := Run(context.Background(), repo, []string{"repo"}, &out, &errb)
	if code != 1 || !strings.Contains(errb.String(), "ディスク空き容量不足") {
		t.Fatalf("code=%d err=%s", code, errb.String())
	}
}

func TestUsage(t *testing.T) {
	var out, errb bytes.Buffer
	code := Run(context.Background(), t.TempDir(), nil, &out, &errb)
	if code != 2 || !strings.Contains(errb.String(), "usage: run.sh") {
		t.Fatalf("code=%d err=%s", code, errb.String())
	}
}

func repoRoot(t *testing.T) string {
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
