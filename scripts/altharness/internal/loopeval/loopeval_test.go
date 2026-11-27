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
base_commit: "6cae3ef6f"
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

const validE01GoFix = `package middleware

import "errors"

type HTTPError struct {
	Code    int
	Message string
}

func (e *HTTPError) Error() string {
	return e.Message
}

func recordServerError(err error, recordFailure func()) {
	if err == nil {
		return
	}
	// Only 5xx server errors should trip the circuit breaker; 4xx must be ignored.
	var he *HTTPError
	if errors.As(err, &he) && he.Code >= 500 {
		recordFailure()
	}
}
`

const validE02PyFix = `"""Minimal reproduction of HybridPrioritySemaphore.acquire return path."""

import asyncio
from collections.abc import Callable


class PrioritySemaphore:
    def __init__(
        self,
        record_queued_acquire: Callable[[str, float, bool], tuple[float, int]],
    ) -> None:
        self._record_queued_acquire = record_queued_acquire

    def _record_slot(
        self,
        result: str,
        start_time: float,
        high_priority: bool,
    ) -> tuple[float, int]:
        return self._record_queued_acquire(result, start_time, high_priority)

    async def acquire(
        self,
        future: asyncio.Future[str],
        start_time: float,
        high_priority: bool,
    ):
        try:
            result = await future
            wait_time, sid = self._record_slot(
                result, start_time, high_priority
            )
        except asyncio.CancelledError:
            if not future.done():
                future.cancel()
            raise
        else:
            return wait_time, sid
`

const validE03PyFix = `"""Minimal reproduction of SSE stream forwarding with contextlib.aclosing."""

import asyncio
from collections.abc import AsyncGenerator
from contextlib import aclosing


async def forward_stream_chunks(
    original_stream: AsyncGenerator[str, None],
    data_queue: asyncio.Queue[tuple[str, str] | None],
) -> None:
    try:
        async with aclosing(original_stream) as stream:
            async for chunk in stream:
                await data_queue.put(("data", chunk))
    except Exception:
        await data_queue.put(("error", "summary generation failed"))
    finally:
        # Always emit terminal sentinel so the SSE response finishes cleanly.
        await data_queue.put(None)
`

const validE04GoFix = `package main

import (
	"net/http"
	"time"

	"alt/httpserver"
)

func newPlaintextServer(addr string, h http.Handler) *http.Server {
	cfg := httpserver.Config{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return httpserver.New(cfg)
}
`

const validE05GoFix = `package bootstrap

import (
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
)

// Matches Echo v4 middleware.BodyLimit("1M") decimal megabyte (1,000,000 bytes).
const (
	maxRequestBodyBytes = 1_000_000
	apiV1Prefix         = "/api/v1"
)

func registerSummarizeGroup(e *echo.Echo, handler echo.HandlerFunc) {
	api := e.Group(apiV1Prefix)
	api.Use(middleware.BodyLimit(maxRequestBodyBytes))
	api.POST("/summarize", handler)
}
`

const validE07ShFix = `#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DOCKER_BIN="${DOCKER_BIN:-docker}"
COMPOSE_FILE="${COMPOSE_FILE:-$REPO_ROOT/compose/compose.yaml}"
COMPOSE_PROJECT="${COMPOSE_PROJECT:-alt}"
ENV_FILE=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --env-file) ENV_FILE="$2"; shift 2 ;;
    *) echo "Unknown arg: $1" >&2; exit 2 ;;
  esac
done

RENDER_ERR="$(mktemp)"
trap 'rm -f "$RENDER_ERR"' EXIT

compose_args=()
if [[ -n "$ENV_FILE" ]]; then
  compose_args+=(--env-file "$ENV_FILE")
fi
compose_args+=(-f "$COMPOSE_FILE" -p "$COMPOSE_PROJECT" config -q)

"$DOCKER_BIN" compose "${compose_args[@]}" 2>"$RENDER_ERR"
if grep -q 'variable is not set' "$RENDER_ERR"; then
  echo "FAIL: unset compose interpolation variable detected" >&2
  exit 1
fi
`

