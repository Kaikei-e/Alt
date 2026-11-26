package packet

import (
	"alt/scripts/altharness/internal/fsperm"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func shaHex(s string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(s)))
	return hex.EncodeToString(sum[:])
}

func runPacketCLI(args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func basePacketMap() map[string]any {
	verifyCmd := "./scripts/lint/run.sh scripts"
	return map[string]any{
		"packet_version": "1.0",
		"packet_id":      "SPEC-04-SAMPLE",
		"scope": map[string]any{
			"target":          "scripts",
			"allowed_paths":   []any{"scripts/agent-harness/**", "scripts/lint/{harness,other}.sh"},
			"forbidden_paths": []any{"scripts/agent-harness/forbidden.go"},
			"max_files":       10,
			"max_functions":   3,
			"allowed_rules":   []any{"S101", "PTH123"},
			"stop_condition":  "対象作業量の上限到達または全ルール解消時点で handoff を作成し終了する",
		},
		"execution_policy": map[string]any{
			"foreground_only":             true,
			"forbid_repo_wide_test_build": true,
			"forbid_bulk_autofix":         true,
			"verification_command":        verifyCmd,
			"max_command_budget_s":        180,
			"measured": map[string]any{
				"duration_s":     1.25,
				"exit_code":      0,
				"at_commit":      "0123456",
				"command_sha256": shaHex(verifyCmd),
			},
		},
		"handoff_requirements": map[string]any{
			"require_modified_files_and_lines": true,
			"require_runtime_behavior_impact":  true,
			"require_change_rationale":         true,
		},
	}
}

func baseHandoffMap() map[string]any {
	return map[string]any{
		"packet_id": "SPEC-04-SAMPLE",
		"status":    "completed",
		"modified_files": []any{
			map[string]any{"path": "scripts/agent-harness/example.go", "added_lines": 12, "deleted_lines": 4},
		},
		"modified_functions_count": 2,
		"runtime_behavior_impact": map[string]any{
			"has_change":         false,
			"checked_dimensions": []any{"timeout", "error_message", "permissions", "type_conversion", "default_value", "control_flow"},
			"summary":            "変更なし（静的検査の型注釈と置換のみで実行時挙動は同一）",
		},
		"rationale":    "対象ルール S101/PTH123 解消のため最小限の静的リファクタを実施。仕様変更なし。",
		"verification": map[string]any{"command": "./scripts/lint/run.sh scripts", "exit_code": 0},
	}
}

func writeJSONFile(t *testing.T, dir, name string, payload any) string {
	t.Helper()
	p := filepath.Join(dir, name)
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, append(raw, '\n'), fsperm.File); err != nil {
		t.Fatal(err)
	}
	return p
}

func setVerifyCmd(pkt map[string]any, cmd string) {
	ep := pkt["execution_policy"].(map[string]any)
	ep["verification_command"] = cmd
	if m, ok := ep["measured"].(map[string]any); ok {
		m["command_sha256"] = shaHex(cmd)
	}
}

func writeStubRunSh(t *testing.T, repo string, exitCode int, sleepSec string) {
	t.Helper()
	p := filepath.Join(repo, "scripts", "lint", "run.sh")
	if err := os.MkdirAll(filepath.Dir(p), fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	body := "#!/usr/bin/env bash\nset -euo pipefail\n"
	if sleepSec != "" {
		body += "sleep " + sleepSec + "\n"
	}
	body += "exit " + string(rune('0'+exitCode)) + "\n"
	if err := fsperm.WriteExecutable(p, []byte(body)); err != nil {
		t.Fatal(err)
	}
}

func runGitCmd(t *testing.T, repo string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, string(out))
	}
	return strings.TrimSpace(string(out))
}

func validPacketMD(extraBody string) string {
	return "---\n" +
		"packet_version: \"1.0\"\n" +
		"packet_id: \"MD-PKT-01\"\n" +
		"scope:\n" +
		"  target: \"scripts\"\n" +
		"  allowed_paths:\n    - \"scripts/agent-harness/**\"\n" +
		"  max_files: 5\n" +
		"  max_functions: 3\n" +
		"  allowed_rules:\n    - \"G112\"\n    - \"errorlint\"\n" +
		"  stop_condition: \"作業量上限に到達したら handoff を出力して終了する\"\n" +
		"execution_policy:\n" +
		"  foreground_only: true\n" +
		"  forbid_repo_wide_test_build: true\n" +
		"  forbid_bulk_autofix: true\n" +
		"  verification_command: \"./scripts/lint/run.sh scripts\"\n" +
		"  max_command_budget_s: 180\n" +
		"  measured:\n" +
		"    duration_s: 1.5\n" +
		"    exit_code: 0\n" +
		"    at_commit: \"0123456\"\n" +
		"    command_sha256: \"" + shaHex("./scripts/lint/run.sh scripts") + "\"\n" +
		"handoff_requirements:\n" +
		"  require_modified_files_and_lines: true\n" +
		"  require_runtime_behavior_impact: true\n" +
		"  require_change_rationale: true\n" +
		"---\n\n" +
		"# タスクパケット MD-PKT-01\n\n" +
		"## 1. 範囲と作業量制約 (Bounded Scope)\n- 対象: `scripts/agent-harness/**`\n\n" +
		"## 2. 実行・検証手順\n- 前景実行のみ許可（背景 `&` 禁止）。\n" +
		"- `golangci-lint --fix`、`cargo clippy --fix`、`ruff check --unsafe-fixes`、`biome check --write --unsafe` などの一括自動修正は禁止。\n" +
		"```bash\n./scripts/lint/run.sh scripts 2>&1\n```\n" + extraBody + "\n" +
		"## 3. Hand-off 記述必須項目\n1. 修正ファイルと行数\n2. 実行時挙動の変更可能性の有無\n3. 変更理由\n"
}

