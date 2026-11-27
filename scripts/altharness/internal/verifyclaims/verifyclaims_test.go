package verifyclaims

import (
	"alt/scripts/altharness/internal/fsperm"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runCLI(args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), fsperm.File); err != nil {
		t.Fatal(err)
	}
}

func TestValidateReportValid(t *testing.T) {
	t.Parallel()

	t.Run("2-column Japanese claim evidence table with anchor", func(t *testing.T) {
		t.Parallel()
		repo := t.TempDir()
		writeFile(t, filepath.Join(repo, "docs", "review", "ledger.md"), "# Evidence\n\n## Plecto\nok\n")
		report := filepath.Join(repo, "report.md")
		writeFile(t, report, `# 実装報告

## 1. 変更概要
- plecto の allow スコープ整理。

## 2. 主張と証拠

| 主張 | 証拠（コマンドと出力の保存先） |
| --- | --- |
| plecto の allow スコープ変更前後で WIT が一致する | コマンド: `+"`./scripts/altharness/alt-harness verify-claims plecto-wit --base-commit ad0850966~1`"+` / 保存先: `+"`docs/review/ledger.md#plecto`"+` |
`)

		code, out, errOut := runCLI("validate", report, "--repo-root", repo, "--format", "json")
		if code != exitOK {
			t.Fatalf("expected exit 0, got %d\nstdout=%s\nstderr=%s", code, out, errOut)
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(out), &payload); err != nil {
			t.Fatalf("unmarshal json: %v\n%s", err, out)
		}
		if payload["valid"] != true {
			t.Fatalf("expected valid=true, got %#v", payload)
		}
	})

	t.Run("3-column English Claim Command Evidence table", func(t *testing.T) {
		t.Parallel()
		repo := t.TempDir()
		writeFile(t, filepath.Join(repo, "evidence", "clippy.log"), "0 warnings\n")
		report := filepath.Join(repo, "report.md")
		writeFile(t, report, `# Implementation Report

| Claim | Command | Evidence |
| --- | --- | --- |
| recap-worker clippy reports zero warnings after allow relocation | `+"`cargo +1.99.1 clippy --all-targets -- -D warnings`"+` | [clippy.log](evidence/clippy.log) |
`)

		code, out, errOut := runCLI("validate", report, "--repo-root", repo)
		if code != exitOK || !strings.Contains(out, "OK:") {
			t.Fatalf("expected exit 0 OK, got %d stdout=%q stderr=%q", code, out, errOut)
		}
	})
}

func TestValidateReportRejectsBlanketClaims(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		content string
		wantErr string
	}{
		{
			name: "blanket Japanese phrase outside table",
			content: `# 実装報告

本番サービス実行時挙動への影響ゼロを確認済み。

| 主張 | 証拠（コマンドと出力の保存先） |
| --- | --- |
| plecto WIT 一致 | コマンド: ` + "`wasm-tools component wit out.wasm`" + ` / 保存先: ` + "`evidence.txt`" + ` |
`,
			wantErr: "BLANKET_UNBACKED_CLAIM",
		},
		{
			name: "blanket English zero impact outside table",
			content: `# Report

Summary: zero impact on production services.

| Claim | Evidence |
| --- | --- |
| plecto WIT matches | Command: ` + "`wasm-tools component wit out.wasm`" + ` / Path: ` + "`evidence.txt`" + ` |
`,
			wantErr: "BLANKET_UNBACKED_CLAIM",
		},
		{
			name: "blanket 動作変更なし outside table",
			content: `# 実装報告

- 全モジュールで動作変更なし。

| 主張 | 証拠（コマンドと出力の保存先） |
| --- | --- |
| recap-worker clippy 警告 0 | コマンド: ` + "`cargo clippy -- -D warnings`" + ` / 保存先: ` + "`evidence.txt`" + ` |
`,
			wantErr: "BLANKET_UNBACKED_CLAIM",
		},
		{
			name: "unbacked blanket row inside table without command or path",
			content: `# 実装報告

| 主張 | 証拠（コマンドと出力の保存先） |
| --- | --- |
| 本番サービス実行時挙動への影響ゼロ | 全ゲート・ユニットテスト exit 0 |
`,
			wantErr: "BLANKET_UNBACKED_CLAIM",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := t.TempDir()
			writeFile(t, filepath.Join(repo, "evidence.txt"), "real output\n")
			report := filepath.Join(repo, "report.md")
			writeFile(t, report, tc.content)

			code, out, errOut := runCLI("validate", report, "--repo-root", repo)
			if code != exitInvalid || !strings.Contains(errOut, tc.wantErr) {
				t.Fatalf("want exit 1 with %s, got code=%d stdout=%q stderr=%q", tc.wantErr, code, out, errOut)
			}
		})
	}
}

