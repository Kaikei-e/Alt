package pki

import (
	"alt/scripts/altharness/internal/fsperm"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCheckCurrentRepo(t *testing.T) {
	root := repoRoot(t)
	code, out, errb := runCLI(context.Background(), "--repo-root", root, "--check")
	if code != 0 {
		t.Fatalf("rc=%d\nstdout:\n%s\nstderr:\n%s", code, out, errb)
	}
	if !strings.Contains(out, "OK: all 6 Python services match canonical PKI templates.") {
		t.Fatalf("stdout: %s", out)
	}
}

func TestDriftMessagesIncludePath(t *testing.T) {
	root := repoRoot(t)
	tmp := t.TempDir()
	code, _, errb := runCLI(context.Background(), "--repo-root", tmp)
	if code != 0 {
		t.Fatalf("sync rc=%d %s", code, errb)
	}
	// Templates come from the real checkout even when --repo-root is a temp tree.
	_ = root
	code, _, errb = runCLI(context.Background(), "--repo-root", tmp, "--check")
	if code != 0 {
		t.Fatalf("clean check rc=%d %s", code, errb)
	}

	target := filepath.Join(tmp, "news-creator", "app", "news_creator", "infra", "pki", "ops.py")
	body, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, append(body, []byte("\n# manual tamper\n")...), fsperm.File); err != nil {
		t.Fatal(err)
	}
	code, out, errb := runCLI(context.Background(), "--repo-root", tmp, "--check")
	combined := out + errb
	if code != 1 || !strings.Contains(combined, "news-creator/app/news_creator/infra/pki/ops.py") {
		t.Fatalf("tamper rc=%d %s", code, combined)
	}

	extra := filepath.Join(tmp, "tag-generator", "app", "tag_generator", "infra", "pki", "rogue_helper.py")
	if err := os.WriteFile(extra, []byte("# unexpected manual file\n"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	// retamper was the previous failure; regenerate a clean tree for the extra-file case.
	tmp2 := t.TempDir()
	if code, _, errb = runCLI(context.Background(), "--repo-root", tmp2); code != 0 {
		t.Fatal(errb)
	}
	extra = filepath.Join(tmp2, "tag-generator", "app", "tag_generator", "infra", "pki", "rogue_helper.py")
	if err := os.WriteFile(extra, []byte("# unexpected manual file\n"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	code, out, errb = runCLI(context.Background(), "--repo-root", tmp2, "--check")
	combined = out + errb
	if code != 1 || !strings.Contains(combined, "tag-generator/app/tag_generator/infra/pki/rogue_helper.py") {
		t.Fatalf("extra rc=%d %s", code, combined)
	}

	tmp3 := t.TempDir()
	if code, _, errb = runCLI(context.Background(), "--repo-root", tmp3); code != 0 {
		t.Fatal(errb)
	}
	missing := filepath.Join(tmp3, "recap-subworker", "recap_subworker", "app", "infra", "pki", "state.py")
	if err := os.Remove(missing); err != nil {
		t.Fatal(err)
	}
	code, out, errb = runCLI(context.Background(), "--repo-root", tmp3, "--check")
	combined = out + errb
	if code != 1 || !strings.Contains(combined, "recap-subworker/recap_subworker/app/infra/pki/state.py") {
		t.Fatalf("missing rc=%d %s", code, combined)
	}
}

func runCLI(ctx context.Context, args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := Run(ctx, args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
}