func validHandoffMD(id string) string {
	return "---\n" +
		"packet_id: \"" + id + "\"\n" +
		"status: \"completed\"\n" +
		"modified_files:\n" +
		"  - path: \"scripts/agent-harness/example.go\"\n" +
		"    added_lines: 20\n" +
		"    deleted_lines: 5\n" +
		"modified_functions_count: 2\n" +
		"runtime_behavior_impact:\n" +
		"  has_change: false\n" +
		"  checked_dimensions:\n" +
		"    - \"timeout\"\n    - \"error_message\"\n    - \"permissions\"\n    - \"type_conversion\"\n    - \"default_value\"\n    - \"control_flow\"\n" +
		"  summary: \"変更なし（境界チェック確認済み）\"\n" +
		"rationale: \"変更なし（対象 lint ルール準拠のみ）\"\n" +
		"verification:\n" +
		"  command: \"./scripts/lint/run.sh scripts\"\n" +
		"  exit_code: 0\n" +
		"---\n\n# Hand-off 報告 " + id + "\n\n## 1. 修正ファイルと行数\n\n## 2. 実行時挙動の変更可能性の有無\n\n## 3. 変更理由\n"
}

func TestSchemaStructureAndValidPacketsParallel(t *testing.T) {
	t.Parallel()

	t.Run("schema draft and additionalProperties", func(t *testing.T) {
		t.Parallel()
		raw, err := os.ReadFile(schemaPath())
		if err != nil {
			t.Fatal(err)
		}
		var schema map[string]any
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatal(err)
		}
		if schema["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
			t.Fatalf("$schema=%v", schema["$schema"])
		}
		defs := schema["$defs"].(map[string]any)
		for _, name := range []string{"TaskPacket", "TaskHandoff", "Scope"} {
			def := defs[name].(map[string]any)
			if def["additionalProperties"] != false {
				t.Fatalf("%s additionalProperties=%v", name, def["additionalProperties"])
			}
		}
	})

	t.Run("valid JSON, --format json, multi-target, MD frontmatter, MD JSON block", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		pJSON := writeJSONFile(t, tmp, "valid.json", basePacketMap())
		if rc, out, errB := runPacketCLI("validate-packet", pJSON); rc != exitOK {
			t.Fatalf("valid json rc=%d out=%s err=%s", rc, out, errB)
		}
		if rc, out, errB := runPacketCLI("validate-packet", pJSON, "--format", "json"); rc != exitOK || !strings.Contains(out, `"valid": true`) {
			t.Fatalf("--format json rc=%d out=%s err=%s", rc, out, errB)
		}

		multi := basePacketMap()
		multi["scope"].(map[string]any)["target"] = []any{"scripts", "harness"}
		setVerifyCmd(multi, "./scripts/lint/run.sh scripts && LINT_BASE=HEAD~1 ./scripts/lint/run.sh harness")
		pMulti := writeJSONFile(t, tmp, "multi.json", multi)
		if rc, _, errB := runPacketCLI("validate-packet", pMulti); rc != exitOK {
			t.Fatalf("multi rc=%d err=%s", rc, errB)
		}

		mdPath := filepath.Join(tmp, "valid.md")
		_ = os.WriteFile(mdPath, []byte(validPacketMD("")), fsperm.File)
		if rc, _, errB := runPacketCLI("validate-packet", mdPath); rc != exitOK {
			t.Fatalf("valid md rc=%d err=%s", rc, errB)
		}

		rawJSON, _ := json.MarshalIndent(basePacketMap(), "", "  ")
		jbPath := filepath.Join(tmp, "json_block.md")
		_ = os.WriteFile(jbPath, []byte("# P\n\n```json\n"+string(rawJSON)+"\n```\n\n## 1. 範囲と作業量制約\n\n## 2. 実行・検証手順\n\n## 3. Hand-off 記述必須項目\n"), fsperm.File)
		if rc, _, errB := runPacketCLI("validate-packet", jbPath); rc != exitOK {
			t.Fatalf("json block md rc=%d err=%s", rc, errB)
		}
	})
}