func TestValidateReportRejectsPlaceholdersAndMissingTables(t *testing.T) {
	t.Parallel()

	t.Run("missing claim evidence table", func(t *testing.T) {
		t.Parallel()
		repo := t.TempDir()
		report := filepath.Join(repo, "report.md")
		writeFile(t, report, "# 実装報告\n\n変更を適用しました。\n")

		code, _, errOut := runCLI("validate", report, "--repo-root", repo)
		if code != exitInvalid || !strings.Contains(errOut, "MISSING_CLAIM_TABLE") {
			t.Fatalf("want MISSING_CLAIM_TABLE, got code=%d stderr=%q", code, errOut)
		}
	})

	t.Run("empty table rows rejected", func(t *testing.T) {
		t.Parallel()
		repo := t.TempDir()
		report := filepath.Join(repo, "report.md")
		writeFile(t, report, "# 実装報告\n\n| 主張 | 証拠 |\n| --- | --- |\n")

		code, _, errOut := runCLI("validate", report, "--repo-root", repo)
		if code != exitInvalid || !strings.Contains(errOut, "MISSING_CLAIM_ROWS") {
			t.Fatalf("want MISSING_CLAIM_ROWS, got code=%d stderr=%q", code, errOut)
		}
	})

	for _, ph := range []string{"TBD", "N/A", "-", "", "なし", "TODO"} {
		ph := ph
		t.Run("placeholder_"+ph, func(t *testing.T) {
			t.Parallel()
			repo := t.TempDir()
			writeFile(t, filepath.Join(repo, "evidence.txt"), "ok\n")
			report := filepath.Join(repo, "report.md")
			writeFile(t, report, "# 実装報告\n\n| 主張 | 証拠 |\n| --- | --- |\n| plecto ABI | "+ph+" |\n")

			code, _, errOut := runCLI("validate", report, "--repo-root", repo)
			if code != exitInvalid || !strings.Contains(errOut, "PLACEHOLDER_CELL") {
				t.Fatalf("placeholder %q want PLACEHOLDER_CELL, got code=%d stderr=%q", ph, code, errOut)
			}
		})
	}
}

