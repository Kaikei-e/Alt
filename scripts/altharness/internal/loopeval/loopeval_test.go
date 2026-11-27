package loopeval

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"alt/scripts/altharness/internal/fsperm"
	"alt/scripts/altharness/internal/guards"
)

func testRepoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "scripts", "harness", "eval", "scenarios")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate repo root from %s", wd)
		}
		dir = parent
	}
}

func writeFilePerm(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), fsperm.Dir); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func makeCleanHandoff(packetID, target string) string {
	return `---
packet_version: "1.0"
packet_id: "` + packetID + `"
status: "completed"
modified_files: []
modified_functions_count: 0
runtime_behavior_impact:
  has_change: false
  checked_dimensions:
    - "control_flow"
    - "default_value"
    - "error_message"
  summary: "変更なし（現行実装の動作とテスト整合を確認済み）"
rationale: "変更なし"
verification:
  command: "./scripts/lint/run.sh ` + target + `"
  exit_code: 0
---

# Hand-off 報告: ` + packetID + `

## 1. 修正ファイルと行数

- 変更ファイルなし（0 件）

## 2. 実行時挙動の変更可能性の有無

- 変更なし

## 3. 変更理由

- 変更なし
`
}

func makeModifiedHandoff(packetID, target, modifiedPath string, funcs int) string {
	return `---
packet_version: "1.0"
packet_id: "` + packetID + `"
status: "completed"
modified_files:
  - path: "` + modifiedPath + `"
    added_lines: 5
    deleted_lines: 5
modified_functions_count: ` + jsonInt(funcs) + `
runtime_behavior_impact:
  has_change: false
  checked_dimensions:
    - "control_flow"
    - "default_value"
    - "error_message"
  summary: "変更なし（リファクタ後の動作確認済み）"
rationale: "対象ルールの修正を適用した"
verification:
  command: "./scripts/lint/run.sh ` + target + `"
  exit_code: 0
---

# Hand-off 報告: ` + packetID + `

## 1. 修正ファイルと行数

- ` + modifiedPath + `

## 2. 実行時挙動の変更可能性の有無

- 変更なし

## 3. 変更理由

- 対象ルールの修正を適用した
`
}

func jsonInt(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func TestCLIUsageAndHelp(t *testing.T) {
	ctx := context.Background()
	var nilCtx context.Context
	var out, errBuf bytes.Buffer
	if rc := Run(nilCtx, []string{"list"}, &out, &errBuf); rc != 2 {
		t.Fatalf("nil ctx: want 2, got %d", rc)
	}
	out.Reset()
	errBuf.Reset()
	if rc := Run(ctx, nil, &out, &errBuf); rc != 2 {
		t.Fatalf("empty args: want 2, got %d", rc)
	}
	out.Reset()
	errBuf.Reset()
	if rc := Run(ctx, []string{"--help"}, &out, &errBuf); rc != 0 {
		t.Fatalf("--help: want 0, got %d (%s)", rc, errBuf.String())
	}
	if !strings.Contains(out.String(), "validate") || !strings.Contains(out.String(), "compare") {
		t.Fatalf("help missing subcommands: %s", out.String())
	}
	out.Reset()
	errBuf.Reset()
	if rc := Run(ctx, []string{"unknown-subcmd"}, &out, &errBuf); rc != 2 {
		t.Fatalf("unknown subcmd: want 2, got %d", rc)
	}
}

func TestListRealScenarios(t *testing.T) {
	ctx := context.Background()
	repo := testRepoRoot(t)

	var out, errBuf bytes.Buffer
	rc := Run(ctx, []string{"list", "--repo-root", repo, "--format", "json"}, &out, &errBuf)
	if rc != 0 {
		t.Fatalf("list json failed (rc=%d): %s", rc, errBuf.String())
	}
	var res struct {
		Count     int        `json:"count"`
		Scenarios []Scenario `json:"scenarios"`
	}
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal list json: %v\n%s", err, out.String())
	}
	if res.Count != 7 || len(res.Scenarios) != 7 {
		t.Fatalf("want 7 scenarios, got count=%d len=%d", res.Count, len(res.Scenarios))
	}
	wantIDs := []string{"E-01", "E-02", "E-03", "E-04", "E-05", "E-06", "E-07"}
	for i, want := range wantIDs {
		if res.Scenarios[i].ID != want {
			t.Fatalf("scenario[%d].ID = %q, want %q", i, res.Scenarios[i].ID, want)
		}
	}

	out.Reset()
	errBuf.Reset()
	rc = Run(ctx, []string{"list", "--repo-root", repo, "--format", "text"}, &out, &errBuf)
	if rc != 0 {
		t.Fatalf("list text failed (rc=%d): %s", rc, errBuf.String())
	}
	for _, id := range wantIDs {
		if !strings.Contains(out.String(), id) {
			t.Fatalf("list text missing %s: %s", id, out.String())
		}
	}
}