func TestInvalidPacketsAndMarkdownBodyParallel(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		mutate func(m map[string]any)
		substr string
	}{
		{"timeout_minutes in scope", func(m map[string]any) { m["scope"].(map[string]any)["timeout_minutes"] = 30 }, "timeout_minutes"},
		{"packet_version 2.0", func(m map[string]any) { m["packet_version"] = "2.0" }, "packet_version"},
		{"invalid packet_id", func(m map[string]any) { m["packet_id"] = "-bad id!" }, "packet_id"},
		{"max_files > 25", func(m map[string]any) { m["scope"].(map[string]any)["max_files"] = 26 }, "max_files"},
		{"boolean max_files", func(m map[string]any) { m["scope"].(map[string]any)["max_files"] = true }, "max_files"},
		{"max_functions > 20", func(m map[string]any) { m["scope"].(map[string]any)["max_functions"] = 21 }, "max_functions"},
		{"traversal allowed_paths", func(m map[string]any) { m["scope"].(map[string]any)["allowed_paths"] = []any{"scripts/../etc/passwd"} }, ""},
		{"abs allowed_paths", func(m map[string]any) { m["scope"].(map[string]any)["allowed_paths"] = []any{"/etc/passwd"} }, ""},
		{"wildcard allowed_rules", func(m map[string]any) { m["scope"].(map[string]any)["allowed_rules"] = []any{"*"} }, ""},
		{"ALL allowed_rules", func(m map[string]any) { m["scope"].(map[string]any)["allowed_rules"] = []any{"ALL"} }, ""},
		{"TBD stop_condition", func(m map[string]any) { m["scope"].(map[string]any)["stop_condition"] = "TBD" }, ""},
		{"foreground_only=false", func(m map[string]any) { m["execution_policy"].(map[string]any)["foreground_only"] = false }, ""},
		{"forbid_bulk_autofix=false", func(m map[string]any) { m["execution_policy"].(map[string]any)["forbid_bulk_autofix"] = false }, ""},
		{"missing run.sh", func(m map[string]any) { setVerifyCmd(m, "go test ./...") }, "run.sh"},
		{"bg & in verify", func(m map[string]any) { setVerifyCmd(m, "./scripts/lint/run.sh scripts &") }, ""},
		{"repo-wide run.sh all", func(m map[string]any) { setVerifyCmd(m, "./scripts/lint/run.sh scripts && ./scripts/lint/run.sh all") }, "FORBIDDEN_REPO_WIDE_COMMAND"},
		{"repo-wide make lint", func(m map[string]any) { setVerifyCmd(m, "make lint && ./scripts/lint/run.sh scripts") }, "FORBIDDEN_REPO_WIDE_COMMAND"},
		{"disabled handoff req", func(m map[string]any) { m["handoff_requirements"].(map[string]any)["require_change_rationale"] = false }, ""},
		{"invalid policy enum", func(m map[string]any) { m["execution_policy"].(map[string]any)["policy"] = "allow-all" }, "policy"},
		{"block-new without flag", func(m map[string]any) { m["execution_policy"].(map[string]any)["policy"] = "block-new" }, "POLICY_COMMAND_MISMATCH"},
		{"block-new with fix-existing env", func(m map[string]any) {
			m["execution_policy"].(map[string]any)["policy"] = "block-new"
			setVerifyCmd(m, "LINT_POLICY=fix-existing ./scripts/lint/run.sh scripts")
		}, "POLICY_COMMAND_MISMATCH"},
		{"fix-existing with block-new env", func(m map[string]any) {
			m["execution_policy"].(map[string]any)["policy"] = "fix-existing"
			setVerifyCmd(m, "LINT_POLICY=block-new ./scripts/lint/run.sh scripts")
		}, "POLICY_COMMAND_MISMATCH"},
		{"over-budget measured", func(m map[string]any) {
			ep := m["execution_policy"].(map[string]any)
			ep["max_command_budget_s"] = 30
			ep["measured"].(map[string]any)["duration_s"] = 30.5
		}, "VERIFICATION_BUDGET_EXCEEDED"},
		{"stale command_sha256", func(m map[string]any) {
			m["execution_policy"].(map[string]any)["measured"].(map[string]any)["command_sha256"] = strings.Repeat("0", 64)
		}, "STALE_MEASURED_COMMAND"},
		{"missing command_sha256", func(m map[string]any) {
			delete(m["execution_policy"].(map[string]any)["measured"].(map[string]any), "command_sha256")
		}, "command_sha256"},
		{"all-zero at_commit", func(m map[string]any) {
			m["execution_policy"].(map[string]any)["measured"].(map[string]any)["at_commit"] = "0000000"
		}, "PLACEHOLDER_MEASURED_COMMIT"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tmp := t.TempDir()
			m := basePacketMap()
			tc.mutate(m)
			p := writeJSONFile(t, tmp, "bad.json", m)
			rc, out, errB := runPacketCLI("validate-packet", p)
			comb := strings.ToLower(out + "\n" + errB)
			if rc != exitInvalid || (tc.substr != "" && !strings.Contains(comb, strings.ToLower(tc.substr))) {
				t.Fatalf("rc=%d out=%s err=%s", rc, out, errB)
			}
		})
	}

	t.Run("invalid markdown body headings and forbidden instructions", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		badBodies := []struct {
			name string
			body string
		}{
			{"missing heading", strings.Replace(validPacketMD(""), "## 2. 実行・検証手順", "## 2. メモ", 1)},
			{"prose golangci-lint --fix", validPacketMD("- まず `golangci-lint run --fix ./...` を実行せよ。\n")},
			{"block clippy --fix", validPacketMD("```bash\ncargo clippy --fix --allow-dirty\n```\n")},
			{"block ruff --unsafe-fixes", validPacketMD("```sh\nruff check --unsafe-fixes .\n```\n")},
			{"block biome --unsafe", validPacketMD("```bash\nbiome check --write --unsafe .\n```\n")},
			{"block bg &", validPacketMD("```bash\n./scripts/lint/run.sh scripts &\n```\n")},
			{"inline prose bg &", validPacketMD("- `./scripts/lint/run.sh scripts &` でバックグラウンド実行する。\n")},
		}
		for _, tc := range badBodies {
			p := filepath.Join(tmp, strings.ReplaceAll(tc.name, " ", "_")+".md")
			_ = os.WriteFile(p, []byte(tc.body), fsperm.File)
			if rc, out, errB := runPacketCLI("validate-packet", p); rc != exitInvalid {
				t.Fatalf("%s expected invalid, got rc=%d out=%s err=%s", tc.name, rc, out, errB)
			}
		}
	})
}