func TestValidateReportRejectsUnsafeAndMissingEvidencePaths(t *testing.T) {
	t.Parallel()

	t.Run("missing evidence file", func(t *testing.T) {
		t.Parallel()
		repo := t.TempDir()
		report := filepath.Join(repo, "report.md")
		writeFile(t, report, "# 実装報告\n\n| 主張 | 証拠 |\n| --- | --- |\n| plecto WIT | コマンド: `wasm-tools component wit a.wasm` / 保存先: `docs/review/missing.md` |\n")

		code, _, errOut := runCLI("validate", report, "--repo-root", repo)
		if code != exitInvalid || !strings.Contains(errOut, "MISSING_EVIDENCE_FILE") {
			t.Fatalf("want MISSING_EVIDENCE_FILE, got code=%d stderr=%q", code, errOut)
		}
	})

	t.Run("empty 0-byte evidence file", func(t *testing.T) {
		t.Parallel()
		repo := t.TempDir()
		writeFile(t, filepath.Join(repo, "docs", "empty.log"), "")
		report := filepath.Join(repo, "report.md")
		writeFile(t, report, "# 実装報告\n\n| 主張 | 証拠 |\n| --- | --- |\n| plecto WIT | コマンド: `wasm-tools component wit a.wasm` / 保存先: `docs/empty.log` |\n")

		code, _, errOut := runCLI("validate", report, "--repo-root", repo)
		if code != exitInvalid || !strings.Contains(errOut, "EMPTY_EVIDENCE_FILE") {
			t.Fatalf("want EMPTY_EVIDENCE_FILE, got code=%d stderr=%q", code, errOut)
		}
	})

	t.Run("path traversal rejected", func(t *testing.T) {
		t.Parallel()
		parent := t.TempDir()
		writeFile(t, filepath.Join(parent, "secret.txt"), "top secret\n")
		repo := filepath.Join(parent, "repo")
		if err := os.MkdirAll(repo, fsperm.Dir); err != nil {
			t.Fatal(err)
		}
		report := filepath.Join(repo, "report.md")
		writeFile(t, report, "# 実装報告\n\n| 主張 | 証拠 |\n| --- | --- |\n| plecto WIT | コマンド: `wasm-tools component wit a.wasm` / 保存先: `../secret.txt` |\n")

		code, _, errOut := runCLI("validate", report, "--repo-root", repo)
		if code != exitInvalid || !strings.Contains(errOut, "UNSAFE_EVIDENCE_PATH") {
			t.Fatalf("want UNSAFE_EVIDENCE_PATH, got code=%d stderr=%q", code, errOut)
		}
	})

	t.Run("absolute path rejected", func(t *testing.T) {
		t.Parallel()
		repo := t.TempDir()
		outside := filepath.Join(t.TempDir(), "abs.txt")
		writeFile(t, outside, "outside\n")
		report := filepath.Join(repo, "report.md")
		writeFile(t, report, "# 実装報告\n\n| 主張 | 証拠 |\n| --- | --- |\n| plecto WIT | コマンド: `wasm-tools component wit a.wasm` / 保存先: `"+outside+"` |\n")

		code, _, errOut := runCLI("validate", report, "--repo-root", repo)
		if code != exitInvalid || !strings.Contains(errOut, "UNSAFE_EVIDENCE_PATH") {
			t.Fatalf("want UNSAFE_EVIDENCE_PATH, got code=%d stderr=%q", code, errOut)
		}
	})

	t.Run("symlink escape file and directory rejected", func(t *testing.T) {
		t.Parallel()
		outside := t.TempDir()
		outsideFile := filepath.Join(outside, "escaped.log")
		writeFile(t, outsideFile, "escaped data\n")

		repo := t.TempDir()
		if err := os.MkdirAll(filepath.Join(repo, "docs"), fsperm.Dir); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outsideFile, filepath.Join(repo, "docs", "link.log")); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(repo, "linked_dir")); err != nil {
			t.Fatal(err)
		}

		repFile := filepath.Join(repo, "rep_file.md")
		writeFile(t, repFile, "# 実装報告\n\n| 主張 | 証拠 |\n| --- | --- |\n| plecto WIT | コマンド: `wasm-tools component wit a.wasm` / 保存先: `docs/link.log` |\n")
		code, _, errOut := runCLI("validate", repFile, "--repo-root", repo)
		if code != exitInvalid || !strings.Contains(errOut, "UNSAFE_EVIDENCE_PATH") {
			t.Fatalf("symlink file want UNSAFE_EVIDENCE_PATH, got code=%d stderr=%q", code, errOut)
		}

		repDir := filepath.Join(repo, "rep_dir.md")
		writeFile(t, repDir, "# 実装報告\n\n| 主張 | 証拠 |\n| --- | --- |\n| plecto WIT | コマンド: `wasm-tools component wit a.wasm` / 保存先: `linked_dir/escaped.log` |\n")
		code, _, errOut = runCLI("validate", repDir, "--repo-root", repo)
		if code != exitInvalid || !strings.Contains(errOut, "UNSAFE_EVIDENCE_PATH") {
			t.Fatalf("symlink dir want UNSAFE_EVIDENCE_PATH, got code=%d stderr=%q", code, errOut)
		}
	})

	t.Run("proxy text without executable command rejected", func(t *testing.T) {
		t.Parallel()
		repo := t.TempDir()
		writeFile(t, filepath.Join(repo, "docs", "log.txt"), "exit 0\n")
		report := filepath.Join(repo, "report.md")
		writeFile(t, report, "# 実装報告\n\n| 主張 | 証拠 |\n| --- | --- |\n| recap-worker clippy clean | 全ゲート exit 0 / 保存先: `docs/log.txt` |\n")

		code, _, errOut := runCLI("validate", report, "--repo-root", repo)
		if code != exitInvalid || !strings.Contains(errOut, "MISSING_REPRO_COMMAND") {
			t.Fatalf("want MISSING_REPRO_COMMAND, got code=%d stderr=%q", code, errOut)
		}
	})
}

