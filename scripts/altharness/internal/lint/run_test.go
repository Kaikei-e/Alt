package lint

import (
	"alt/scripts/altharness/internal/fsperm"
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
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

func TestListDoesNotMutateLintPolicyEnv(t *testing.T) {
	repo := repoRoot(t)
	origVal, hadOrig := os.LookupEnv("LINT_POLICY")
	if err := os.Unsetenv("LINT_POLICY"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if hadOrig {
			_ = os.Setenv("LINT_POLICY", origVal)
		} else {
			_ = os.Unsetenv("LINT_POLICY")
		}
	})

	var out, errb bytes.Buffer
	if code := Run(context.Background(), repo, []string{"--list"}, &out, &errb); code != 0 {
		t.Fatalf("code=%d err=%s", code, errb.String())
	}
	if v, ok := os.LookupEnv("LINT_POLICY"); ok {
		t.Fatalf("lint.Run(--list) mutated process env LINT_POLICY=%q", v)
	}
}

func TestListTargetsParallelReadOnly(t *testing.T) {
	t.Parallel()
	repo := repoRoot(t)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			targets, err := ListTargets(repo)
			if err != nil || len(targets) == 0 {
				t.Errorf("ListTargets failed: err=%v len=%d", err, len(targets))
			}
			var out, errb bytes.Buffer
			if code := Run(context.Background(), repo, []string{"--list"}, &out, &errb); code != 0 {
				t.Errorf("Run(--list) code=%d err=%s", code, errb.String())
			}
		}()
	}
	wg.Wait()
}

func TestLauncherOlderMtimeRevertAndReadOnlySandbox(t *testing.T) {
	t.Parallel()
	repo := repoRoot(t)
	launcherSrc := filepath.Join(repo, "scripts", "altharness", "alt-harness")
	launcherBytes, err := os.ReadFile(launcherSrc)
	if err != nil {
		t.Fatal(err)
	}

	tmpRepo := t.TempDir()
	modDir := filepath.Join(tmpRepo, "scripts", "altharness")
	cmdDir := filepath.Join(modDir, "cmd", "alt-harness")
	if err := os.MkdirAll(cmdDir, fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	launcher := filepath.Join(modDir, "alt-harness")
	if err := os.WriteFile(launcher, launcherBytes, fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modDir, "go.mod"), []byte("module testlauncher\n\ngo 1.24\n"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	mainGo := filepath.Join(cmdDir, "main.go")
	v1 := "package main\nimport \"fmt\"\nfunc main() { fmt.Println(\"ver-one\") }\n"
	v2 := "package main\nimport \"fmt\"\nfunc main() { fmt.Println(\"ver-two-reverted\") }\n"
	if err := os.WriteFile(mainGo, []byte(v1), fsperm.File); err != nil {
		t.Fatal(err)
	}

	runLauncher := func(extraEnv ...string) (string, string, int) {
		t.Helper()
		c := exec.Command(launcher, "--help")
		c.Dir = tmpRepo
		var outBuf, errBuf bytes.Buffer
		c.Stdout = &outBuf
		c.Stderr = &errBuf
		if len(extraEnv) > 0 {
			c.Env = append(os.Environ(), extraEnv...)
		}
		err := c.Run()
		code := 0
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				code = ee.ExitCode()
			} else {
				t.Fatalf("exec failed: %v", err)
			}
		}
		return outBuf.String(), errBuf.String(), code
	}

	// 1. Initial build produces ver-one.
	stdout, stderr, code := runLauncher()
	if code != 0 || strings.TrimSpace(stdout) != "ver-one" {
		t.Fatalf("v1 build: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}

	// 2. Overwrite main.go with an OLDER mtime (like cp -p / tar revert) -> MUST rebuild to ver-two-reverted!
	if err := os.WriteFile(mainGo, []byte(v2), fsperm.File); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-10 * time.Minute)
	if err := os.Chtimes(mainGo, past, past); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(cmdDir, past, past); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code = runLauncher()
	if code != 0 || strings.TrimSpace(stdout) != "ver-two-reverted" {
		t.Fatalf("older-mtime revert did not rebuild: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}

	// 3. Fresh prebuilt binary MUST succeed in a 0500 read-only TMPDIR/GOCACHE sandbox with zero stderr warnings!
	roDir := filepath.Join(t.TempDir(), "ro")
	if err := os.MkdirAll(roDir, fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(roDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(roDir, fsperm.Dir) })
	stdout, stderr, code = runLauncher("GOCACHE="+filepath.Join(roDir, "gocache"), "TMPDIR="+roDir)
	if code != 0 || strings.TrimSpace(stdout) != "ver-two-reverted" || stderr != "" {
		t.Fatalf("prebuilt fresh binary failed in RO sandbox: code=%d stdout=%q stderr=%q", code, stdout, stderr)
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