func TestValidateRealScenarios(t *testing.T) {
	ctx := context.Background()
	repo := testRepoRoot(t)

	var out, errBuf bytes.Buffer
	rc := Run(ctx, []string{"validate", "--repo-root", repo, "--format", "json"}, &out, &errBuf)
	if rc != 0 {
		t.Fatalf("validate real repo failed (rc=%d): stdout=%s stderr=%s", rc, out.String(), errBuf.String())
	}
	var res ValidationReport
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal validate json: %v", err)
	}
	if !res.Valid || res.ScenarioCount != 7 || len(res.Errors) != 0 {
		t.Fatalf("unexpected validate report: %+v", res)
	}

	out.Reset()
	errBuf.Reset()
	rc = Run(ctx, []string{"validate", "--repo-root", repo, "--min-scenarios", "10", "--format", "json"}, &out, &errBuf)
	if rc != 1 {
		t.Fatalf("validate min-scenarios=10: want 1, got %d", rc)
	}
	if !strings.Contains(out.String(), "INSUFFICIENT_SCENARIOS") {
		t.Fatalf("expected INSUFFICIENT_SCENARIOS, got: %s", out.String())
	}
}

func TestValidateRejectsObserverEffectAndBadFixtures(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()

	// Seed a minimal valid corpus case C-01.
	writeFilePerm(t, filepath.Join(tmp, "scripts/harness/corpus/C-01/meta.toml"), `
id = "C-01"
title = "Sample"
category = "autofix-logic-corruption"
language = "go"
before_file = "before.go"
after_file = "after.go"
`, fsperm.File)
	writeFilePerm(t, filepath.Join(tmp, "scripts/harness/corpus/C-01/before.go"), "package p\n", fsperm.File)
	writeFilePerm(t, filepath.Join(tmp, "scripts/harness/corpus/C-01/after.go"), "package p\n", fsperm.File)

	// Scenario E-01: forbidden workspace name ("eval-box") and observer leak in packet.md.
	writeFilePerm(t, filepath.Join(tmp, "scripts/harness/eval/scenarios/E-01/scenario.toml"), `
id = "E-01"
corpus_id = "C-01"
title = "Bad neutrality"
language = "go"
base_commit = "6cae3ef6f"
neutral_workspace_name = "eval-box"
target_file = "pkg/a.go"
check01_expected_signal = ""
has_char_test = false
char_test_command = ""
required_reads = ["pkg/a.go"]
`, fsperm.File)
	writeFilePerm(t, filepath.Join(tmp, "scripts/harness/eval/scenarios/E-01/packet.md"), `---
packet_version: "1.0"
packet_id: "PKT-BAD-001"
scope:
  target: "svc-a"
  allowed_paths: ["pkg/**"]
  max_files: 3
  max_functions: 3
  allowed_rules: ["errorlint"]
  stop_condition: "done"
execution_policy:
  policy: "fix-existing"
  foreground_only: true
  forbid_repo_wide_test_build: true
  forbid_bulk_autofix: true
  verification_command: "./scripts/lint/run.sh svc-a"
  measured:
    duration_s: 1.0
    exit_code: 0
    at_commit: "6cae3ef6f"
    command_sha256: "242b3c2882f9d3cc9a54494cc5a973e8f192d5f7842897151db5fd635b32e706"
handoff_requirements:
  require_modified_files_and_lines: true
  require_runtime_behavior_impact: true
  require_change_rationale: true
---
## 1. 範囲と作業量制約
This is an eval using an evaluation rubric and blinded judge.
## 2. 実行・検証手順
`+"```bash\n./scripts/lint/run.sh svc-a\n```"+`
## 3. Hand-off 記述必須項目
`, fsperm.File)

	// Scenario E-02 & E-03: duplicate neutral_workspace_name and missing corpus_id.
	writeFilePerm(t, filepath.Join(tmp, "scripts/harness/eval/scenarios/E-02/scenario.toml"), `
id = "E-02"
corpus_id = "C-01"
title = "Dup one"
language = "go"
base_commit = "6cae3ef6f"
neutral_workspace_name = "shared-svc"
target_file = "pkg/b.go"
has_char_test = false
required_reads = ["pkg/b.go"]
`, fsperm.File)
	writeFilePerm(t, filepath.Join(tmp, "scripts/harness/eval/scenarios/E-02/packet.md"), "not a valid packet", fsperm.File)

	writeFilePerm(t, filepath.Join(tmp, "scripts/harness/eval/scenarios/E-03/scenario.toml"), `
id = "E-03"
corpus_id = "C-99"
title = "Dup two and missing corpus"
language = "go"
base_commit = "zzzz"
neutral_workspace_name = "shared-svc"
target_file = "../escape.go"
has_char_test = false
required_reads = []
`, fsperm.File)
	writeFilePerm(t, filepath.Join(tmp, "scripts/harness/eval/scenarios/E-03/packet.md"), "not a valid packet", fsperm.File)

	var out, errBuf bytes.Buffer
	rc := Run(ctx, []string{"validate", "--repo-root", tmp, "--min-scenarios", "1", "--format", "json"}, &out, &errBuf)
	if rc != 1 {
		t.Fatalf("expected validate failure (rc=1), got %d: %s", rc, out.String())
	}
	var rep ValidationReport
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	codes := map[string]bool{}
	for _, issue := range rep.Errors {
		codes[issue.Code] = true
	}
	for _, want := range []string{
		"NEUTRALITY_VIOLATION",
		"OBSERVER_EFFECT_LEAK",
		"DUPLICATE_NEUTRAL_WORKSPACE_NAME",
		"MISSING_CORPUS_CASE",
		"INVALID_BASE_COMMIT",
		"UNSAFE_TARGET_FILE",
		"EMPTY_REQUIRED_READS",
	} {
		if !codes[want] {
			t.Errorf("missing expected error code %s in %+v", want, rep.Errors)
		}
	}
}

