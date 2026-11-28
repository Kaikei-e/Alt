package selfcheck

import (
	"alt/scripts/altharness/internal/fsperm"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
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
	if !strings.Contains(out, "agent_harness_selfcheck: OK") || !strings.Contains(out, "codex_hooks=10") || !strings.Contains(out, "claude_hooks=15") || !strings.Contains(out, "agy_hooks=6") || !strings.Contains(out, "shared_blocks=2") {
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

func TestStdlibGovernanceTOMLParser(t *testing.T) {
	val, found, err := parseGovernanceMaxJudgmentTOML("version = 1\n\n[governance]\nmax_judgment_rules = 6 # inline comment\n\n[targets.foo]\npython_noqa = 10\n")
	if err != nil || !found || val != 6 {
		t.Fatalf("got (%d, %v, %v), want (6, true, nil)", val, found, err)
	}
	_, found, err = parseGovernanceMaxJudgmentTOML("version = 1\n[other]\nmax_judgment_rules = 99\n")
	if err != nil || found {
		t.Fatalf("other section should not match: (%v, %v)", found, err)
	}
	_, _, err = parseGovernanceMaxJudgmentTOML("[broken\n")
	if err == nil {
		t.Fatalf("expected parse error for unclosed header")
	}
}

func TestCheckAgyHookFileSchema(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	script := filepath.Join(root, "scripts", "altharness", "alt-harness")
	if err := os.MkdirAll(filepath.Dir(script), fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	if err := fsperm.WriteExecutable(script, []byte("#!/usr/bin/env bash\n")); err != nil {
		t.Fatal(err)
	}

	// Reject Claude/Codex "hooks" wrapper.
	var findings []Finding
	checkAgyHookFile(root, ".agents/hooks.json", map[string]any{"hooks": map[string]any{}}, &findings)
	if len(findings) == 0 || !strings.Contains(findings[0].Message, `not a "hooks" wrapper`) {
		t.Fatalf("expected wrapper rejection, got %+v", findings)
	}

	// Reject grouped SessionStart (must be flat).
	findings = nil
	checkAgyHookFile(root, ".agents/hooks.json", map[string]any{
		"alt": map[string]any{
			"SessionStart": []any{
				map[string]any{"hooks": []any{map[string]any{"type": "command", "command": "echo hi"}}},
			},
		},
	}, &findings)
	foundFlatErr := false
	for _, f := range findings {
		if strings.Contains(f.Message, "flat handler objects") {
			foundFlatErr = true
		}
	}
	if !foundFlatErr {
		t.Fatalf("expected flat SessionStart error, got %+v", findings)
	}
}

func TestCheckHookFileRejectsPreToolUseSwallow(t *testing.T) {
	root := t.TempDir()
	var findings []Finding
	cfg := map[string]any{
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{
					"matcher": "Edit",
					"hooks": []any{
						map[string]any{
							"type":      "command",
							"onFailure": "block",
							"command":   `[ -f "$s" ] || exit 0; python3 "$s"`,
						},
						map[string]any{
							"type":      "command",
							"onFailure": "block",
							"command":   `python3 "$s" || true`,
						},
					},
				},
			},
		},
	}
	checkHookFile(root, ".claude/settings.json", cfg, true, &findings)
	swallows := 0
	for _, f := range findings {
		if f.ID == "H3" && strings.Contains(f.Message, "swallows failure") {
			swallows++
		}
	}
	if swallows != 2 {
		t.Fatalf("expected 2 H3 swallow findings, got %d (%+v)", swallows, findings)
	}
}

func TestAdrPreflightSkillWiringPassesSelfcheck(t *testing.T) {
	t.Parallel()
	root := findRepo(t)
	raw, err := os.ReadFile(filepath.Join(root, ".claude", "skills", "adr-preflight", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile("(?s)```json\\s*(\\{.*?\\})\\s*```")
	m := re.FindSubmatch(raw)
	if len(m) < 2 {
		t.Fatalf("adr-preflight SKILL.md missing JSON wiring snippet")
	}
	var cfg map[string]any
	if err := json.Unmarshal(m[1], &cfg); err != nil {
		t.Fatalf("adr-preflight SKILL.md JSON invalid: %v", err)
	}
	hooksMap, ok := cfg["hooks"].(map[string]any)
	if !ok {
		t.Fatalf("SKILL.md snippet missing hooks object")
	}

	var findings []Finding
	sawPreToolUsePreflight := false
	for event, entries := range hooksMap {
		for _, pair := range iterHooks(entries) {
			cmd, _ := pair.hook["command"].(string)
			checkHookCommandRefs(root, "SKILL.md", cmd, &findings)
			if event == "PreToolUse" {
				if pair.hook["onFailure"] != "block" {
					findings = append(findings, Finding{"H3", "SKILL.md", `PreToolUse command hook lacks "onFailure": "block"`})
				}
				if strings.Contains(cmd, "|| exit 0") || strings.Contains(cmd, "|| true") {
					findings = append(findings, Finding{"H3", "SKILL.md", `PreToolUse command hook swallows failure`})
				}
				if sub, _, ok := extractAltHarnessCall(cmd); ok && sub == "preflight" {
					sawPreToolUsePreflight = true
				}
			}
		}
	}
	if len(findings) != 0 {
		t.Fatalf("SKILL.md wiring snippet produced selfcheck findings: %+v", findings)
	}
	if !sawPreToolUsePreflight {
		t.Fatalf("SKILL.md PreToolUse snippet did not wire alt-harness preflight")
	}
}

func TestExtractAltHarnessCallDirectAndVarIndirection(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		cmd     string
		wantSub string
		wantOK  bool
	}{
		{
			name:    "direct quoted invocation",
			cmd:     `"$CLAUDE_PROJECT_DIR/scripts/altharness/alt-harness" preflight --hook`,
			wantSub: "preflight",
			wantOK:  true,
		},
		{
			name:    "shell variable indirection",
			cmd:     `s="$CLAUDE_PROJECT_DIR/scripts/altharness/alt-harness"; [ -x "$s" ] && "$s" preflight --hook`,
			wantSub: "preflight",
			wantOK:  true,
		},
		{
			name:    "bare invocation without subcommand",
			cmd:     `"$CLAUDE_PROJECT_DIR/scripts/altharness/alt-harness"`,
			wantSub: "",
			wantOK:  true,
		},
		{
			name:    "unrelated command",
			cmd:     `echo hello`,
			wantSub: "",
			wantOK:  false,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sub, _, ok := extractAltHarnessCall(tc.cmd)
			if ok != tc.wantOK || sub != tc.wantSub {
				t.Fatalf("got (%q, %v), want (%q, %v)", sub, ok, tc.wantSub, tc.wantOK)
			}
		})
	}
}

func findRepo(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "scripts", "altharness", "alt-harness")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repo not found")
		}
		dir = parent
	}
}
