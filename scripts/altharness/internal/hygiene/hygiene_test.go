package hygiene

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

func TestZeroByteAndELF(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	gitInit(t, root)
	keep := filepath.Join(root, "keep.txt")
	if err := os.WriteFile(keep, []byte("hello"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	gitAdd(t, root, "keep.txt")
	gitCommit(t, root, "init")
	if err := os.WriteFile(keep, nil, fsperm.File); err != nil {
		t.Fatal(err)
	}
	elf := filepath.Join(root, "tool")
	if err := os.WriteFile(elf, []byte{0x7f, 'E', 'L', 'F', 0, 0, 0, 0}, fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	code := Run(context.Background(), []string{"--repo-root", root}, &out, &errb)
	if code != 1 {
		t.Fatalf("code=%d out=%s err=%s", code, out.String(), errb.String())
	}
	text := out.String()
	if !strings.Contains(text, "[hygiene.zero-byte-tracked] keep.txt:") {
		t.Fatalf("missing zero-byte: %s", text)
	}
	if !strings.Contains(text, "[hygiene.binary-artifact] tool: detected ELF binary artifact (untracked)") {
		t.Fatalf("missing elf: %s", text)
	}
}

func TestAllowlistedInitAndClean(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	gitInit(t, root)
	if err := os.WriteFile(filepath.Join(root, "README"), []byte("x"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	gitAdd(t, root, "README")
	gitCommit(t, root, "init")
	initFile := filepath.Join(root, "pkg", "__init__.py")
	if err := os.MkdirAll(filepath.Dir(initFile), fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(initFile, nil, fsperm.File); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	code := Run(context.Background(), []string{"--repo-root", root}, &out, &errb)
	if code != 0 {
		t.Fatalf("code=%d out=%s err=%s", code, out.String(), errb.String())
	}
	if !strings.Contains(out.String(), "OK: worktree hygiene clean") {
		t.Fatalf("out=%s", out.String())
	}
}

func TestNotGitFailsClosed(t *testing.T) {
	t.Parallel()
	var out, errb bytes.Buffer
	code := Run(context.Background(), []string{"--repo-root", t.TempDir()}, &out, &errb)
	if code != 2 || !strings.Contains(errb.String(), "not inside a git worktree") {
		t.Fatalf("code=%d err=%s", code, errb.String())
	}
}

func TestJavaClassIsNotMachO(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	gitInit(t, root)
	if err := os.WriteFile(filepath.Join(root, "README"), []byte("x"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	gitAdd(t, root, "README")
	gitCommit(t, root, "init")
	// CAFEBABE with nfat_arch huge (Java class major/minor) must not be a fat Mach-O.
	class := []byte{0xca, 0xfe, 0xba, 0xbe, 0x00, 0x00, 0x00, 0x3d, 0, 0, 0, 0}
	if err := os.WriteFile(filepath.Join(root, "A.class"), class, fsperm.File); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	code := Run(context.Background(), []string{"--repo-root", root}, &out, &errb)
	if code != 0 {
		t.Fatalf("code=%d out=%s err=%s", code, out.String(), errb.String())
	}
}

func gitInit(t *testing.T, root string) {
	t.Helper()
	cmd := exec.Command("git", "init", "-q")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	cfg := [][]string{
		{"config", "user.email", "t@example.com"},
		{"config", "user.name", "t"},
	}
	for _, args := range cfg {
		c := exec.Command("git", args...)
		c.Dir = root
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
}

func gitAdd(t *testing.T, root, path string) {
	t.Helper()
	c := exec.Command("git", "add", path)
	c.Dir = root
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v %s", err, out)
	}
}

func gitCommit(t *testing.T, root, msg string) {
	t.Helper()
	c := exec.Command("git", "commit", "-q", "-m", msg)
	c.Dir = root
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v %s", err, out)
	}
}