func TestStageAndGradeAllSevenScenariosBeforeAndAfter(t *testing.T) {
	ctx := context.Background()
	repo := testRepoRoot(t)

	cases := []struct {
		id         string
		corpusID   string
		afterFile  string
		packetID   string
		neutralDir string
		targetFile string
		expectC01  bool
		modFuncs   int
	}{
		{"E-01", "C-01", "after.go", "PKT-MW-001", "svc-middleware", "pkg/middleware/errors.go", true, 1},
		{"E-02", "C-06", "after.py", "PKT-QUEUE-002", "worker-queue", "src/queue.py", true, 1},
		{"E-03", "C-07", "after.py", "PKT-STREAM-003", "stream-handler", "src/stream.py", true, 1},
		{"E-04", "C-11", "after.go", "PKT-SERVER-004", "edge-server", "cmd/server/main.go", true, 1},
		{"E-05", "C-15", "after.go", "PKT-API-005", "api-bootstrap", "internal/bootstrap/server.go", true, 1},
		{"E-06", "C-16", "after.go", "PKT-SCHEMA-006", "schema-adapter", "internal/openapi/server.gen.go", false, 1},
		{"E-07", "C-18", "after.sh", "PKT-STACK-007", "stack-check", "scripts/check-compose-variables.sh", false, 0},
	}

	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			outDir := t.TempDir()
			var stageOut, stageErr bytes.Buffer
			rc := Run(ctx, []string{"stage", tc.id, "--out-dir", outDir, "--repo-root", repo, "--format", "json"}, &stageOut, &stageErr)
			if rc != 0 {
				t.Fatalf("stage %s failed (rc=%d): %s", tc.id, rc, stageErr.String())
			}
			var staged StageResult
			if err := json.Unmarshal(stageOut.Bytes(), &staged); err != nil {
				t.Fatalf("unmarshal stage: %v", err)
			}
			if staged.NeutralWorkspaceName != tc.neutralDir {
				t.Fatalf("neutral_workspace_name=%q, want %q", staged.NeutralWorkspaceName, tc.neutralDir)
			}
			if staged.BaseCommit == "" {
				t.Fatalf("empty base_commit")
			}
			// Verify zero corpus/rubric metadata leaked into staged workspace.
			for _, forbidden := range []string{"meta.toml", "scenario.toml", "rubric.md"} {
				if _, err := os.Stat(filepath.Join(staged.WorkspaceDir, forbidden)); err == nil {
					t.Fatalf("forbidden file %s found in staged workspace", forbidden)
				}
			}

			// 1) Grade clean before.* workspace with valid read transcript -> PASS (4/4).
			auxDir := t.TempDir()
			cleanHandoff := filepath.Join(auxDir, "handoff-clean.md")
			writeFilePerm(t, cleanHandoff, makeCleanHandoff(tc.packetID, tc.neutralDir), fsperm.File)

			transcriptPath := filepath.Join(auxDir, "transcript.jsonl")
			absTarget := filepath.Join(staged.WorkspaceDir, filepath.FromSlash(tc.targetFile))
			readEvt, _ := json.Marshal(map[string]any{
				"type": "tool_use",
				"name": "Read",
				"input": map[string]any{
					"file_path": absTarget,
				},
			})
			writeFilePerm(t, transcriptPath, string(readEvt)+"\n", fsperm.File)

			var gOut, gErr bytes.Buffer
			rc = Run(ctx, []string{
				"grade",
				"--scenario", tc.id,
				"--workspace", staged.WorkspaceDir,
				"--handoff", cleanHandoff,
				"--transcript", transcriptPath,
				"--base-commit", staged.BaseCommit,
				"--repo-root", repo,
				"--format", "json",
			}, &gOut, &gErr)
			if rc != 0 {
				t.Fatalf("grade clean %s failed (rc=%d): stdout=%s stderr=%s", tc.id, rc, gOut.String(), gErr.String())
			}
			var cleanRes GradeResult
			if err := json.Unmarshal(gOut.Bytes(), &cleanRes); err != nil {
				t.Fatalf("unmarshal clean grade: %v", err)
			}
			if !cleanRes.Passed || cleanRes.Score != 4 || cleanRes.MaxScore != 4 {
				t.Fatalf("clean %s want 4/4 passed, got %+v", tc.id, cleanRes)
			}

			// 2) Apply buggy after.* over target_file and grade -> MUST FAIL.
			afterBytes, err := readFileContainment(filepath.Join(repo, "scripts/harness/corpus", tc.corpusID), tc.afterFile)
			if err != nil {
				t.Fatalf("read after file: %v", err)
			}
			mode := fsperm.File
			if strings.HasSuffix(tc.targetFile, ".sh") {
				mode = fsperm.Dir
			}
			if err := writeFileContainment(staged.WorkspaceDir, tc.targetFile, afterBytes, mode); err != nil {
				t.Fatalf("overwrite target with buggy after: %v", err)
			}
			buggyHandoff := filepath.Join(auxDir, "handoff-buggy.md")
			writeFilePerm(t, buggyHandoff, makeModifiedHandoff(tc.packetID, tc.neutralDir, tc.targetFile, tc.modFuncs), fsperm.File)

			gOut.Reset()
			gErr.Reset()
			rc = Run(ctx, []string{
				"grade",
				"--scenario", tc.id,
				"--workspace", staged.WorkspaceDir,
				"--handoff", buggyHandoff,
				"--transcript", transcriptPath,
				"--base-commit", staged.BaseCommit,
				"--repo-root", repo,
				"--format", "json",
			}, &gOut, &gErr)
			if rc != 1 {
				t.Fatalf("grade buggy %s: want rc=1, got %d (stdout=%s stderr=%s)", tc.id, rc, gOut.String(), gErr.String())
			}
			var buggyRes GradeResult
			if err := json.Unmarshal(gOut.Bytes(), &buggyRes); err != nil {
				t.Fatalf("unmarshal buggy grade: %v", err)
			}
			if buggyRes.Passed {
				t.Fatalf("buggy %s unexpectedly passed: %+v", tc.id, buggyRes)
			}
			if buggyRes.Signals.CharTest.Passed {
				t.Fatalf("buggy %s char_test unexpectedly passed", tc.id)
			}
			if buggyRes.Signals.Loop01.Passed {
				t.Fatalf("buggy %s loop01 unexpectedly passed", tc.id)
			}
			if tc.expectC01 && (buggyRes.Signals.Check01.Passed || !buggyRes.Signals.Check01.ExpectedSignalTriggered) {
				t.Fatalf("buggy %s check01 did not trigger expected signal: %+v", tc.id, buggyRes.Signals.Check01)
			}
		})
	}
}

