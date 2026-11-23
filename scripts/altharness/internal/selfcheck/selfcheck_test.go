package selfcheck

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRealRepoPasses(t *testing.T) {
	root := findRepo(t)
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{"--root", root}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code=%d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "agent_harness_selfcheck: OK") || !strings.Contains(out, "codex_hooks=8") || !strings.Contains(out, "shared_blocks=2") {
		t.Fatalf("stdout=%s", out)
	}
}

func TestJudgmentRatchetCeiling(t *testing.T) {
	root := findRepo(t)
	text, err := os.ReadFile(filepath.Join(root, ruleLedgerRel))
	if err != nil {
		t.Fatal(err)
	}
	if extractMaxJudgment(string(text)) != 6 {
		t.Fatalf("ledger ceiling=%d", extractMaxJudgment(string(text)))
	}
}

func findRepo(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "scripts", "agent-harness", "harness_selfcheck.py")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repo not found")
		}
		dir = parent
	}
}