func TestStageAndGradeAllSevenScenariosBeforeAndAfter(t *testing.T) {
	ctx := context.Background()
	repo := testRepoRoot(t)

	cases := []struct {
		id           string
		corpusID     string
		afterFile    string
		packetID     string
		neutralDir   string
		targetFile   string
		expectC01    bool
		requireDiff  bool
		validEdit    string
		validFuncs   int
		buggyModFunc int
	}{
		{"E-01", "C-01", "after.go", "PKT-MW-001", "svc-middleware", "pkg/middleware/errors.go", true, true, validE01GoFix, 1, 1},
		{"E-02", "C-06", "after.py", "PKT-QUEUE-002", "worker-queue", "src/queue.py", true, true, validE02PyFix, 2, 1},
		{"E-03", "C-07", "after.py", "PKT-STREAM-003", "stream-handler", "src/stream.py", true, true, validE03PyFix, 1, 1},
		{"E-04", "C-11", "after.go", "PKT-SERVER-004", "edge-server", "cmd/server/main.go", true, true, validE04GoFix, 1, 1},
		{"E-05", "C-15", "after.go", "PKT-API-005", "api-bootstrap", "internal/bootstrap/server.go", true, true, validE05GoFix, 1, 1},
		{"E-06", "C-16", "after.go", "PKT-SCHEMA-006", "schema-adapter", "internal/openapi/server.gen.go", false, false, "", 0, 1},
		{"E-07", "C-18", "after.sh", "PKT-STACK-007", "stack-check", "scripts/check-compose-variables.sh", false, true, validE07ShFix, 0, 0},
	}

	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			outDir := neutralTempDir(t)
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
			// Verify zero corpus/rubric/oracle files leaked into staged workspace.
			for _, forbidden := range []string{"meta.toml", "scenario.toml", "rubric.md", "test_characterization.sh"} {
				if _, err := os.Stat(filepath.Join(staged.WorkspaceDir, forbidden)); err == nil {
					t.Fatalf("forbidden file %s found in staged workspace", forbidden)
				}
			}
			// Verify zero internal harness label comments leaked into staged target_file or run.sh.
			stagedTargetBytes, err := readFileContainment(staged.WorkspaceDir, tc.targetFile)
			if err != nil {
				t.Fatalf("read staged target: %v", err)
			}
			if harnessLabelCommentRE.Match(stagedTargetBytes) {
				t.Fatalf("staged %s leaked internal harness label:\n%s", tc.targetFile, string(stagedTargetBytes))
			}
			runShBytes, err := readFileContainment(staged.WorkspaceDir, "scripts/lint/run.sh")
			if err != nil {
				t.Fatalf("read staged run.sh: %v", err)
			}
			lowerRunSh := strings.ToLower(string(runShBytes))
			for _, tok := range forbiddenNeutralTokens {
				if strings.Contains(lowerRunSh, tok) {
					t.Fatalf("staged run.sh for %s contains forbidden neutral token %q", tc.id, tok)
				}
			}

			auxDir := neutralTempDir(t)
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

			// 1) Zero-edit no-op check:
			//    - Scenarios with require_non_empty_diff=true MUST FAIL with NOOP_ZERO_EDITS!
			//    - E-06 (DO NOT EDIT, require_non_empty_diff=false) MUST PASS 4/4!
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
			var noopRes GradeResult
			if err := json.Unmarshal(gOut.Bytes(), &noopRes); err != nil {
				t.Fatalf("unmarshal zero-edit grade %s: %v\nstdout=%s", tc.id, err, gOut.String())
			}
			if tc.requireDiff {
				if rc != 1 || noopRes.Passed {
					t.Fatalf("zero-edit %s want FAIL (rc=1), got rc=%d res=%+v", tc.id, rc, noopRes)
				}
				if !strings.Contains(gOut.String(), "NOOP_ZERO_EDITS") {
					t.Fatalf("zero-edit %s missing NOOP_ZERO_EDITS: %s", tc.id, gOut.String())
				}

				// 2) Genuine valid edit -> MUST score 4/4 PASS!
				mode := fsperm.File
				if strings.HasSuffix(tc.targetFile, ".sh") {
					mode = fsperm.Dir
				}
				if err := writeFileContainment(staged.WorkspaceDir, tc.targetFile, []byte(tc.validEdit), mode); err != nil {
					t.Fatalf("write valid edit %s: %v", tc.id, err)
				}
				validHandoff := filepath.Join(auxDir, "handoff-valid.md")
				writeFilePerm(t, validHandoff, makeModifiedHandoff(tc.packetID, tc.neutralDir, tc.targetFile, tc.validFuncs), fsperm.File)

				gOut.Reset()
				gErr.Reset()
				rc = Run(ctx, []string{
					"grade",
					"--scenario", tc.id,
					"--workspace", staged.WorkspaceDir,
					"--handoff", validHandoff,
					"--transcript", transcriptPath,
					"--base-commit", staged.BaseCommit,
					"--repo-root", repo,
					"--format", "json",
				}, &gOut, &gErr)
				if rc != 0 {
					t.Fatalf("valid edit %s failed (rc=%d): stdout=%s stderr=%s", tc.id, rc, gOut.String(), gErr.String())
				}
				var validRes GradeResult
				if err := json.Unmarshal(gOut.Bytes(), &validRes); err != nil {
					t.Fatalf("unmarshal valid grade %s: %v", tc.id, err)
				}
				if !validRes.Passed || validRes.Score != 4 || validRes.MaxScore != 4 {
					t.Fatalf("valid edit %s want 4/4 passed, got %+v", tc.id, validRes)
				}
			} else {
				if rc != 0 || !noopRes.Passed || noopRes.Score != 4 || noopRes.MaxScore != 4 {
					t.Fatalf("DO-NOT-EDIT %s want 4/4 passed, got rc=%d res=%+v", tc.id, rc, noopRes)
				}
			}

			// 3) Apply buggy after.* over target_file and grade -> MUST FAIL.
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
			writeFilePerm(t, buggyHandoff, makeModifiedHandoff(tc.packetID, tc.neutralDir, tc.targetFile, tc.buggyModFunc), fsperm.File)

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
	realDir := neutralTempDir(t)
	linkParent := neutralTempDir(t)
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
	outDir := neutralTempDir(t)

	var stageOut, stageErr bytes.Buffer
	if rc := Run(ctx, []string{"stage", "E-01", "--out-dir", outDir, "--repo-root", repo, "--format", "json"}, &stageOut, &stageErr); rc != 0 {
		t.Fatalf("stage E-01: %s", stageErr.String())
	}
	var staged StageResult
	if err := json.Unmarshal(stageOut.Bytes(), &staged); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := writeFileContainment(staged.WorkspaceDir, "pkg/middleware/errors.go", []byte(validE01GoFix), fsperm.File); err != nil {
		t.Fatalf("apply valid E-01 fix: %v", err)
	}
	auxDir := neutralTempDir(t)
	handoffPath := filepath.Join(auxDir, "handoff.md")
	writeFilePerm(t, handoffPath, makeModifiedHandoff("PKT-MW-001", "svc-middleware", "pkg/middleware/errors.go", 1), fsperm.File)
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
	if len(lines) < 2 {
		t.Fatalf("expected header + decision row(s), got %d lines:\n%s", len(lines), string(tsvBytes))
	}
	loopEvalRows := 0
	for _, ln := range lines[1:] {
		c := strings.Split(ln, "\t")
		if len(c) == 6 && c[1] == "loop-eval" {
			loopEvalRows++
		}
	}
	if loopEvalRows != 1 {
		t.Fatalf("expected 1 loop-eval row, got %d:\n%s", loopEvalRows, string(tsvBytes))
	}
	cols := strings.Split(lines[len(lines)-1], "\t")
	if len(cols) != 6 {
		t.Fatalf("expected 6 TSV columns, got %d: %q", len(cols), lines[len(lines)-1])
	}
	if cols[1] != "loop-eval" || cols[5] != "info" {
		t.Fatalf("expected phase=loop-eval result=info, got phase=%q result=%q", cols[1], cols[5])
	}
	if !guards.IsValidEvidencePointer(cols[4]) {
		t.Fatalf("evidence column %q is not a valid pointer", cols[4])
	}
	prevLineCount := len(lines)

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
	if len(lines) != prevLineCount+1 {
		t.Fatalf("expected +1 decision row after recording regression (prev=%d), got %d:\n%s", prevLineCount, len(lines), string(tsvBytes))
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
	outDir := neutralTempDir(t)

	var stageOut, stageErr bytes.Buffer
	if rc := Run(ctx, []string{"stage", "E-01", "--out-dir", outDir, "--repo-root", repo, "--format", "json"}, &stageOut, &stageErr); rc != 0 {
		t.Fatalf("stage E-01: %s", stageErr.String())
	}
	var staged StageResult
	if err := json.Unmarshal(stageOut.Bytes(), &staged); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := writeFileContainment(staged.WorkspaceDir, "pkg/middleware/errors.go", []byte(validE01GoFix), fsperm.File); err != nil {
		t.Fatalf("apply valid E-01 fix: %v", err)
	}
	auxDir := neutralTempDir(t)
	handoffPath := filepath.Join(auxDir, "handoff.md")
	writeFilePerm(t, handoffPath, makeModifiedHandoff("PKT-MW-001", "svc-middleware", "pkg/middleware/errors.go", 1), fsperm.File)

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

func neutralTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "work-run-*")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// TestSPEC03ZeroEditNoopFailsAndRunShSupportsPolicy reproduces SPEC-03:
// - Zero-edit no-op on E-01 must NOT score 4/4 PASS (fails with NOOP_ZERO_EDITS and run.sh exit 1).
// - Staged scripts/lint/run.sh must parse --policy block-new and --policy=fix-existing.
// - Genuine valid fix on E-01 scores 4/4 PASS.
func TestSPEC03ZeroEditNoopFailsAndRunShSupportsPolicy(t *testing.T) {
	ctx := context.Background()
	repo := testRepoRoot(t)
	outDir := neutralTempDir(t)

	var stageOut, stageErr bytes.Buffer
	if rc := Run(ctx, []string{"stage", "E-01", "--out-dir", outDir, "--repo-root", repo, "--format", "json"}, &stageOut, &stageErr); rc != 0 {
		t.Fatalf("stage E-01 failed (rc=%d): %s", rc, stageErr.String())
	}
	var staged StageResult
	if err := json.Unmarshal(stageOut.Bytes(), &staged); err != nil {
		t.Fatalf("unmarshal stage: %v", err)
	}

	auxDir := neutralTempDir(t)
	cleanHandoff := filepath.Join(auxDir, "ho-noop.md")
	writeFilePerm(t, cleanHandoff, makeCleanHandoff("PKT-MW-001", "svc-middleware"), fsperm.File)
	trPath := filepath.Join(auxDir, "tr.jsonl")
	writeFilePerm(t, trPath, `{"type":"tool_use","name":"Read","input":{"file_path":"pkg/middleware/errors.go"}}`+"\n", fsperm.File)

	// 1) Zero-edit no-op MUST FAIL with NOOP_ZERO_EDITS (even with --no-rerun).
	var gOut, gErr bytes.Buffer
	rc := Run(ctx, []string{
		"grade",
		"--scenario", "E-01",
		"--workspace", staged.WorkspaceDir,
		"--handoff", cleanHandoff,
		"--transcript", trPath,
		"--require-transcript",
		"--base-commit", staged.BaseCommit,
		"--no-rerun",
		"--repo-root", repo,
		"--format", "json",
	}, &gOut, &gErr)
	if rc != 1 {
		t.Fatalf("zero-edit E-01 with --no-rerun: want rc=1, got %d (stdout=%s)", rc, gOut.String())
	}
	if !strings.Contains(gOut.String(), "NOOP_ZERO_EDITS") {
		t.Fatalf("expected NOOP_ZERO_EDITS in grade output, got: %s", gOut.String())
	}

	// 2) On unmodified before.go:
	//    - `./scripts/lint/run.sh svc-middleware` (default fix-existing) exits 1 (errorlint violation)
	//    - `./scripts/lint/run.sh --policy=fix-existing svc-middleware` exits 1
	//    - `./scripts/lint/run.sh --policy block-new svc-middleware` exits 0 (parses --policy cleanly)
	if code, _, _, _, err := runBashForeground(ctx, "./scripts/lint/run.sh svc-middleware", staged.WorkspaceDir, 10_000_000_000); err != nil || code != 1 {
		t.Fatalf("run.sh default on unmodified E-01: want exit 1, got code=%d err=%v", code, err)
	}
	if code, _, _, _, err := runBashForeground(ctx, "./scripts/lint/run.sh --policy=fix-existing svc-middleware", staged.WorkspaceDir, 10_000_000_000); err != nil || code != 1 {
		t.Fatalf("run.sh --policy=fix-existing on unmodified E-01: want exit 1, got code=%d err=%v", code, err)
	}
	if code, _, outB, errB, err := runBashForeground(ctx, "./scripts/lint/run.sh --policy block-new svc-middleware", staged.WorkspaceDir, 10_000_000_000); err != nil || code != 0 {
		t.Fatalf("run.sh --policy block-new on unmodified E-01: want exit 0, got code=%d err=%v out=%s stderr=%s", code, err, string(outB), string(errB))
	}

	// 3) Genuine valid errorlint fix (errors.As + && he.Code >= 500) -> run.sh exits 0 AND grade scores 4/4 PASS.
	validFix := `package middleware

import "errors"

type HTTPError struct {
	Code    int
	Message string
}

func (e *HTTPError) Error() string {
	return e.Message
}

func recordServerError(err error, recordFailure func()) {
	if err == nil {
		return
	}
	// Only 5xx server errors should trip the circuit breaker; 4xx must be ignored.
	var he *HTTPError
	if errors.As(err, &he) && he.Code >= 500 {
		recordFailure()
	}
}
`
	if err := writeFileContainment(staged.WorkspaceDir, "pkg/middleware/errors.go", []byte(validFix), fsperm.File); err != nil {
		t.Fatalf("write valid fix: %v", err)
	}
	if code, _, outB, errB, err := runBashForeground(ctx, "./scripts/lint/run.sh --policy=fix-existing svc-middleware", staged.WorkspaceDir, 10_000_000_000); err != nil || code != 0 {
		t.Fatalf("run.sh --policy=fix-existing after valid fix: want exit 0, got code=%d err=%v out=%s stderr=%s", code, err, string(outB), string(errB))
	}
	validHandoff := filepath.Join(auxDir, "ho-valid.md")
	writeFilePerm(t, validHandoff, makeModifiedHandoff("PKT-MW-001", "svc-middleware", "pkg/middleware/errors.go", 1), fsperm.File)
	gOut.Reset()
	gErr.Reset()
	rc = Run(ctx, []string{
		"grade",
		"--scenario", "E-01",
		"--workspace", staged.WorkspaceDir,
		"--handoff", validHandoff,
		"--transcript", trPath,
		"--require-transcript",
		"--base-commit", staged.BaseCommit,
		"--repo-root", repo,
		"--format", "json",
	}, &gOut, &gErr)
	if rc != 0 {
		t.Fatalf("valid fix E-01 grade: want rc=0, got %d (stdout=%s stderr=%s)", rc, gOut.String(), gErr.String())
	}
	var res GradeResult
	if err := json.Unmarshal(gOut.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal valid grade: %v", err)
	}
	if !res.Passed || res.Score != 4 || res.MaxScore != 4 {
		t.Fatalf("valid fix E-01: want 4/4 PASS, got %+v", res)
	}
}

// TestSPEC08AndSEC06OracleIsolationOutDirNeutralityAndHandoffParity reproduces SPEC-08 and SEC-06:
// - --out-dir containing forbidden tokens (/tmp/.../eval-benchmark-leak) rejected with NON_NEUTRAL_OUT_DIR.
// - test_characterization.sh MUST NOT exist inside staged workspace.
// - Internal harness labels (e.g. ARCH-02 in E-04/C-11) stripped from staged target file.
// - Tampering with workspace/test_characterization.sh cannot fool grader (oracle executes from sc.Dir).
// - Markdown handoff with non-structured frontmatter + fenced ```json with unread extra.go FAILS with UNVERIFIED_FILE_READ_CLAIM.
// - User-role spoofed transcript and failed read tool calls FAIL with UNVERIFIED_FILE_READ_CLAIM.
func TestSPEC08AndSEC06OracleIsolationOutDirNeutralityAndHandoffParity(t *testing.T) {
	ctx := context.Background()
	repo := testRepoRoot(t)

	// 1) Non-neutral --out-dir MUST be rejected with NON_NEUTRAL_OUT_DIR.
	parent := neutralTempDir(t)
	badOutDir := filepath.Join(parent, "eval-benchmark-leak")
	var stageOut, stageErr bytes.Buffer
	rc := Run(ctx, []string{"stage", "E-01", "--out-dir", badOutDir, "--repo-root", repo}, &stageOut, &stageErr)
	if rc == 0 {
		t.Fatalf("non-neutral --out-dir %s was accepted (rc=0)", badOutDir)
	}
	if !strings.Contains(stageErr.String(), "NON_NEUTRAL_OUT_DIR") {
		t.Fatalf("expected NON_NEUTRAL_OUT_DIR error, got: %s", stageErr.String())
	}

	// 2) Oracle isolation + ARCH-02 label stripping on E-04.
	goodOutDir := neutralTempDir(t)
	stageOut.Reset()
	stageErr.Reset()
	if rc := Run(ctx, []string{"stage", "E-04", "--out-dir", goodOutDir, "--repo-root", repo, "--format", "json"}, &stageOut, &stageErr); rc != 0 {
		t.Fatalf("stage E-04 failed: %s", stageErr.String())
	}
	var stagedE04 StageResult
	if err := json.Unmarshal(stageOut.Bytes(), &stagedE04); err != nil {
		t.Fatalf("unmarshal E-04 stage: %v", err)
	}
	if _, err := os.Stat(filepath.Join(stagedE04.WorkspaceDir, "test_characterization.sh")); !os.IsNotExist(err) {
		t.Fatalf("oracle test_characterization.sh must NOT be staged into workspace")
	}
	stagedMain, err := readFileContainment(stagedE04.WorkspaceDir, "cmd/server/main.go")
	if err != nil {
		t.Fatalf("read staged cmd/server/main.go: %v", err)
	}
	if strings.Contains(string(stagedMain), "ARCH-02") {
		t.Fatalf("internal harness label ARCH-02 leaked into staged cmd/server/main.go:\n%s", string(stagedMain))
	}

	// 3) SEC-06 exact repro: metadata frontmatter + fenced ```json with unread extra.go + user-role transcript spoof.
	stageOut.Reset()
	stageErr.Reset()
	if rc := Run(ctx, []string{"stage", "E-01", "--out-dir", goodOutDir, "--repo-root", repo, "--format", "json"}, &stageOut, &stageErr); rc != 0 {
		t.Fatalf("stage E-01 failed: %s", stageErr.String())
	}
	var stagedE01 StageResult
	if err := json.Unmarshal(stageOut.Bytes(), &stagedE01); err != nil {
		t.Fatalf("unmarshal E-01 stage: %v", err)
	}
	if _, err := os.Stat(filepath.Join(stagedE01.WorkspaceDir, "test_characterization.sh")); !os.IsNotExist(err) {
		t.Fatalf("oracle test_characterization.sh must NOT be staged into E-01 workspace")
	}
	// Plant a fake workspace test_characterization.sh that always exits 0, and break errors.go -> oracle from sc.Dir must STILL fail!
	writeFilePerm(t, filepath.Join(stagedE01.WorkspaceDir, "test_characterization.sh"), "#!/usr/bin/env bash\nexit 0\n", fsperm.Dir)
	afterBytes, err := readFileContainment(filepath.Join(repo, "scripts/harness/corpus/C-01"), "after.go")
	if err != nil {
		t.Fatalf("read C-01/after.go: %v", err)
	}
	if err := writeFileContainment(stagedE01.WorkspaceDir, "pkg/middleware/errors.go", afterBytes, fsperm.File); err != nil {
		t.Fatalf("write C-01/after.go: %v", err)
	}
	scE01, err := loadScenario(repo, "E-01")
	if err != nil {
		t.Fatalf("loadScenario E-01: %v", err)
	}
	charSig := runCharTestSignal(ctx, stagedE01.WorkspaceDir, scE01)
	if charSig.Passed {
		t.Fatalf("tampered workspace/test_characterization.sh bypassed trusted oracle in sc.Dir")
	}
	_ = os.Remove(filepath.Join(stagedE01.WorkspaceDir, "test_characterization.sh"))

	// Restore valid E-01 fix + add extra.go not read in transcript, with metadata frontmatter + fenced json handoff.
	validE01 := `package middleware

import "errors"

type HTTPError struct {
	Code    int
	Message string
}

func (e *HTTPError) Error() string {
	return e.Message
}

func recordServerError(err error, recordFailure func()) {
	if err == nil {
		return
	}
	var he *HTTPError
	if errors.As(err, &he) && he.Code >= 500 {
		recordFailure()
	}
}
`
	if err := writeFileContainment(stagedE01.WorkspaceDir, "pkg/middleware/errors.go", []byte(validE01), fsperm.File); err != nil {
		t.Fatalf("write valid E-01: %v", err)
	}
	if err := writeFileContainment(stagedE01.WorkspaceDir, "pkg/middleware/extra.go", []byte("package middleware\nfunc H() int { return 1 }\n"), fsperm.File); err != nil {
		t.Fatalf("write extra.go: %v", err)
	}

	auxDir := neutralTempDir(t)
	fencedHandoff := filepath.Join(auxDir, "handoff-fenced.md")
	writeFilePerm(t, fencedHandoff, `---
doc: handoff
---
## 1. 修正ファイルと行数
x
## 2. 実行時挙動の変更可能性の有無
x
## 3. 変更理由
x
`+"```json\n"+`{"packet_id":"PKT-MW-001","status":"completed","modified_files":[{"path":"pkg/middleware/errors.go","added_lines":3,"deleted_lines":1},{"path":"pkg/middleware/extra.go","added_lines":2,"deleted_lines":0}],"modified_functions_count":2,"runtime_behavior_impact":{"has_change":false,"checked_dimensions":["control_flow"],"summary":"変更なし","candidate_responses":[]},"rationale":"x","verification":{"command":"./scripts/lint/run.sh svc-middleware","exit_code":0}}`+"\n```\n", fsperm.File)

	// (a) Transcript reads only errors.go (not extra.go) -> fenced JSON handoff MUST expose extra.go as UNVERIFIED_FILE_READ_CLAIM!
	trPartial := filepath.Join(auxDir, "tr-partial.jsonl")
	writeFilePerm(t, trPartial, `{"type":"tool_use","name":"Read","input":{"file_path":"pkg/middleware/errors.go"}}`+"\n", fsperm.File)
	var gOut, gErr bytes.Buffer
	rc = Run(ctx, []string{
		"grade",
		"--scenario", "E-01",
		"--workspace", stagedE01.WorkspaceDir,
		"--handoff", fencedHandoff,
		"--transcript", trPartial,
		"--require-transcript",
		"--base-commit", stagedE01.BaseCommit,
		"--repo-root", repo,
		"--format", "json",
	}, &gOut, &gErr)
	if rc != 1 {
		t.Fatalf("fenced JSON handoff with unread extra.go: want rc=1, got %d (stdout=%s)", rc, gOut.String())
	}
	var partialRes GradeResult
	if err := json.Unmarshal(gOut.Bytes(), &partialRes); err != nil {
		t.Fatalf("unmarshal partial: %v", err)
	}
	if partialRes.Signals.TranscriptReadAudit.Passed {
		t.Fatalf("expected TranscriptReadAudit to fail for unread extra.go in fenced JSON handoff: %+v", partialRes.Signals.TranscriptReadAudit)
	}
	foundExtra := false
	for _, u := range partialRes.Signals.TranscriptReadAudit.UnverifiedClaims {
		if u == "pkg/middleware/extra.go" {
			foundExtra = true
		}
	}
	if !foundExtra {
		t.Fatalf("expected pkg/middleware/extra.go in unverified_claims, got %+v", partialRes.Signals.TranscriptReadAudit.UnverifiedClaims)
	}

	// (b) User-role metadata spoof and failed tool invocations MUST NOT count as observed reads.
	spoofCases := []struct {
		name  string
		jsonl string
	}{
		{
			name:  "user_role_nested_map",
			jsonl: `{"role":"user","m":{"name":"read","path":"pkg/middleware/errors.go"}}` + "\n",
		},
		{
			name:  "system_role_direct",
			jsonl: `{"role":"system","name":"read","input":{"path":"pkg/middleware/errors.go"}}` + "\n",
		},
		{
			name:  "arbitrary_nested_metadata",
			jsonl: `{"role":"assistant","metadata":{"capability":{"name":"read","path":"pkg/middleware/errors.go"}}}` + "\n",
		},
		{
			name:  "is_error_true",
			jsonl: `{"type":"tool_use","name":"Read","input":{"file_path":"pkg/middleware/errors.go"},"is_error":true}` + "\n",
		},
		{
			name:  "status_denied",
			jsonl: `{"tool":"read_file","args":{"path":"pkg/middleware/errors.go"},"status":"denied"}` + "\n",
		},
		{
			name:  "non_empty_error_field",
			jsonl: `{"tool":"read_file","args":{"path":"pkg/middleware/errors.go"},"error":"ENOENT"}` + "\n",
		},
		{
			name: "tool_result_is_error_two_line",
			jsonl: `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"toolu_1","name":"Read","input":{"file_path":"pkg/middleware/errors.go"}}]}}` + "\n" +
				`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_1","is_error":true,"content":"not found"}]}}` + "\n",
		},
	}
	for _, sc := range spoofCases {
		t.Run(sc.name, func(t *testing.T) {
			tp := filepath.Join(auxDir, sc.name+".jsonl")
			writeFilePerm(t, tp, sc.jsonl, fsperm.File)
			sig := runTranscriptAuditSignal(stagedE01.WorkspaceDir, tp, true, []string{"pkg/middleware/errors.go"}, nil, nil)
			if sig.Passed || len(sig.ObservedReads) != 0 {
				t.Fatalf("spoof %s unexpectedly observed reads or passed: %+v", sc.name, sig)
			}
			if len(sig.Errors) == 0 || sig.Errors[0].Code != "UNVERIFIED_FILE_READ_CLAIM" {
				t.Fatalf("spoof %s expected UNVERIFIED_FILE_READ_CLAIM, got %+v", sc.name, sig.Errors)
			}
		})
	}
}

// TestSPEC04UnsafeClaimedReadAndNormalizedCompare reproduces SPEC-04:
// - Unsafe --claimed-read (../secret.go, /etc/passwd) MUST fail with UNVERIFIED_FILE_READ_CLAIM (not silently dropped).
// - Comparing 3/3 vs 4/4 MUST output verdict="no_change" (normalized score 1.0 vs 1.0).
// - Comparing 3/3 vs 3/4 MUST output verdict="regressed" (normalized score 1.0 vs 0.75).
// - Dropping transcript evaluation (before evaluated, after omitted) MUST output verdict="regressed".
func TestSPEC04UnsafeClaimedReadAndNormalizedCompare(t *testing.T) {
	ctx := context.Background()
	ws := neutralTempDir(t)
	aux := neutralTempDir(t)
	trPath := filepath.Join(aux, "tr.jsonl")
	writeFilePerm(t, trPath, `{"type":"tool_use","name":"Read","input":{"file_path":"pkg/middleware/errors.go"}}`+"\n", fsperm.File)

	for _, badClaim := range []string{"../secret.go", "/etc/passwd"} {
		sig := runTranscriptAuditSignal(ws, trPath, true, []string{"pkg/middleware/errors.go"}, nil, []string{badClaim})
		if sig.Passed {
			t.Fatalf("unsafe --claimed-read %q unexpectedly passed: %+v", badClaim, sig)
		}
		if sig.Fidelity >= 1.0 {
			t.Fatalf("unsafe --claimed-read %q should reduce fidelity below 1.0, got %v", badClaim, sig.Fidelity)
		}
		foundClaim := false
		for _, u := range sig.UnverifiedClaims {
			if u == badClaim {
				foundClaim = true
			}
		}
		if !foundClaim {
			t.Fatalf("unsafe --claimed-read %q missing from UnverifiedClaims: %+v", badClaim, sig.UnverifiedClaims)
		}
		foundCode := false
		for _, e := range sig.Errors {
			if e.Code == "UNVERIFIED_FILE_READ_CLAIM" {
				foundCode = true
			}
		}
		if !foundCode {
			t.Fatalf("unsafe --claimed-read %q missing UNVERIFIED_FILE_READ_CLAIM error: %+v", badClaim, sig.Errors)
		}
	}

	tmp := neutralTempDir(t)
	makeNoTr := func(score, maxScore int, passed bool) GradeResult {
		return GradeResult{ScenarioID: "E-01", Score: score, MaxScore: maxScore, Passed: passed}
	}
	b3of3 := filepath.Join(tmp, "b3of3.json")
	a4of4 := filepath.Join(tmp, "a4of4.json")
	a3of4 := filepath.Join(tmp, "a3of4.json")

	raw3of3, _ := json.Marshal([]GradeResult{makeNoTr(3, 3, true), makeNoTr(3, 3, true), makeNoTr(3, 3, true)})
	raw4of4, _ := json.Marshal([]GradeResult{
		makeGradeSample("E-01", true, 4, 4, 1.0),
		makeGradeSample("E-01", true, 4, 4, 1.0),
		makeGradeSample("E-01", true, 4, 4, 1.0),
	})
	raw3of4, _ := json.Marshal([]GradeResult{
		makeGradeSample("E-01", false, 3, 4, 0.5),
		makeGradeSample("E-01", false, 3, 4, 0.5),
		makeGradeSample("E-01", false, 3, 4, 0.5),
	})
	writeFilePerm(t, b3of3, string(raw3of3), fsperm.File)
	writeFilePerm(t, a4of4, string(raw4of4), fsperm.File)
	writeFilePerm(t, a3of4, string(raw3of4), fsperm.File)

	// 1) 3/3 (100%) vs 4/4 (100%) -> no_change, delta 0.0.
	var out, errBuf bytes.Buffer
	rc := Run(ctx, []string{"compare", "--before", b3of3, "--after", a4of4, "--repo-root", tmp, "--format", "json"}, &out, &errBuf)
	if rc != 0 {
		t.Fatalf("3/3 vs 4/4: want rc=0, got %d (out=%s err=%s)", rc, out.String(), errBuf.String())
	}
	var cmpSame CompareReport
	if err := json.Unmarshal(out.Bytes(), &cmpSame); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if cmpSame.Verdict != "no_change" || cmpSame.DeltaMeanScore != 0.0 || cmpSame.BeforeMeanScore != 1.0 || cmpSame.AfterMeanScore != 1.0 {
		t.Fatalf("3/3 vs 4/4 want verdict=no_change normalized 1.0->1.0 (delta 0), got %+v", cmpSame)
	}

	// 2) 3/3 (100%) vs 3/4 (75%) -> regressed.
	out.Reset()
	errBuf.Reset()
	rc = Run(ctx, []string{"compare", "--before", b3of3, "--after", a3of4, "--repo-root", tmp, "--format", "json"}, &out, &errBuf)
	if rc != 1 {
		t.Fatalf("3/3 vs 3/4: want rc=1, got %d (%s)", rc, out.String())
	}
	var cmpDrop CompareReport
	if err := json.Unmarshal(out.Bytes(), &cmpDrop); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if cmpDrop.Verdict != "regressed" || cmpDrop.DeltaMeanScore >= 0 {
		t.Fatalf("3/3 vs 3/4 want verdict=regressed negative delta, got %+v", cmpDrop)
	}

	// 3) Dropping transcript evaluation (4/4 evaluated -> 3/3 unevaluated) -> regressed.
	out.Reset()
	errBuf.Reset()
	rc = Run(ctx, []string{"compare", "--before", a4of4, "--after", b3of3, "--repo-root", tmp, "--format", "json"}, &out, &errBuf)
	if rc != 1 {
		t.Fatalf("dropped transcript audit: want rc=1, got %d (%s)", rc, out.String())
	}
	var cmpDropTr CompareReport
	if err := json.Unmarshal(out.Bytes(), &cmpDropTr); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if cmpDropTr.Verdict != "regressed" {
		t.Fatalf("dropped transcript audit want verdict=regressed, got %+v", cmpDropTr)
	}
}

// TestSEC01INV01CompareSymlinksAndSpaceEvidence reproduces SEC-01 and INV-01:
// - Dangling symlink on --decisions-file or --runs-dir MUST be refused without creating victim files.
// - Outside-repo --runs-dir containing spaces MUST encode a valid file:// evidence pointer so compare --record succeeds cleanly without orphan artifacts.
func TestSEC01INV01CompareSymlinksAndSpaceEvidence(t *testing.T) {
	ctx := context.Background()
	tmpRepo := neutralTempDir(t)
	samples := []GradeResult{
		makeGradeSample("E-01", true, 4, 4, 1.0),
		makeGradeSample("E-01", true, 4, 4, 1.0),
		makeGradeSample("E-01", true, 4, 4, 1.0),
	}
	raw, _ := json.Marshal(samples)
	bPath := filepath.Join(tmpRepo, "b.json")
	aPath := filepath.Join(tmpRepo, "a.json")
	writeFilePerm(t, bPath, string(raw), fsperm.File)
	writeFilePerm(t, aPath, string(raw), fsperm.File)

	// 1) Dangling symlink at decisions.tsv MUST be rejected and victim MUST NOT be created.
	outside := neutralTempDir(t)
	victim := filepath.Join(outside, "victim.tsv")
	runsDir := filepath.Join(tmpRepo, ".harness", "runs")
	branchDir := filepath.Join(runsDir, "main")
	if err := os.MkdirAll(branchDir, fsperm.Dir); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	danglingDec := filepath.Join(branchDir, "decisions.tsv")
	if err := os.Symlink(victim, danglingDec); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	var out, errBuf bytes.Buffer
	rc := Run(ctx, []string{
		"compare",
		"--before", bPath,
		"--after", aPath,
		"--record",
		"--branch", "main",
		"--runs-dir", runsDir,
		"--repo-root", tmpRepo,
	}, &out, &errBuf)
	if rc == 0 {
		t.Fatalf("expected dangling symlink decisions.tsv to fail, got rc=0")
	}
	if _, err := os.Stat(victim); err == nil {
		t.Fatalf("dangling symlink victim file %s was created outside repo", victim)
	}
	if entries, err := os.ReadDir(filepath.Join(branchDir, "eval")); err == nil && len(entries) > 0 {
		t.Fatalf("orphan compare artifact left behind after symlink refusal: %+v", entries)
	}

	// 2) Symlinked --runs-dir MUST be rejected.
	symRuns := filepath.Join(tmpRepo, "sym-runs")
	if err := os.Symlink(outside, symRuns); err != nil {
		t.Fatalf("symlink runs-dir: %v", err)
	}
	out.Reset()
	errBuf.Reset()
	rc = Run(ctx, []string{
		"compare",
		"--before", bPath,
		"--after", aPath,
		"--record",
		"--branch", "main",
		"--runs-dir", symRuns,
		"--repo-root", tmpRepo,
	}, &out, &errBuf)
	if rc == 0 {
		t.Fatalf("expected symlink --runs-dir to fail, got rc=0")
	}

	// 3) INV-01: Outside-repo --runs-dir with spaces produces valid file:// pointer and passes decisions.tsv write.
	spaceRuns := filepath.Join(outside, "my eval runs")
	if err := os.MkdirAll(spaceRuns, fsperm.Dir); err != nil {
		t.Fatalf("mkdir spaceRuns: %v", err)
	}
	out.Reset()
	errBuf.Reset()
	rc = Run(ctx, []string{
		"compare",
		"--before", bPath,
		"--after", aPath,
		"--record",
		"--branch", "main",
		"--runs-dir", spaceRuns,
		"--repo-root", tmpRepo,
		"--format", "json",
	}, &out, &errBuf)
	if rc != 0 {
		t.Fatalf("compare --record with spaces in --runs-dir failed (rc=%d): out=%s err=%s", rc, out.String(), errBuf.String())
	}
	var spaceCmp CompareReport
	if err := json.Unmarshal(out.Bytes(), &spaceCmp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	tsvBytes, err := readRegularFile(spaceCmp.DecisionsFile)
	if err != nil {
		t.Fatalf("read space decisions.tsv: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(tsvBytes)), "\n")
	lastCols := strings.Split(lines[len(lines)-1], "\t")
	if len(lastCols) != 6 || !guards.IsValidEvidencePointer(lastCols[4]) {
		t.Fatalf("invalid evidence pointer for space runs-dir: %q", lastCols)
	}
}

// TestSEC08BackgroundEscapeBypass reproduces SEC-08:
// Unquoted backslash-quote (`echo \" ; echo BG &`) and double-quoted command substitution (`echo "$(sleep 1 &)"`)
// MUST be rejected by hasBackgroundExecution.
func TestSEC08BackgroundEscapeBypass(t *testing.T) {
	blocked := []string{
		`echo \" ; echo BG &`,
		`echo \' ; echo BG &`,
		`echo "$(sleep 1 &)"`,
		"echo \"`sleep 1 &`\"",
		`echo \\"ok\\" &`,
		"# comment\nsleep 1 &",
	}
	for _, cmd := range blocked {
		if !hasBackgroundExecution(cmd) {
			t.Errorf("hasBackgroundExecution(%q) = false, want true (blocked)", cmd)
		}
	}
	allowed := []string{
		`echo "foo & bar"`,
		`echo 'foo & bar'`,
		`go test ./... && go vet ./...`,
		`bash script.sh 2>&1`,
		`bash script.sh &>out.log`,
	}
	for _, cmd := range allowed {
		if hasBackgroundExecution(cmd) {
			t.Errorf("hasBackgroundExecution(%q) = true, want false (allowed)", cmd)
		}
	}
}