func TestStageRefusesSymlinkOutDir(t *testing.T) {
	ctx := context.Background()
	repo := testRepoRoot(t)
	realDir := t.TempDir()
	linkParent := t.TempDir()
	sym := filepath.Join(linkParent, "out-link")
	if err := os.Symlink(realDir, sym); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	var out, errBuf bytes.Buffer
	rc := Run(ctx, []string{"stage", "E-01", "--out-dir", sym, "--repo-root", repo}, &out, &errBuf)
	if rc == 0 {
		t.Fatalf("expected symlink --out-dir to be refused, got rc=0")
	}
}

func TestTranscriptReadAuditFormatsAndFabricationDetection(t *testing.T) {
	ctx := context.Background()
	repo := testRepoRoot(t)
	outDir := t.TempDir()

	var stageOut, stageErr bytes.Buffer
	if rc := Run(ctx, []string{"stage", "E-01", "--out-dir", outDir, "--repo-root", repo, "--format", "json"}, &stageOut, &stageErr); rc != 0 {
		t.Fatalf("stage E-01: %s", stageErr.String())
	}
	var staged StageResult
	if err := json.Unmarshal(stageOut.Bytes(), &staged); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	auxDir := t.TempDir()
	handoffPath := filepath.Join(auxDir, "handoff.md")
	writeFilePerm(t, handoffPath, makeCleanHandoff("PKT-MW-001", "svc-middleware"), fsperm.File)
	absTarget := filepath.Join(staged.WorkspaceDir, "pkg/middleware/errors.go")

	formats := []struct {
		name  string
		jsonl string
	}{
		{
			name:  "claude_nested_message",
			jsonl: `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Read","input":{"file_path":` + jsonQuote(absTarget) + `}}]}}` + "\n",
		},
		{
			name:  "antigravity_tool_args",
			jsonl: `{"tool":"read_file","args":{"Path":` + jsonQuote(absTarget) + `}}` + "\n",
		},
		{
			name:  "antigravity_view_file",
			jsonl: `{"name":"view_file","arguments":{"AbsolutePath":"pkg/middleware/errors.go"}}` + "\n",
		},
		{
			name:  "codex_function_call_string_args",
			jsonl: `{"type":"function_call","name":"read_file","arguments":"{\"path\":\"pkg/middleware/errors.go\"}"}` + "\n",
		},
		{
			name:  "generic_tool_name",
			jsonl: `{"tool_name":"Read","file_path":"./pkg/middleware/errors.go"}` + "\n",
		},
	}

	for _, tc := range formats {
		t.Run(tc.name, func(t *testing.T) {
			tp := filepath.Join(auxDir, tc.name+".jsonl")
			writeFilePerm(t, tp, tc.jsonl, fsperm.File)
			var gOut, gErr bytes.Buffer
			rc := Run(ctx, []string{
				"grade",
				"--scenario", "E-01",
				"--workspace", staged.WorkspaceDir,
				"--handoff", handoffPath,
				"--transcript", tp,
				"--base-commit", staged.BaseCommit,
				"--no-rerun",
				"--repo-root", repo,
				"--format", "json",
			}, &gOut, &gErr)
			if rc != 0 {
				t.Fatalf("format %s failed (rc=%d): stdout=%s stderr=%s", tc.name, rc, gOut.String(), gErr.String())
			}
			var res GradeResult
			if err := json.Unmarshal(gOut.Bytes(), &res); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if !res.Signals.TranscriptReadAudit.Passed || res.Signals.TranscriptReadAudit.Fidelity != 1.0 {
				t.Fatalf("format %s unexpected audit: %+v", tc.name, res.Signals.TranscriptReadAudit)
			}
		})
	}

	// Fabricated read claim: transcript only reads an unrelated file.
	fabPath := filepath.Join(auxDir, "fabricated.jsonl")
	writeFilePerm(t, fabPath, `{"type":"tool_use","name":"Read","input":{"file_path":"README.md"}}`+"\n", fsperm.File)
	var gOut, gErr bytes.Buffer
	rc := Run(ctx, []string{
		"grade",
		"--scenario", "E-01",
		"--workspace", staged.WorkspaceDir,
		"--handoff", handoffPath,
		"--transcript", fabPath,
		"--base-commit", staged.BaseCommit,
		"--no-rerun",
		"--repo-root", repo,
		"--format", "json",
	}, &gOut, &gErr)
	if rc != 1 {
		t.Fatalf("fabricated read claim: want rc=1, got %d (%s)", rc, gOut.String())
	}
	var fabRes GradeResult
	if err := json.Unmarshal(gOut.Bytes(), &fabRes); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if fabRes.Passed || fabRes.Signals.TranscriptReadAudit.Passed {
		t.Fatalf("fabricated read claim should fail: %+v", fabRes)
	}
	if len(fabRes.Signals.TranscriptReadAudit.Errors) == 0 || fabRes.Signals.TranscriptReadAudit.Errors[0].Code != "UNVERIFIED_FILE_READ_CLAIM" {
		t.Fatalf("expected UNVERIFIED_FILE_READ_CLAIM, got %+v", fabRes.Signals.TranscriptReadAudit.Errors)
	}

	// Missing transcript with --require-transcript must fail closed.
	gOut.Reset()
	gErr.Reset()
	rc = Run(ctx, []string{
		"grade",
		"--scenario", "E-01",
		"--workspace", staged.WorkspaceDir,
		"--handoff", handoffPath,
		"--base-commit", staged.BaseCommit,
		"--no-rerun",
		"--require-transcript",
		"--repo-root", repo,
		"--format", "json",
	}, &gOut, &gErr)
	if rc != 1 {
		t.Fatalf("--require-transcript without transcript: want rc=1, got %d", rc)
	}
	if !strings.Contains(gOut.String(), "MISSING_TRANSCRIPT") {
		t.Fatalf("expected MISSING_TRANSCRIPT, got %s", gOut.String())
	}

	// Missing transcript without --require-transcript scores 3/3.
	gOut.Reset()
	gErr.Reset()
	rc = Run(ctx, []string{
		"grade",
		"--scenario", "E-01",
		"--workspace", staged.WorkspaceDir,
		"--handoff", handoffPath,
		"--base-commit", staged.BaseCommit,
		"--no-rerun",
		"--repo-root", repo,
		"--format", "json",
	}, &gOut, &gErr)
	if rc != 0 {
		t.Fatalf("no transcript optional: want rc=0, got %d (%s)", rc, gOut.String())
	}
	var noTr GradeResult
	if err := json.Unmarshal(gOut.Bytes(), &noTr); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !noTr.Passed || noTr.Score != 3 || noTr.MaxScore != 3 || noTr.Signals.TranscriptReadAudit.Evaluated {
		t.Fatalf("unexpected optional-transcript result: %+v", noTr)
	}
}

func jsonQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func makeGradeSample(scenarioID string, passed bool, score, maxScore int, fidelity float64) GradeResult {
	return GradeResult{
		ScenarioID: scenarioID,
		Score:      score,
		MaxScore:   maxScore,
		Passed:     passed,
		Signals: GradeSignals{
			TranscriptReadAudit: TranscriptAuditSignal{
				Evaluated: true,
				Passed:    fidelity >= 1.0,
				Fidelity:  fidelity,
			},
		},
	}
}

func TestCompareInsufficientTrialsAndRecording(t *testing.T) {
	ctx := context.Background()
	tmpRepo := t.TempDir()
	// Init minimal .harness/runs dir via guards.
	runsDir := filepath.Join(tmpRepo, ".harness", "runs")
	decisionsPath := filepath.Join(runsDir, "eval-branch", "decisions.tsv")

	// 1) Under-sampled comparison (only 2 trials < min-trials 3) -> INSUFFICIENT_TRIALS, no write.
	underBefore := filepath.Join(tmpRepo, "under-before.json")
	underAfter := filepath.Join(tmpRepo, "under-after.json")
	bRaw, _ := json.Marshal([]GradeResult{
		makeGradeSample("E-01", true, 4, 4, 1.0),
		makeGradeSample("E-01", true, 4, 4, 1.0),
	})
	writeFilePerm(t, underBefore, string(bRaw), fsperm.File)
	writeFilePerm(t, underAfter, string(bRaw), fsperm.File)

	var out, errBuf bytes.Buffer
	rc := Run(ctx, []string{
		"compare",
		"--before", underBefore,
		"--after", underAfter,
		"--record",
		"--branch", "eval-branch",
		"--runs-dir", runsDir,
		"--decisions-file", decisionsPath,
		"--repo-root", tmpRepo,
		"--format", "json",
	}, &out, &errBuf)
	if rc != 1 {
		t.Fatalf("under-sampled compare: want rc=1, got %d (%s)", rc, out.String())
	}
	if !strings.Contains(out.String(), "INSUFFICIENT_TRIALS") {
		t.Fatalf("expected INSUFFICIENT_TRIALS, got %s", out.String())
	}
	if _, err := os.Stat(decisionsPath); err == nil {
		t.Fatalf("decisions.tsv should not be written when trials are insufficient")
	}

	// 2) Valid 3-trial before (directory of *.json) vs after (single JSON array) showing improvement + --record.
	beforeDir := filepath.Join(tmpRepo, "before-trials")
	beforeSamples := []GradeResult{
		makeGradeSample("E-01", false, 2, 4, 0.5),
		makeGradeSample("E-01", true, 4, 4, 1.0),
		makeGradeSample("E-01", false, 2, 4, 0.5),
		makeGradeSample("E-02", true, 4, 4, 1.0),
		makeGradeSample("E-02", true, 4, 4, 1.0),
		makeGradeSample("E-02", true, 4, 4, 1.0),
	}
	for i, s := range beforeSamples {
		raw, _ := json.MarshalIndent(s, "", "  ")
		writeFilePerm(t, filepath.Join(beforeDir, "trial-"+jsonInt(i)+".json"), string(raw), fsperm.File)
	}
	afterFile := filepath.Join(tmpRepo, "after-trials.json")
	afterSamples := []GradeResult{
		makeGradeSample("E-01", true, 4, 4, 1.0),
		makeGradeSample("E-01", true, 4, 4, 1.0),
		makeGradeSample("E-01", true, 4, 4, 1.0),
		makeGradeSample("E-02", true, 4, 4, 1.0),
		makeGradeSample("E-02", true, 4, 4, 1.0),
		makeGradeSample("E-02", true, 4, 4, 1.0),
	}
	aRaw, _ := json.MarshalIndent(afterSamples, "", "  ")
	writeFilePerm(t, afterFile, string(aRaw), fsperm.File)

	out.Reset()
	errBuf.Reset()
	rc = Run(ctx, []string{
		"compare",
		"--before", beforeDir,
		"--after", afterFile,
		"--min-trials", "3",
		"--template-version", "v2",
		"--branch", "eval-branch",
		"--runs-dir", runsDir,
		"--decisions-file", decisionsPath,
		"--record",
		"--repo-root", tmpRepo,
		"--format", "json",
	}, &out, &errBuf)
	if rc != 0 {
		t.Fatalf("improved compare failed (rc=%d): stdout=%s stderr=%s", rc, out.String(), errBuf.String())
	}
	var cmp CompareReport
	if err := json.Unmarshal(out.Bytes(), &cmp); err != nil {
		t.Fatalf("unmarshal compare: %v\n%s", err, out.String())
	}
	if cmp.Verdict != "improved" {
		t.Fatalf("want verdict=improved, got %q (%+v)", cmp.Verdict, cmp)
	}
	if len(cmp.Regressions) != 0 || len(cmp.Improvements) != 1 || cmp.Improvements[0] != "E-01" {
		t.Fatalf("unexpected improvements/regressions: %+v", cmp)
	}
	if cmp.RecordedArtifact == "" {
		t.Fatalf("expected recorded_artifact path")
	}
	if _, err := os.Stat(cmp.RecordedArtifact); err != nil {
		t.Fatalf("recorded artifact missing at %s: %v", cmp.RecordedArtifact, err)
	}
	tsvBytes, err := readRegularFile(decisionsPath)
	if err != nil {
		t.Fatalf("read decisions.tsv: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(tsvBytes)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected header + 1 decision row, got %d lines:\n%s", len(lines), string(tsvBytes))
	}
	cols := strings.Split(lines[1], "\t")
	if len(cols) != 6 {
		t.Fatalf("expected 6 TSV columns, got %d: %q", len(cols), lines[1])
	}
	if cols[1] != "loop-eval" || cols[5] != "info" {
		t.Fatalf("expected phase=loop-eval result=info, got phase=%q result=%q", cols[1], cols[5])
	}
	if !guards.IsValidEvidencePointer(cols[4]) {
		t.Fatalf("evidence column %q is not a valid pointer", cols[4])
	}

	// 3) Regression case: swap before and after -> verdict=regressed, rc=1, still recorded!
	out.Reset()
	errBuf.Reset()
	rc = Run(ctx, []string{
		"compare",
		"--before", afterFile,
		"--after", beforeDir,
		"--min-trials", "3",
		"--template-version", "v3-bad",
		"--branch", "eval-branch",
		"--runs-dir", runsDir,
		"--decisions-file", decisionsPath,
		"--record",
		"--repo-root", tmpRepo,
		"--format", "json",
	}, &out, &errBuf)
	if rc != 1 {
		t.Fatalf("regressed compare: want rc=1, got %d (%s)", rc, out.String())
	}
	var reg CompareReport
	if err := json.Unmarshal(out.Bytes(), &reg); err != nil {
		t.Fatalf("unmarshal regressed: %v", err)
	}
	if reg.Verdict != "regressed" || len(reg.Regressions) != 1 || reg.Regressions[0] != "E-01" {
		t.Fatalf("unexpected regression report: %+v", reg)
	}
	tsvBytes, err = readRegularFile(decisionsPath)
	if err != nil {
		t.Fatalf("read decisions.tsv after regression: %v", err)
	}
	lines = strings.Split(strings.TrimSpace(string(tsvBytes)), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected header + 2 decision rows after recording regression, got %d", len(lines))
	}

	// 4) No-change case: identical before and after -> verdict=no_change, rc=0.
	out.Reset()
	errBuf.Reset()
	rc = Run(ctx, []string{
		"compare",
		"--before", afterFile,
		"--after", afterFile,
		"--min-trials", "3",
		"--repo-root", tmpRepo,
		"--format", "json",
	}, &out, &errBuf)
	if rc != 0 {
		t.Fatalf("no_change compare: want rc=0, got %d (%s)", rc, out.String())
	}
	var nc CompareReport
	if err := json.Unmarshal(out.Bytes(), &nc); err != nil {
		t.Fatalf("unmarshal no_change: %v", err)
	}
	if nc.Verdict != "no_change" || len(nc.Regressions) != 0 || len(nc.Improvements) != 0 {
		t.Fatalf("unexpected no_change report: %+v", nc)
	}
}