func TestCheckTemplate(t *testing.T) {
	t.Parallel()

	t.Run("repo implementation-report template passes", func(t *testing.T) {
		t.Parallel()
		root, err := resolveRepoRoot("")
		if err != nil {
			t.Fatal(err)
		}
		code, out, errOut := runCLI("check-template", "--repo-root", root)
		if code != exitOK || !strings.Contains(out, "OK:") {
			t.Fatalf("check-template failed: code=%d stdout=%q stderr=%q", code, out, errOut)
		}
	})

	t.Run("missing template fails", func(t *testing.T) {
		t.Parallel()
		empty := t.TempDir()
		code, _, errOut := runCLI("check-template", "--repo-root", empty)
		if code != exitInvalid || !strings.Contains(errOut, "MISSING_TEMPLATE") {
			t.Fatalf("want MISSING_TEMPLATE, got code=%d stderr=%q", code, errOut)
		}
	})

	t.Run("template without claim evidence table fails", func(t *testing.T) {
		t.Parallel()
		repo := t.TempDir()
		writeFile(t, filepath.Join(repo, "scripts", "agent-harness", "templates", "implementation-report.md.tmpl"), "# 実装報告\n\n## 1. 変更概要\n")
		code, _, errOut := runCLI("check-template", "--repo-root", repo)
		if code != exitInvalid || !strings.Contains(errOut, "MISSING_CLAIM_TABLE") {
			t.Fatalf("want MISSING_CLAIM_TABLE, got code=%d stderr=%q", code, errOut)
		}
	})
}