func TestHandoffCrossValidationAndCandidatesParallel(t *testing.T) {
	t.Parallel()

	t.Run("standalone, cross-validation, placeholders, and paths", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		pktPath := writeJSONFile(t, tmp, "pkt.json", basePacketMap())
		hoPath := writeJSONFile(t, tmp, "ho.json", baseHandoffMap())
		if rc, _, errB := runPacketCLI("validate-handoff", hoPath, "--packet", pktPath); rc != exitOK {
			t.Fatalf("valid handoff rc=%d err=%s", rc, errB)
		}

		// Brace glob match + partial_bounded_stop non-zero + MD handoff.
		hoBrace := baseHandoffMap()
		hoBrace["modified_files"] = []any{map[string]any{"path": "scripts/lint/harness.sh", "added_lines": 5, "deleted_lines": 0}}
		if rc, _, errB := runPacketCLI("validate-handoff", writeJSONFile(t, tmp, "ho_brace.json", hoBrace), "--packet", pktPath); rc != exitOK {
			t.Fatalf("brace glob rc=%d err=%s", rc, errB)
		}
		hoPartial := baseHandoffMap()
		hoPartial["status"] = "partial_bounded_stop"
		hoPartial["verification"].(map[string]any)["exit_code"] = 1
		if rc, _, errB := runPacketCLI("validate-handoff", writeJSONFile(t, tmp, "ho_partial.json", hoPartial), "--packet", pktPath); rc != exitOK {
			t.Fatalf("partial rc=%d err=%s", rc, errB)
		}
		pktMD := filepath.Join(tmp, "p.md")
		hoMD := filepath.Join(tmp, "h.md")
		_ = os.WriteFile(pktMD, []byte(validPacketMD("")), fsperm.File)
		_ = os.WriteFile(hoMD, []byte(validHandoffMD("MD-PKT-01")), fsperm.File)
		if rc, _, errB := runPacketCLI("validate-handoff", hoMD, "--packet", pktMD); rc != exitOK {
			t.Fatalf("md handoff rc=%d err=%s", rc, errB)
		}

		for _, badTok := range []string{"", "TBD", "N/A", "-", "none", "None", "null", "なし", "無し"} {
			h := baseHandoffMap()
			h["rationale"] = badTok
			if rc, _, _ := runPacketCLI("validate-handoff", writeJSONFile(t, tmp, "r_"+shaHex(badTok)[:8]+".json", h)); rc != exitInvalid {
				t.Fatalf("placeholder rationale %q accepted", badTok)
			}
		}

		// Completed with exit_code=1, packet_id mismatch, max_files, max_functions, outside, forbidden, unnormalized.
		hoSilent := baseHandoffMap()
		hoSilent["verification"].(map[string]any)["exit_code"] = 1
		if rc, _, _ := runPacketCLI("validate-handoff", writeJSONFile(t, tmp, "silent.json", hoSilent)); rc != exitInvalid {
			t.Fatalf("completed exit 1 should fail")
		}
		hoBadVerify := baseHandoffMap()
		hoBadVerify["verification"].(map[string]any)["command"] = "golangci-lint run ./..."
		if rc, _, errB := runPacketCLI("validate-handoff", writeJSONFile(t, tmp, "bad_v.json", hoBadVerify), "--packet", pktPath); rc != exitInvalid || !strings.Contains(errB, "MISSING_VERIFICATION_TARGET") {
			t.Fatalf("bad verify rc=%d err=%s", rc, errB)
		}
		for _, unnorm := range []string{"scripts//agent-harness/forbidden.go", "scripts/./agent-harness/forbidden.go"} {
			h := baseHandoffMap()
			h["modified_files"] = []any{map[string]any{"path": unnorm, "added_lines": 1, "deleted_lines": 0}}
			if rc, _, _ := runPacketCLI("validate-handoff", writeJSONFile(t, tmp, "u_"+shaHex(unnorm)[:8]+".json", h), "--packet", pktPath); rc != exitInvalid {
				t.Fatalf("unnormalized %q should fail", unnorm)
			}
		}
	})

	t.Run("CHECK-01 candidate responses and multi-language test existence", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		repo := initGitRepo(t)
		_ = os.MkdirAll(filepath.Join(repo, "pkg"), fsperm.Dir)
		_ = os.WriteFile(filepath.Join(repo, "pkg", "service_test.go"), []byte("package pkg\nimport \"testing\"\nfunc TestSuite(t *testing.T) {\n\tt.Run(\"SubCase\", func(t *testing.T) {})\n}\nfunc setupServer() {}\ntype MySuite struct{}\nfunc (s *MySuite) TestSuiteMethod() {}\n"), fsperm.File)
		_ = os.MkdirAll(filepath.Join(repo, "tests"), fsperm.Dir)
		_ = os.WriteFile(filepath.Join(repo, "tests", "test_x.py"), []byte("def _helper():\n    return 1\ndef test_top():\n    assert True\nclass TestCls:\n    async def test_bar(self):\n        assert True\n"), fsperm.File)
		_ = os.MkdirAll(filepath.Join(repo, "src"), fsperm.Dir)
		_ = os.WriteFile(filepath.Join(repo, "src", "lib.rs"), []byte("pub fn helper_not_a_test() -> bool { true }\n#[cfg(test)]\nmod tests {\n    #[tokio::test]\n    async fn test_async_rust() {}\n    #[rstest]\n    fn parses_valid_url() {}\n}\n"), fsperm.File)
		_ = os.MkdirAll(filepath.Join(repo, "web"), fsperm.Dir)
		_ = os.WriteFile(filepath.Join(repo, "web", "widget.test.ts"), []byte("describe(\"Widget\", () => {\n  it(\"should render widget\", () => {});\n});\n"), fsperm.File)
		runGitCmd(t, repo, "add", ".")
		runGitCmd(t, repo, "commit", "-q", "-m", "tests")

		pktPath := writeJSONFile(t, tmp, "pkt.json", basePacketMap())
		reportPath := writeJSONFile(t, tmp, "rep.json", []any{
			map[string]any{"id": "cand-001", "file": "scripts/agent-harness/example.go", "function": "example_fn", "kind": "go.control_flow_return_change", "before": "if=1", "after": "if=0"},
		})
		emptyReport := writeJSONFile(t, tmp, "empty.json", []any{})

		// Unanswered candidate -> UNANSWERED_BEHAVIOR_CANDIDATE.
		if rc, _, errB := runPacketCLI("validate-handoff", writeJSONFile(t, tmp, "unans.json", baseHandoffMap()), "--packet", pktPath, "--screener-report", reportPath); rc != exitInvalid || !strings.Contains(errB, "UNANSWERED_BEHAVIOR_CANDIDATE") {
			t.Fatalf("unanswered rc=%d err=%s", rc, errB)
		}

		// Contradictory has_change=false with intended -> CONTRADICTORY_BEHAVIOR_IMPACT.
		hoContra := baseHandoffMap()
		hoContra["runtime_behavior_impact"].(map[string]any)["candidate_responses"] = []any{
			map[string]any{"candidate_id": "cand-001", "disposition": "intended", "test_name": "TestSuite"},
		}
		if rc, _, errB := runPacketCLI("validate-handoff", writeJSONFile(t, tmp, "contra.json", hoContra), "--packet", pktPath, "--repo-root", repo); rc != exitInvalid || !strings.Contains(errB, "CONTRADICTORY_BEHAVIOR_IMPACT") {
			t.Fatalf("contra rc=%d err=%s", rc, errB)
		}

		// Unreverted unintended -> UNREVERTED_UNINTENDED_CANDIDATE; empty report -> PASS.
		hoUnrev := baseHandoffMap()
		hoUnrev["runtime_behavior_impact"].(map[string]any)["candidate_responses"] = []any{
			map[string]any{"candidate_id": "cand-001", "disposition": "unintended_reverted", "note": "reverted"},
		}
		hoUnrevPath := writeJSONFile(t, tmp, "unrev.json", hoUnrev)
		if rc, _, errB := runPacketCLI("validate-handoff", hoUnrevPath, "--packet", pktPath, "--screener-report", reportPath); rc != exitInvalid || !strings.Contains(errB, "UNREVERTED_UNINTENDED_CANDIDATE") {
			t.Fatalf("unrev rc=%d err=%s", rc, errB)
		}
		if rc, _, errB := runPacketCLI("validate-handoff", hoUnrevPath, "--packet", pktPath, "--screener-report", emptyReport); rc != exitOK {
			t.Fatalf("reverted rc=%d err=%s", rc, errB)
		}

		// Non-existent / prod / helper tests rejected; real tests across Go/Py/Rust/TS accepted.
		checkTestName := func(tname string, wantOK bool) {
			t.Helper()
			ho := baseHandoffMap()
			ho["runtime_behavior_impact"].(map[string]any)["has_change"] = true
			ho["runtime_behavior_impact"].(map[string]any)["candidate_responses"] = []any{
				map[string]any{"candidate_id": "cand-001", "disposition": "intended", "test_name": tname},
			}
			rc, _, errB := runPacketCLI("validate-handoff", writeJSONFile(t, tmp, "tn_"+shaHex(tname)[:8]+".json", ho), "--packet", pktPath, "--screener-report", reportPath, "--repo-root", repo)
			if wantOK && rc != exitOK {
				t.Fatalf("test_name %q should pass, rc=%d err=%s", tname, rc, errB)
			}
			if !wantOK && (rc != exitInvalid || !strings.Contains(errB, "NONEXISTENT_CANDIDATE_TEST")) {
				t.Fatalf("test_name %q should fail NONEXISTENT_CANDIDATE_TEST, rc=%d err=%s", tname, rc, errB)
			}
		}
		for _, bad := range []string{"TestFake_999", "helper_not_a_test", "TestSuite/FakeSubCase", "setupServer", "_helper"} {
			checkTestName(bad, false)
		}
		for _, good := range []string{"TestSuite", "TestSuite/SubCase", "TestSuiteMethod", "test_top", "tests/test_x.py::TestCls::test_bar", "test_bar[case-1]", "pkg::tests::test_async_rust", "parses_valid_url", "should render widget", "Widget"} {
			checkTestName(good, true)
		}
	})
}