func TestGradeAutoResolvesBaseCommitAndRejectsMalformedTranscript(t *testing.T) {
	ctx := context.Background()
	repo := testRepoRoot(t)
	outDir := t.TempDir()

	var stageOut, stageErr bytes.Buffer
	if rc := Run(ctx, []string{"stage", "E-01", "--out-dir", outDir, "--repo-root", repo, "--format", "json"}, &stageOut, &stageErr); rc != 0 {
		t.Fatalf("stage E-01: %s", stageErr.String())
	}
	var staged StageResult
	if err := json.Unmarshal(stageOut.Bytes(), &staged); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	auxDir := t.TempDir()
	handoffPath := filepath.Join(auxDir, "handoff.md")
	writeFilePerm(t, handoffPath, makeCleanHandoff("PKT-MW-001", "svc-middleware"), fsperm.File)

	// Omit --base-commit -> auto-resolves workspace root commit.
	var gOut, gErr bytes.Buffer
	rc := Run(ctx, []string{
		"grade",
		"--scenario", "E-01",
		"--workspace", staged.WorkspaceDir,
		"--handoff", handoffPath,
		"--no-rerun",
		"--repo-root", repo,
		"--format", "json",
	}, &gOut, &gErr)
	if rc != 0 {
		t.Fatalf("grade auto base-commit failed (rc=%d): stdout=%s stderr=%s", rc, gOut.String(), gErr.String())
	}
	var res GradeResult
	if err := json.Unmarshal(gOut.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if res.BaseCommit != staged.BaseCommit {
		t.Fatalf("auto-resolved base_commit=%q, want %q", res.BaseCommit, staged.BaseCommit)
	}

	// Malformed JSONL transcript must fail closed with TRANSCRIPT_PARSE_ERROR.
	badTranscript := filepath.Join(auxDir, "bad.jsonl")
	writeFilePerm(t, badTranscript, "{not-valid-json\n", fsperm.File)
	gOut.Reset()
	gErr.Reset()
	rc = Run(ctx, []string{
		"grade",
		"--scenario", "E-01",
		"--workspace", staged.WorkspaceDir,
		"--handoff", handoffPath,
		"--transcript", badTranscript,
		"--no-rerun",
		"--repo-root", repo,
		"--format", "json",
	}, &gOut, &gErr)
	if rc != 1 {
		t.Fatalf("malformed transcript: want rc=1, got %d (%s)", rc, gOut.String())
	}
	if !strings.Contains(gOut.String(), "TRANSCRIPT_PARSE_ERROR") {
		t.Fatalf("expected TRANSCRIPT_PARSE_ERROR, got %s", gOut.String())
	}
}

func TestCharTestSignalRejectsForbiddenCommands(t *testing.T) {
	ctx := context.Background()
	ws := t.TempDir()

	sigAutofix := runCharTestSignal(ctx, ws, Scenario{
		ID:              "E-99",
		HasCharTest:     true,
		CharTestCommand: "golangci-lint run --fix ./...",
	})
	if sigAutofix.Passed || len(sigAutofix.Errors) == 0 || sigAutofix.Errors[0].Code != "FORBIDDEN_AUTOFIX_COMMAND" {
		t.Fatalf("expected FORBIDDEN_AUTOFIX_COMMAND, got %+v", sigAutofix)
	}

	sigBG := runCharTestSignal(ctx, ws, Scenario{
		ID:              "E-99",
		HasCharTest:     true,
		CharTestCommand: "bash test_characterization.sh &",
	})
	if sigBG.Passed || len(sigBG.Errors) == 0 || sigBG.Errors[0].Code != "FORBIDDEN_BACKGROUND_EXECUTION" {
		t.Fatalf("expected FORBIDDEN_BACKGROUND_EXECUTION, got %+v", sigBG)
	}
}