func TestPlectoWITMatchMismatchAndGuards(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("wasm-tools"); err != nil {
		t.Skip("wasm-tools not installed")
	}

	buildWITComponent := func(t *testing.T, dir, name, witSrc string) string {
		t.Helper()
		witPath := filepath.Join(dir, name+".wit")
		wasmPath := filepath.Join(dir, name+".wasm")
		writeFile(t, witPath, witSrc)
		cmd := exec.CommandContext(context.Background(), "wasm-tools", "component", "wit", witPath, "--wasm", "-o", wasmPath)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("wasm-tools build %s: %v\n%s", name, err, string(out))
		}
		return wasmPath
	}

	dir := t.TempDir()
	beforeWasm := buildWITComponent(t, dir, "before", "package demo:filter@0.1.0;\nworld filter {\n  export ping: func() -> u32;\n}\n")
	afterSame := buildWITComponent(t, dir, "after_same", "package demo:filter@0.1.0;\nworld filter {\n  export ping: func() -> u32;\n}\n")
	afterDiff := buildWITComponent(t, dir, "after_diff", "package demo:filter@0.1.0;\nworld filter {\n  export ping: func() -> u64;\n}\n")

	t.Run("matching WIT passes and writes durable evidence", func(t *testing.T) {
		t.Parallel()
		evDir := filepath.Join(t.TempDir(), "ev-pass")
		code, out, errOut := runCLI("plecto-wit", "--before-wasm", beforeWasm, "--after-wasm", afterSame, "--evidence-dir", evDir, "--format", "json")
		if code != exitOK {
			t.Fatalf("expected exit 0, got %d stdout=%s stderr=%s", code, out, errOut)
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(out), &payload); err != nil {
			t.Fatalf("invalid json: %v\n%s", err, out)
		}
		if payload["wit_equal"] != true {
			t.Fatalf("expected wit_equal=true, got %#v", payload)
		}
		for _, f := range []string{"summary.json", "base.wit", "head.wit", "wit.diff", "command.log"} {
			if _, err := os.Stat(filepath.Join(evDir, f)); err != nil {
				t.Fatalf("missing evidence file %s: %v", f, err)
			}
		}
	})

	t.Run("mismatched WIT fails with PLECTO_WIT_MISMATCH", func(t *testing.T) {
		t.Parallel()
		evDir := filepath.Join(t.TempDir(), "ev-fail")
		code, out, errOut := runCLI("plecto-wit", "--before-wasm", beforeWasm, "--after-wasm", afterDiff, "--evidence-dir", evDir, "--format", "json")
		if code != exitInvalid {
			t.Fatalf("expected exit 1 on WIT mismatch, got %d stdout=%s stderr=%s", code, out, errOut)
		}
		if !strings.Contains(out+errOut, "PLECTO_WIT_MISMATCH") {
			t.Fatalf("expected PLECTO_WIT_MISMATCH in output: stdout=%s stderr=%s", out, errOut)
		}
		diffBytes, err := os.ReadFile(filepath.Join(evDir, "wit.diff"))
		if err != nil || len(bytes.TrimSpace(diffBytes)) == 0 {
			t.Fatalf("expected non-empty wit.diff, err=%v content=%q", err, string(diffBytes))
		}
	})

	t.Run("unsafe --base-commit leading dash rejected", func(t *testing.T) {
		t.Parallel()
		code, _, errOut := runCLI("plecto-wit", "--base-commit=--upload-pack=evil", "--repo-root", t.TempDir())
		if code != exitUsage || !strings.Contains(errOut, "base-commit") {
			t.Fatalf("expected exit 2 for leading dash base-commit, got code=%d stderr=%q", code, errOut)
		}
	})
}

