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

func TestTitleRangeRejectsNumericPrefix(t *testing.T) {
	lines := []string{"# 失敗の型（FP-01 〜 FP-020）", ""}
	sections := []fpSection{{num: 1, nn: "01"}, {num: 2, nn: "02"}}
	var findings []Finding
	checkFPSequence(lines, sections, &findings)
	if len(findings) != 1 || !strings.Contains(findings[0].Message, "first-line title must contain 'FP-01 〜 FP-02'") {
		t.Fatalf("findings=%+v", findings)
	}
	exact := []string{"# 失敗の型（FP-01 〜 FP-02）"}
	findings = nil
	checkFPSequence(exact, sections, &findings)
	if len(findings) != 0 {
		t.Fatalf("exact title findings=%+v", findings)
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