func TestTemplatesMeasureAndGeneratedProtectionParallel(t *testing.T) {
	t.Parallel()

	t.Run("render-template and measure JSON, Markdown, flow YAML, fenced JSON", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		repo := initGitRepo(t)
		writeStubRunSh(t, repo, 0, "")
		runGitCmd(t, repo, "add", ".")
		runGitCmd(t, repo, "commit", "-q", "-m", "stub")

		rc, out, errB := runPacketCLI("render-template", "--kind", "packet", "--packet-id", "RENDER-42")
		if rc != exitOK || !strings.Contains(out, "RENDER-42") || strings.Contains(out, "measured:") {
			t.Fatalf("render-template rc=%d out=%s err=%s", rc, out, errB)
		}
		renderedPkt := filepath.Join(tmp, "rendered.md")
		_ = os.WriteFile(renderedPkt, []byte(out), fsperm.File)
		if rc, _, errB := runPacketCLI("validate-packet", renderedPkt); rc != exitInvalid || !strings.Contains(errB, "UNMEASURED_VERIFICATION_COMMAND") {
			t.Fatalf("unmeasured rendered should fail, rc=%d err=%s", rc, errB)
		}
		if rc, _, errB := runPacketCLI("measure", renderedPkt, "--repo-root", repo); rc != exitOK {
			t.Fatalf("measure md rc=%d err=%s", rc, errB)
		}
		if rc, _, errB := runPacketCLI("validate-packet", renderedPkt); rc != exitOK {
			t.Fatalf("validate after measure rc=%d err=%s", rc, errB)
		}

		// Flow-mapping YAML measure + fenced JSON with \d backslash.
		flowMD := filepath.Join(tmp, "flow.md")
		_ = os.WriteFile(flowMD, []byte("---\npacket_version: \"1.0\"\npacket_id: \"FLOW-001\"\nscope: { target: \"scripts\", allowed_paths: [\"scripts/**\"], max_files: 5, max_functions: 3, allowed_rules: [\"S101\"], stop_condition: \"done\" }\nexecution_policy: { policy: \"fix-existing\", foreground_only: true, forbid_repo_wide_test_build: true, forbid_bulk_autofix: true, verification_command: \"./scripts/lint/run.sh scripts\" }\nhandoff_requirements: { require_modified_files_and_lines: true, require_runtime_behavior_impact: true, require_change_rationale: true }\n---\n\n## 1. 範囲と作業量制約\n\n## 2. 実行・検証手順\n\n## 3. Hand-off 記述必須項目\n"), fsperm.File)
		if rc, _, errB := runPacketCLI("measure", flowMD, "--repo-root", repo); rc != exitOK {
			t.Fatalf("flow measure rc=%d err=%s", rc, errB)
		}
		if rc, _, errB := runPacketCLI("validate-packet", flowMD); rc != exitOK {
			t.Fatalf("flow validate rc=%d err=%s", rc, errB)
		}

		// Missing file & bad packet-id -> exit 2.
		if rc, _, _ := runPacketCLI("validate-packet", filepath.Join(tmp, "missing.json")); rc != exitUsage {
			t.Fatalf("missing file rc=%d want 2", rc)
		}
		if rc, _, _ := runPacketCLI("render-template", "--kind", "packet", "--packet-id", "bad id!"); rc != exitUsage {
			t.Fatalf("bad packet-id rc=%d want 2", rc)
		}
	})

	t.Run("CHECK-05 list-generated-forbidden-paths and header deletion and CWE-88", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		repo := initGitRepo(t)
		_ = os.MkdirAll(filepath.Join(repo, "pkg"), fsperm.Dir)
		_ = os.WriteFile(filepath.Join(repo, "pkg", "real.pb.go"), []byte("// Code generated by protoc-gen-go. DO NOT EDIT.\npackage pkg\n"), fsperm.File)
		_ = os.WriteFile(filepath.Join(repo, "pkg", "lower.gen.go"), []byte("// code generated by custom-gen. do not edit.\npackage pkg\n"), fsperm.File)
		_ = os.MkdirAll(filepath.Join(repo, "stopped-using-k8s"), fsperm.Dir)
		_ = os.WriteFile(filepath.Join(repo, "stopped-using-k8s", "old.pb.go"), []byte("// Code generated by protoc-gen-go. DO NOT EDIT.\npackage old\n"), fsperm.File)
		_ = os.MkdirAll(filepath.Join(repo, "scripts", "harness", "corpus", "C-16"), fsperm.Dir)
		_ = os.WriteFile(filepath.Join(repo, "scripts", "harness", "corpus", "C-16", "before.go"), []byte("// Code generated by oapi-codegen. DO NOT EDIT.\npackage c16\n"), fsperm.File)
		runGitCmd(t, repo, "add", ".")
		runGitCmd(t, repo, "commit", "-q", "-m", "gen")

		rc, out, errB := runPacketCLI("list-generated-forbidden-paths", "--repo-root", repo, "--format", "json")
		var listed []string
		_ = json.Unmarshal([]byte(out), &listed)
		if rc != exitOK || len(listed) != 2 || listed[0] != "pkg/lower.gen.go" || listed[1] != "pkg/real.pb.go" {
			t.Fatalf("listed rc=%d out=%s err=%s", rc, out, errB)
		}

		// Delete header on disk: validate-handoff MUST still catch via HEAD.
		_ = os.WriteFile(filepath.Join(repo, "pkg", "real.pb.go"), []byte("package pkg\nfunc HandEdited() {}\n"), fsperm.File)
		pkt := basePacketMap()
		pkt["scope"].(map[string]any)["allowed_paths"] = []any{"pkg/**"}
		pktPath := writeJSONFile(t, tmp, "p.json", pkt)
		ho := baseHandoffMap()
		ho["modified_files"] = []any{map[string]any{"path": "pkg/real.pb.go", "added_lines": 1, "deleted_lines": 1}}
		hoPath := writeJSONFile(t, tmp, "h.json", ho)
		if rc, _, errB := runPacketCLI("validate-handoff", hoPath, "--packet", pktPath, "--repo-root", repo); rc != exitInvalid || !strings.Contains(errB, "GENERATED_CODE_MODIFIED") {
			t.Fatalf("deleted header rc=%d err=%s", rc, errB)
		}

		// CWE-88: leading '-' on --base-commit exits 2.
		if rc, _, _ := runPacketCLI("validate-handoff", hoPath, "--packet", pktPath, "--repo-root", repo, "--base-commit=--output=/tmp/evil"); rc != exitUsage {
			t.Fatalf("CWE-88 rc=%d want 2", rc)
		}
	})
}