func TestRecapClippyInventoryAndGuards(t *testing.T) {
	t.Parallel()

	makeProject := func(t *testing.T) string {
		t.Helper()
		repo := t.TempDir()
		writeFile(t, filepath.Join(repo, "recap-worker", "rust-toolchain.toml"), "[toolchain]\nchannel = \"1.99.1\"\n")
		writeFile(t, filepath.Join(repo, "recap-worker", "recap-worker", "Cargo.toml"), "[package]\nname = \"demo\"\nversion = \"0.1.0\"\n")
		writeFile(t, filepath.Join(repo, "recap-worker", "recap-worker", "src", "lib.rs"), `#![allow(dead_code)]

#[allow(clippy::too_many_lines, clippy::float_cmp)]
pub fn demo() -> f64 {
    1.0
}
`)
		return repo
	}

	writeCargoStub := func(t *testing.T, script string) string {
		t.Helper()
		p := filepath.Join(t.TempDir(), "cargo-stub")
		if err := fsperm.WriteExecutable(p, []byte("#!/usr/bin/env bash\nset -euo pipefail\n"+script)); err != nil {
			t.Fatal(err)
		}
		return p
	}

	t.Run("zero warnings passes and writes complete allow inventory", func(t *testing.T) {
		t.Parallel()
		repo := makeProject(t)
		evDir := filepath.Join(t.TempDir(), "clippy-ok")
		cargoStub := writeCargoStub(t, `
if [[ "${2:-}" == "--version" ]]; then
  echo "cargo 1.99.1 (stub)"
  exit 0
fi
if [[ "${2:-}" == "clippy" && "${3:-}" == "--version" ]]; then
  echo "clippy 0.1.99 (stub)"
  exit 0
fi
echo '{"reason":"build-finished","success":true}'
exit 0
`)
		var stdout, stderr bytes.Buffer
		code := runRecapClippy(context.Background(), recapClippyOptions{
			repoRoot:    repo,
			projectRel:  "recap-worker/recap-worker",
			evidenceDir: evDir,
			format:      "json",
			cargoBin:    cargoStub,
		}, &stdout, &stderr)
		if code != exitOK {
			t.Fatalf("expected exit 0, got %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
		}
		var res RecapClippyResult
		if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
			t.Fatalf("unmarshal json: %v\n%s", err, stdout.String())
		}
		if res.WarningCount != 0 || res.AllowInventoryTotal != 2 || res.AllowInnerCount != 1 || res.AllowOuterCount != 1 {
			t.Fatalf("unexpected result: %+v", res)
		}
		if res.AllowByRule["dead_code"] != 1 || res.AllowByRule["clippy::too_many_lines"] != 1 || res.AllowByRule["clippy::float_cmp"] != 1 {
			t.Fatalf("unexpected allow_by_rule: %#v", res.AllowByRule)
		}
		for _, f := range []string{"summary.json", "clippy.log", "allow_inventory.txt"} {
			if _, err := os.Stat(filepath.Join(evDir, f)); err != nil {
				t.Fatalf("missing %s: %v", f, err)
			}
		}
	})

	t.Run("compiler warning causes exit 1 even if cargo exits 0", func(t *testing.T) {
		t.Parallel()
		repo := makeProject(t)
		evDir := filepath.Join(t.TempDir(), "clippy-warn")
		cargoStub := writeCargoStub(t, `
if [[ "${2:-}" == "--version" || "${3:-}" == "--version" ]]; then
  echo "stub 1.99.1"
  exit 0
fi
echo '{"reason":"compiler-message","message":{"level":"warning","message":"unused variable"}}'
exit 0
`)
		var stdout, stderr bytes.Buffer
		code := runRecapClippy(context.Background(), recapClippyOptions{
			repoRoot:    repo,
			projectRel:  "recap-worker/recap-worker",
			evidenceDir: evDir,
			format:      "json",
			cargoBin:    cargoStub,
		}, &stdout, &stderr)
		if code != exitInvalid {
			t.Fatalf("expected exit 1 on warning, got %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
		}
	})

	t.Run("project-dir traversal rejected", func(t *testing.T) {
		t.Parallel()
		repo := makeProject(t)
		code, _, errOut := runCLI("recap-clippy", "--repo-root", repo, "--project-dir", "../outside")
		if code != exitUsage && code != exitInvalid {
			t.Fatalf("expected non-zero on traversal, got %d stderr=%q", code, errOut)
		}
		if !strings.Contains(errOut, "UNSAFE_PROJECT_DIR") && !strings.Contains(errOut, "project-dir") {
			t.Fatalf("unexpected stderr=%q", errOut)
		}
	})
}

func TestValidateReportAcceptsSubshellCdCommandAndRejectsBadOCI(t *testing.T) {
	t.Parallel()

	t.Run("subshell cd && cargo command accepted", func(t *testing.T) {
		t.Parallel()
		repo := t.TempDir()
		writeFile(t, filepath.Join(repo, "docs", "review", "ev.md"), "# ev\n")
		report := filepath.Join(repo, "report.md")
		writeFile(t, report, "# Report\n\n| 主張 | 証拠（コマンドと出力の保存先） |\n| --- | --- |\n| recap-worker clippy 警告 0 | コマンド: `(cd recap-worker/recap-worker && cargo +1.99.1 clippy --offline --all-targets -- -D warnings)` / 保存先: `docs/review/ev.md` |\n")

		code, out, errOut := runCLI("validate", report, "--repo-root", repo)
		if code != exitOK {
			t.Fatalf("expected exit 0, got %d stdout=%q stderr=%q", code, out, errOut)
		}
	})

	t.Run("OCI artifact digest traversal rejected", func(t *testing.T) {
		t.Parallel()
		artDir := t.TempDir()
		writeFile(t, filepath.Join(artDir, "index.json"), `{"manifests":[{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"sha256:../../etc/passwd"}]}`)
		var logBuf bytes.Buffer
		if _, _, err := inspectShippedOCIArtifact(context.Background(), "wasm-tools", artDir, "", &logBuf); err == nil {
			t.Fatal("expected error for OCI manifest digest traversal, got nil")
		}
	})
}

func TestLiveCaptureVerify06Evidence(t *testing.T) {
	outDir := strings.TrimSpace(os.Getenv("ALT_VERIFY06_CAPTURE_DIR"))
	if outDir == "" {
		t.Skip("set ALT_VERIFY06_CAPTURE_DIR to run live plecto-wit + recap-clippy evidence capture")
	}
	root, err := resolveRepoRoot("")
	if err != nil {
		t.Fatal(err)
	}
	plectoEv := filepath.Join(outDir, "plecto-wit")
	plectoArgs := []string{"plecto-wit", "--repo-root", root, "--base-commit", defaultPlectoBaseRev, "--evidence-dir", plectoEv, "--format", "json"}
	if artDir := strings.TrimSpace(os.Getenv("ALT_VERIFY06_PLECTO_ARTIFACT_DIR")); artDir != "" {
		plectoArgs = append(plectoArgs, "--artifact-dir", artDir)
	}
	code, out, errOut := runCLI(plectoArgs...)
	if code != exitOK {
		t.Fatalf("plecto-wit failed: code=%d stdout=%s stderr=%s", code, out, errOut)
	}
	t.Logf("PLECTO-WIT JSON:\n%s", out)

	clippyEv := filepath.Join(outDir, "recap-clippy")
	code, out, errOut = runCLI("recap-clippy", "--repo-root", root, "--evidence-dir", clippyEv, "--format", "json")
	if code != exitOK {
		t.Fatalf("recap-clippy failed: code=%d stdout=%s stderr=%s", code, out, errOut)
	}
	t.Logf("RECAP-CLIPPY JSON:\n%s", out)
}

func TestCommittedVerify06EvidenceLedgerValidates(t *testing.T) {
	t.Parallel()
	root, err := resolveRepoRoot("")
	if err != nil {
		t.Fatal(err)
	}
	ledger := filepath.Join(root, "docs", "review", "verify-06-evidence-ledger.md")
	code, out, errOut := runCLI("validate", ledger, "--repo-root", root, "--format", "json")
	if code != exitOK {
		t.Fatalf("committed verify-06-evidence-ledger.md failed validate: code=%d\nstdout=%s\nstderr=%s", code, out, errOut)
	}
	var res ValidateResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("unmarshal validate json: %v\n%s", err, out)
	}
	if !res.Valid || res.ClaimsChecked < 4 || res.EvidencePathsChecked < 4 {
		t.Fatalf("unexpected ledger validation summary: %+v", res)
	}
}

func TestValidateReportRejectsCarbonHighBypassesAndCfgAttrAllows(t *testing.T) {
	t.Parallel()

	t.Run("single script token cannot serve as both command and evidence", func(t *testing.T) {
		t.Parallel()
		repo := t.TempDir()
		writeFile(t, filepath.Join(repo, "scripts", "lint", "adr.sh"), "#!/bin/sh\nexit 0\n")
		report := filepath.Join(repo, "report.md")
		writeFile(t, report, "# R\n\n| 主張 | 証拠 |\n| --- | --- |\n| ADR passes | `./scripts/lint/adr.sh` |\n")
		code, _, errOut := runCLI("validate", report, "--repo-root", repo)
		if code == exitOK {
			t.Fatalf("expected single-token script bypass to fail, got exit 0")
		}
		if !strings.Contains(errOut, "MISSING_EVIDENCE_PATH") && !strings.Contains(errOut, "NON_EVIDENCE_ARTIFACT") {
			t.Fatalf("expected MISSING_EVIDENCE_PATH or NON_EVIDENCE_ARTIFACT, got: %s", errOut)
		}
	})

	t.Run("prose starting with go/make/git does not satisfy command requirement", func(t *testing.T) {
		t.Parallel()
		repo := t.TempDir()
		writeFile(t, filepath.Join(repo, "docs", "review", "ev.log"), "ok\n")
		report := filepath.Join(repo, "report.md")
		writeFile(t, report, "# R\n\n| 主張 | 証拠 |\n| --- | --- |\n| safe | go through logs and check `docs/review/ev.log` |\n")
		code, _, errOut := runCLI("validate", report, "--repo-root", repo)
		if code == exitOK || !strings.Contains(errOut, "MISSING_REPRO_COMMAND") {
			t.Fatalf("expected MISSING_REPRO_COMMAND for prose bypass, got code=%d stderr=%s", code, errOut)
		}
	})

	t.Run("missing markdown anchor and bare self-reference rejected", func(t *testing.T) {
		t.Parallel()
		repo := t.TempDir()
		writeFile(t, filepath.Join(repo, "docs", "review", "notes.md"), "# Real Heading\n\ncontent\n")
		report := filepath.Join(repo, "docs", "review", "rep.md")
		writeFile(t, report, "# Rep\n\n| 主張 | 証拠 |\n| --- | --- |\n| c1 | `go test ./...` / `docs/review/notes.md#nonexistent-999` |\n| c2 | `go test ./...` / `docs/review/rep.md` |\n")
		code, _, errOut := runCLI("validate", report, "--repo-root", repo)
		if code == exitOK || !strings.Contains(errOut, "MISSING_EVIDENCE_ANCHOR") || !strings.Contains(errOut, "SELF_REFERENTIAL_EVIDENCE") {
			t.Fatalf("expected MISSING_EVIDENCE_ANCHOR and SELF_REFERENTIAL_EVIDENCE, got code=%d stderr=%s", code, errOut)
		}
	})

	t.Run("unclosed code fence and reworded blanket claim rejected", func(t *testing.T) {
		t.Parallel()
		repo := t.TempDir()
		writeFile(t, filepath.Join(repo, "docs", "review", "ev.log"), "ok\n")
		report := filepath.Join(repo, "report.md")
		writeFile(t, report, "# R\n\n| 主張 | 証拠 |\n| --- | --- |\n| 全モジュールで実行時挙動への影響なし | `go test ./...` / `docs/review/ev.log` |\n\n```text\nunclosed fence hides trailing text\n")
		code, _, errOut := runCLI("validate", report, "--repo-root", repo)
		if code == exitOK || !strings.Contains(errOut, "UNCLOSED_CODE_FENCE") || !strings.Contains(errOut, "BLANKET_UNBACKED_CLAIM") {
			t.Fatalf("expected UNCLOSED_CODE_FENCE and BLANKET_UNBACKED_CLAIM, got code=%d stderr=%s", code, errOut)
		}
	})

	t.Run("inventoryRustAllows captures cfg_attr allows", func(t *testing.T) {
		t.Parallel()
		proj := t.TempDir()
		writeFile(t, filepath.Join(proj, "src", "lib.rs"), "#[cfg_attr(not(test), allow(dead_code))]\nfn helper() {}\n#![cfg_attr(test, allow(unused_imports))]\n")
		entries, _, err := inventoryRustAllows(proj)
		if err != nil {
			t.Fatalf("inventoryRustAllows: %v", err)
		}
		if len(entries) != 2 {
			t.Fatalf("expected 2 cfg_attr allow entries, got %d: %+v", len(entries), entries)
		}
	})
}