func TestVerifyHandoffAndSequencePortedParallel(t *testing.T) {
	t.Parallel()

	t.Run("LOOP-01 verify-handoff core, rerun, evidence, comments, test expectations, git mv, staged", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		repo := initGitRepo(t)
		runGitCmd(t, repo, "checkout", "-q", "-b", "feat/demo")
		writeStubRunSh(t, repo, 0, "")
		_ = os.MkdirAll(filepath.Join(repo, "pkg"), fsperm.Dir)
		_ = os.MkdirAll(filepath.Join(repo, "tests"), fsperm.Dir)
		_ = os.WriteFile(filepath.Join(repo, "pkg", "worker.py"), []byte("def alpha(x: int) -> int:\n    return x + 1\n\ndef beta(y: int) -> int:\n    return y * 2\n"), fsperm.File)
		_ = os.WriteFile(filepath.Join(repo, "tests", "test_worker.py"), []byte("from pkg.worker import alpha\n\ndef test_alpha():\n    assert alpha(1) == 2\n"), fsperm.File)
		runGitCmd(t, repo, "add", ".")
		runGitCmd(t, repo, "commit", "-q", "-m", "base")
		base := gitHead(t, repo)

		_ = os.WriteFile(filepath.Join(repo, "pkg", "worker.py"), []byte("def alpha(x: int) -> int:\n    result = x + 1\n    return result\n\ndef beta(y: int) -> int:\n    return y * 2\n"), fsperm.File)
		pkt := basePacketMap()
		pkt["packet_id"] = "LOOP01-TEST"
		pkt["task_kind"] = "lint-fix"
		pkt["base_commit"] = base
		pkt["scope"].(map[string]any)["allowed_paths"] = []any{"pkg/**", "tests/**"}
		pkt["scope"].(map[string]any)["forbidden_paths"] = []any{"pkg/secret.py"}
		pktPath := writeJSONFile(t, tmp, "pkt.json", pkt)

		ho := baseHandoffMap()
		ho["packet_id"] = "LOOP01-TEST"
		ho["modified_files"] = []any{map[string]any{"path": "pkg/worker.py", "added_lines": 2, "deleted_lines": 1}}
		ho["modified_functions_count"] = 1
		hoPath := writeJSONFile(t, tmp, "ho.json", ho)
		runsDir := filepath.Join(tmp, "runs")

		// Clean verify-handoff + evidence under feat-demo/verify/LOOP01-TEST.json.
		if rc, _, errB := runPacketCLI("verify-handoff", hoPath, "--packet", pktPath, "--repo-root", repo, "--runs-dir", runsDir); rc != exitOK {
			t.Fatalf("clean verify-handoff rc=%d err=%s", rc, errB)
		}
		evPath := filepath.Join(runsDir, "feat-demo", "verify", "LOOP01-TEST.json")
		if _, err := os.Stat(evPath); err != nil {
			t.Fatalf("missing evidence file %s: %v", evPath, err)
		}

		// False modified_functions_count -> MODIFIED_FUNCTIONS_MISMATCH.
		hoBadFn := baseHandoffMap()
		hoBadFn["packet_id"] = "LOOP01-TEST"
		hoBadFn["modified_files"] = []any{map[string]any{"path": "pkg/worker.py", "added_lines": 2, "deleted_lines": 1}}
		hoBadFn["modified_functions_count"] = 2
		if rc, _, errB := runPacketCLI("verify-handoff", writeJSONFile(t, tmp, "ho_badfn.json", hoBadFn), "--packet", pktPath, "--repo-root", repo, "--no-rerun"); rc != exitInvalid || !strings.Contains(errB, "MODIFIED_FUNCTIONS_MISMATCH") {
			t.Fatalf("bad fn rc=%d err=%s", rc, errB)
		}

		// Phantom file -> PHANTOM_MODIFIED_FILE.
		hoPhantom := baseHandoffMap()
		hoPhantom["packet_id"] = "LOOP01-TEST"
		hoPhantom["modified_files"] = []any{
			map[string]any{"path": "pkg/worker.py", "added_lines": 2, "deleted_lines": 1},
			map[string]any{"path": "pkg/ghost.py", "added_lines": 1, "deleted_lines": 0},
		}
		hoPhantom["modified_functions_count"] = 1
		if rc, _, errB := runPacketCLI("verify-handoff", writeJSONFile(t, tmp, "ho_ph.json", hoPhantom), "--packet", pktPath, "--repo-root", repo, "--no-rerun"); rc != exitInvalid || !strings.Contains(errB, "PHANTOM_MODIFIED_FILE") {
			t.Fatalf("phantom rc=%d err=%s", rc, errB)
		}

		// Unrequested comment -> UNREQUESTED_COMMENT_ADDED; inline bare ADR excuse also rejected.
		_ = os.WriteFile(filepath.Join(repo, "pkg", "worker.py"), []byte("# unrequested\ndef alpha(x: int) -> int:\n    return x + 1\n\ndef beta(y: int) -> int:\n    return y * 2\n"), fsperm.File)
		hoZero := baseHandoffMap()
		hoZero["packet_id"] = "LOOP01-TEST"
		hoZero["modified_files"] = []any{map[string]any{"path": "pkg/worker.py", "added_lines": 1, "deleted_lines": 0}}
		hoZero["modified_functions_count"] = 0
		hoZeroPath := writeJSONFile(t, tmp, "ho_zero.json", hoZero)
		if rc, _, errB := runPacketCLI("verify-handoff", hoZeroPath, "--packet", pktPath, "--repo-root", repo, "--no-rerun"); rc != exitInvalid || !strings.Contains(errB, "UNREQUESTED_COMMENT_ADDED") {
			t.Fatalf("comment rc=%d err=%s", rc, errB)
		}
		_ = os.WriteFile(filepath.Join(repo, "pkg", "worker.py"), []byte("def alpha(x: int) -> int:\n    return x + 1  # excuse ADR-000001\n\ndef beta(y: int) -> int:\n    return y * 2\n"), fsperm.File)
		if rc, _, errB := runPacketCLI("verify-handoff", hoZeroPath, "--packet", pktPath, "--repo-root", repo, "--no-rerun"); rc != exitInvalid || !strings.Contains(errB, "UNREQUESTED_COMMENT_ADDED") {
			t.Fatalf("inline excuse rc=%d err=%s", rc, errB)
		}

		// Test expectation change -> FORBIDDEN_TEST_EXPECTATION_CHANGE, allowed with opt-in.
		runGitCmd(t, repo, "checkout", "--", "pkg/worker.py")
		_ = os.WriteFile(filepath.Join(repo, "tests", "test_worker.py"), []byte("from pkg.worker import alpha\n\ndef test_alpha():\n    assert alpha(1) == 999\n"), fsperm.File)
		hoTC := baseHandoffMap()
		hoTC["packet_id"] = "LOOP01-TEST"
		hoTC["modified_files"] = []any{map[string]any{"path": "tests/test_worker.py", "added_lines": 1, "deleted_lines": 1}}
		hoTC["modified_functions_count"] = 1
		hoTC["runtime_behavior_impact"].(map[string]any)["has_change"] = true
		hoTC["runtime_behavior_impact"].(map[string]any)["candidate_responses"] = []any{
			map[string]any{"file": "tests/test_worker.py", "function": "<test>", "kind": "test.expectation_change", "disposition": "intended", "test_name": "test_alpha"},
		}
		hoTCPath := writeJSONFile(t, tmp, "ho_tc.json", hoTC)
		if rc, _, errB := runPacketCLI("verify-handoff", hoTCPath, "--packet", pktPath, "--repo-root", repo, "--no-rerun"); rc != exitInvalid || !strings.Contains(errB, "FORBIDDEN_TEST_EXPECTATION_CHANGE") {
			t.Fatalf("test expect rc=%d err=%s", rc, errB)
		}
		pkt["scope"].(map[string]any)["allow_test_expectation_changes"] = true
		pktAllowTC := writeJSONFile(t, tmp, "pkt_allow_tc.json", pkt)
		if rc, _, errB := runPacketCLI("verify-handoff", hoTCPath, "--packet", pktAllowTC, "--repo-root", repo, "--no-rerun"); rc != exitOK {
			t.Fatalf("opt-in test expect rc=%d err=%s", rc, errB)
		}
	})

	t.Run("VERIFY-01 sequence structural, bugfix, clean worktree, and symlink safety", func(t *testing.T) {
		t.Parallel()
		// 1. <2 commits -> SEQUENCE_TOO_FEW_COMMITS.
		repoFew := initGitRepo(t)
		_ = os.WriteFile(filepath.Join(repoFew, "app.sh"), []byte("echo 1\n"), fsperm.File)
		runGitCmd(t, repoFew, "add", ".")
		runGitCmd(t, repoFew, "commit", "-q", "-m", "base")
		baseFew := gitHead(t, repoFew)
		_ = os.WriteFile(filepath.Join(repoFew, "test_app.sh"), []byte("test \"$(sh app.sh)\" = 1\n"), fsperm.File)
		runGitCmd(t, repoFew, "add", ".")
		runGitCmd(t, repoFew, "commit", "-q", "-m", "c1 only")
		if rc, _, errB := runPacketCLI("verify-sequence", "--base-commit", baseFew, "--test-command", "sh test_app.sh", "--repo-root", repoFew); rc != exitInvalid || !strings.Contains(errB, "SEQUENCE_TOO_FEW_COMMITS") {
			t.Fatalf("few commits rc=%d err=%s", rc, errB)
		}

		// 2. Bugfix RED->GREEN passes with --bugfix, fails without --bugfix (SEQUENCE_BASELINE_TEST_FAILED).
		repoBF := initGitRepo(t)
		_ = os.WriteFile(filepath.Join(repoBF, "app.sh"), []byte("echo 1\n"), fsperm.File)
		runGitCmd(t, repoBF, "add", ".")
		runGitCmd(t, repoBF, "commit", "-q", "-m", "base")
		baseBF := gitHead(t, repoBF)
		_ = os.WriteFile(filepath.Join(repoBF, "test_app.sh"), []byte("test \"$(sh app.sh)\" = 2\n"), fsperm.File)
		runGitCmd(t, repoBF, "add", ".")
		runGitCmd(t, repoBF, "commit", "-q", "-m", "c1 red test")
		_ = os.WriteFile(filepath.Join(repoBF, "app.sh"), []byte("echo 2\n"), fsperm.File)
		runGitCmd(t, repoBF, "add", ".")
		runGitCmd(t, repoBF, "commit", "-q", "-m", "c2 fix")
		if rc, _, errB := runPacketCLI("verify-sequence", "--base-commit", baseBF, "--test-command", "sh test_app.sh", "--repo-root", repoBF); rc != exitInvalid || !strings.Contains(errB, "SEQUENCE_BASELINE_TEST_FAILED") {
			t.Fatalf("base fail rc=%d err=%s", rc, errB)
		}
		if rc, _, errB := runPacketCLI("verify-sequence", "--base-commit", baseBF, "--test-command", "sh test_app.sh", "--bugfix", "--repo-root", repoBF); rc != exitOK {
			t.Fatalf("bugfix ok rc=%d err=%s", rc, errB)
		}

		// 3. Clean behavior-preserving sequence + --bugfix rejects already-green + dirty worktree rejected + no leaked worktrees.
		repoOK := initGitRepo(t)
		_ = os.WriteFile(filepath.Join(repoOK, "app.sh"), []byte("echo 1\n"), fsperm.File)
		_ = os.MkdirAll(filepath.Join(repoOK, "src"), fsperm.Dir)
		_ = os.MkdirAll(filepath.Join(repoOK, "tests"), fsperm.Dir)
		_ = os.WriteFile(filepath.Join(repoOK, "src", "prod.txt"), []byte("ORIGINAL_PROD\n"), fsperm.File)
		_ = os.Symlink("../src/prod.txt", filepath.Join(repoOK, "tests", "test_alias.sh"))
		runGitCmd(t, repoOK, "add", ".")
		runGitCmd(t, repoOK, "commit", "-q", "-m", "base")
		baseOK := gitHead(t, repoOK)
		_ = os.Remove(filepath.Join(repoOK, "tests", "test_alias.sh"))
		_ = os.WriteFile(filepath.Join(repoOK, "tests", "test_alias.sh"), []byte("grep -q ORIGINAL_PROD src/prod.txt && test \"$(sh app.sh)\" = 1\n"), fsperm.File)
		runGitCmd(t, repoOK, "add", "tests/test_alias.sh")
		runGitCmd(t, repoOK, "commit", "-q", "-m", "c1 test")
		_ = os.WriteFile(filepath.Join(repoOK, "app.sh"), []byte("v=1\necho \"$v\"\n"), fsperm.File)
		runGitCmd(t, repoOK, "add", "app.sh")
		runGitCmd(t, repoOK, "commit", "-q", "-m", "c2 refactor")

		if rc, _, errB := runPacketCLI("verify-sequence", "--base-commit", baseOK, "--test-command", "sh tests/test_alias.sh", "--bugfix", "--repo-root", repoOK); rc != exitInvalid || !strings.Contains(errB, "SEQUENCE_BUGFIX_NOT_RED") {
			t.Fatalf("bugfix not red rc=%d err=%s", rc, errB)
		}
		if rc, _, errB := runPacketCLI("verify-sequence", "--base-commit", baseOK, "--test-command", "sh tests/test_alias.sh", "--repo-root", repoOK); rc != exitOK {
			t.Fatalf("clean sequence rc=%d err=%s", rc, errB)
		}
		wtList := runGitCmd(t, repoOK, "worktree", "list", "--porcelain")
		if strings.Count(wtList, "worktree ") != 1 {
			t.Fatalf("leaked worktrees: %s", wtList)
		}

		// Dirty worktree rejected.
		_ = os.WriteFile(filepath.Join(repoOK, "app.sh"), []byte("echo 999\n"), fsperm.File)
		if rc, _, errB := runPacketCLI("verify-sequence", "--base-commit", baseOK, "--test-command", "sh tests/test_alias.sh", "--repo-root", repoOK); rc != exitInvalid || !strings.Contains(errB, "SEQUENCE_DIRTY_WORKTREE") {
			t.Fatalf("dirty worktree rc=%d err=%s", rc, errB)
		}
	})
}
