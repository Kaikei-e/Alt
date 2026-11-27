package verifydiff_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/quick"

	"alt/scripts/altharness/internal/fsperm"
	"alt/scripts/altharness/internal/guards"
	"alt/scripts/altharness/internal/verifydiff"
)

const testScaffoldMarker = "# VERIFY-02-" + "TEMP-OLD-IMPL"

func initTempGitRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runGitCmd(t, repo, "init", "-b", "main")
	runGitCmd(t, repo, "config", "user.email", "verify02-test@example.com")
	runGitCmd(t, repo, "config", "user.name", "Verify02 Test")
	return repo
}

func runGitCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
	}
	return strings.TrimSpace(string(out))
}

func writeRepoFile(t *testing.T, repo, rel, content string) string {
	t.Helper()
	full := filepath.Join(repo, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), fsperm.Dir); err != nil {
		t.Fatalf("mkdir %s: %v", full, err)
	}
	if err := os.WriteFile(full, []byte(content), fsperm.File); err != nil {
		t.Fatalf("write %s: %v", full, err)
	}
	return full
}

func writeDiffRunnerScript(t *testing.T, dir, name string) string {
	t.Helper()
	// Pure Python stdlib differential runner comparing old vs new compute(x, y) over seeded trials.
	script := `#!/usr/bin/env python3
import importlib.util
import os
import sys

def load_module(mod_name, path):
    spec = importlib.util.spec_from_file_location(mod_name, path)
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod

old_path = os.environ["ALT_DIFF_OLD_FILE"]
new_path = os.environ["ALT_DIFF_NEW_FILE"]
seed = int(os.environ["ALT_DIFF_SEED"])
tmpdir = os.environ["ALT_DIFF_TMPDIR"]
assert os.path.isdir(tmpdir), "ALT_DIFF_TMPDIR missing"
assert os.path.isfile(old_path), "ALT_DIFF_OLD_FILE missing"
assert os.path.isfile(new_path), "ALT_DIFF_NEW_FILE missing"

old_mod = load_module("old_impl", old_path)
new_mod = load_module("new_impl", new_path)

state = seed & 0xFFFFFFFFFFFFFFFF
for trial in range(50):
    state = (state * 6364136223846793005 + 1442695040888963407) & 0xFFFFFFFFFFFFFFFF
    a = (state >> 16) % 200 - 100
    state = (state * 6364136223846793005 + 1442695040888963407) & 0xFFFFFFFFFFFFFFFF
    b = (state >> 16) % 200 - 100
    old_val = old_mod.compute(a, b)
    new_val = new_mod.compute(a, b)
    if old_val != new_val:
        print(f"DIVERGENCE trial={trial} a={a} b={b} old={old_val} new={new_val}", file=sys.stderr)
        sys.exit(1)

print(f"EQUIVALENT trials=50 seed={seed} tmpdir={tmpdir}")
`
	path := filepath.Join(dir, name)
	if err := fsperm.WriteExecutable(path, []byte(script)); err != nil {
		t.Fatalf("write executable runner: %v", err)
	}
	return path
}

func TestRunEquivalentSplitPassesAndRecordsEvidenceAndDecision(t *testing.T) {
	t.Parallel()
	repo := initTempGitRepo(t)
	writeRepoFile(t, repo, "pkg/calc.py", "def compute(a, b):\n    return (a * a) + (3 * b) - 7\n")
	runGitCmd(t, repo, "add", "pkg/calc.py")
	runGitCmd(t, repo, "commit", "-m", "pre-split")
	baseSHA := runGitCmd(t, repo, "rev-parse", "HEAD")

	// Behavior-preserving split into helper functions.
	writeRepoFile(t, repo, "pkg/calc.py",
		"def _sq(x):\n    return x * x\n\n"+
			"def _lin(y):\n    return 3 * y - 7\n\n"+
			"def compute(a, b):\n    return _sq(a) + _lin(b)\n",
	)
	runGitCmd(t, repo, "add", "pkg/calc.py")
	runGitCmd(t, repo, "commit", "-m", "split calc")

	runnerDir := t.TempDir()
	runner := writeDiffRunnerScript(t, runnerDir, "diff_runner.py")

	var stdout, stderr bytes.Buffer
	code := verifydiff.Run(context.Background(), []string{
		"run",
		"--repo-root", repo,
		"--base-commit", "HEAD~1",
		"--file", "pkg/calc.py",
		"--runner-cmd", "python3 " + runner,
		"--seed", "20261127",
		"--label", "calc-split",
		"--format", "json",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d\nstdout=%s\nstderr=%s", code, stdout.String(), stderr.String())
	}

	var report verifydiff.Evidence
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("invalid json output: %v\nraw=%s", err, stdout.String())
	}
	if report.BaseCommit != baseSHA {
		t.Fatalf("base_commit=%q, want %q", report.BaseCommit, baseSHA)
	}
	if report.File != "pkg/calc.py" || report.Seed != 20261127 || report.ExitCode != 0 {
		t.Fatalf("unexpected report: %+v", report)
	}
	if len(report.OutputSHA256) != 64 {
		t.Fatalf("expected 64-char sha256, got %q", report.OutputSHA256)
	}
	if !strings.Contains(report.StdoutExcerpt, "EQUIVALENT trials=50 seed=20261127") {
		t.Fatalf("unexpected stdout_excerpt: %q", report.StdoutExcerpt)
	}

	// Verify temporary extraction directory was cleaned up via defer.
	for _, field := range strings.Fields(report.StdoutExcerpt) {
		if strings.HasPrefix(field, "tmpdir=") {
			tmpPath := strings.TrimPrefix(field, "tmpdir=")
			if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
				t.Fatalf("expected temp dir %s to be removed after run", tmpPath)
			}
		}
	}

	// Verify evidence file written on disk.
	evPath := filepath.Join(repo, ".harness", "evidence", "verify-02", "calc-split.json")
	evRaw, err := os.ReadFile(evPath)
	if err != nil {
		t.Fatalf("missing evidence file %s: %v", evPath, err)
	}
	var diskEv verifydiff.Evidence
	if err := json.Unmarshal(evRaw, &diskEv); err != nil {
		t.Fatalf("corrupt evidence JSON: %v", err)
	}
	if diskEv.OutputSHA256 != report.OutputSHA256 {
		t.Fatalf("disk evidence sha256 %q != stdout %q", diskEv.OutputSHA256, report.OutputSHA256)
	}

	// Verify LOOP-03 decisions.tsv row.
	decisionsPath := filepath.Join(repo, ".harness", "runs", "main", "decisions.tsv")
	rawTSV, err := os.ReadFile(decisionsPath)
	if err != nil {
		t.Fatalf("missing decisions.tsv at %s: %v", decisionsPath, err)
	}
	lines := strings.Split(strings.TrimSpace(string(rawTSV)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected header + 1 row in decisions.tsv, got %d lines:\n%s", len(lines), string(rawTSV))
	}
	if lines[0] != "ts\tphase\tdecision\twhy\tevidence\tresult" {
		t.Fatalf("unexpected header: %q", lines[0])
	}
	cols := strings.Split(lines[1], "\t")
	if len(cols) != 6 {
		t.Fatalf("expected 6 columns, got %d: %q", len(cols), lines[1])
	}
	if cols[1] != "verify-02" {
		t.Fatalf("phase=%q, want verify-02", cols[1])
	}
	if cols[4] != ".harness/evidence/verify-02/calc-split.json" {
		t.Fatalf("evidence pointer=%q, want .harness/evidence/verify-02/calc-split.json", cols[4])
	}
	if !guards.IsValidEvidencePointer(cols[4]) {
		t.Fatalf("evidence pointer %q rejected by guards.IsValidEvidencePointer", cols[4])
	}
	if cols[5] != "pass" {
		t.Fatalf("result=%q, want pass", cols[5])
	}
}

func TestRunBrokenSplitFailsAndRecordsFailDecision(t *testing.T) {
	t.Parallel()
	repo := initTempGitRepo(t)
	writeRepoFile(t, repo, "pkg/calc.py", "def compute(a, b):\n    return (a * a) + (3 * b) - 7\n")
	runGitCmd(t, repo, "add", "pkg/calc.py")
	runGitCmd(t, repo, "commit", "-m", "pre-split")

	// Intentionally broken post-split implementation (+7 instead of -7).
	writeRepoFile(t, repo, "pkg/calc.py",
		"def _sq(x):\n    return x * x\n\n"+
			"def _lin(y):\n    return 3 * y + 7\n\n"+
			"def compute(a, b):\n    return _sq(a) + _lin(b)\n",
	)
	runGitCmd(t, repo, "add", "pkg/calc.py")
	runGitCmd(t, repo, "commit", "-m", "broken split")

	runnerDir := t.TempDir()
	runner := writeDiffRunnerScript(t, runnerDir, "diff_runner.py")

	var stdout, stderr bytes.Buffer
	code := verifydiff.Run(context.Background(), []string{
		"run",
		"--repo-root", repo,
		"--base-commit", "HEAD~1",
		"--file", "pkg/calc.py",
		"--runner-cmd", "python3 " + runner,
		"--seed", "42",
		"--label", "calc-broken",
		"--format", "json",
	}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("expected non-zero exit code for broken split, got 0\nstdout=%s", stdout.String())
	}

	evPath := filepath.Join(repo, ".harness", "evidence", "verify-02", "calc-broken.json")
	evRaw, err := os.ReadFile(evPath)
	if err != nil {
		t.Fatalf("missing evidence JSON for failed run: %v", err)
	}
	var diskEv verifydiff.Evidence
	if err := json.Unmarshal(evRaw, &diskEv); err != nil {
		t.Fatalf("parse evidence JSON: %v", err)
	}
	if diskEv.ExitCode == 0 {
		t.Fatalf("expected non-zero exit_code in evidence JSON, got 0")
	}

	decisionsPath := filepath.Join(repo, ".harness", "runs", "main", "decisions.tsv")
	rawTSV, err := os.ReadFile(decisionsPath)
	if err != nil {
		t.Fatalf("missing decisions.tsv: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(rawTSV)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	cols := strings.Split(lines[1], "\t")
	if cols[5] != "fail" {
		t.Fatalf("expected result=fail in decisions.tsv, got %q", cols[5])
	}
}

func TestRunRejectsPathTraversalAndSymlinkEscape(t *testing.T) {
	t.Parallel()
	repo := initTempGitRepo(t)
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "secret.py")
	if err := os.WriteFile(outsideFile, []byte("x = 1\n"), fsperm.File); err != nil {
		t.Fatalf("write outside file: %v", err)
	}

	writeRepoFile(t, repo, "pkg/ok.py", "x = 1\n")
	linkPath := filepath.Join(repo, "pkg", "escape_link.py")
	if err := os.Symlink(outsideFile, linkPath); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	runGitCmd(t, repo, "add", "pkg/ok.py", "pkg/escape_link.py")
	runGitCmd(t, repo, "commit", "-m", "initial")

	cases := []struct {
		name string
		args []string
	}{
		{
			name: "dotdot traversal in --file",
			args: []string{"run", "--repo-root", repo, "--base-commit", "HEAD", "--file", "../outside.py", "--runner-cmd", "true", "--seed", "1"},
		},
		{
			name: "nested dotdot traversal in --file",
			args: []string{"run", "--repo-root", repo, "--base-commit", "HEAD", "--file", "pkg/../../outside.py", "--runner-cmd", "true", "--seed", "1"},
		},
		{
			name: "absolute --file",
			args: []string{"run", "--repo-root", repo, "--base-commit", "HEAD", "--file", outsideFile, "--runner-cmd", "true", "--seed", "1"},
		},
		{
			name: "symlink escaping repo root",
			args: []string{"run", "--repo-root", repo, "--base-commit", "HEAD", "--file", "pkg/escape_link.py", "--runner-cmd", "true", "--seed", "1"},
		},
		{
			name: "flag injection in --base-commit",
			args: []string{"run", "--repo-root", repo, "--base-commit", "--upload-pack=evil", "--file", "pkg/ok.py", "--runner-cmd", "true", "--seed", "1"},
		},
		{
			name: "traversal in --label",
			args: []string{"run", "--repo-root", repo, "--base-commit", "HEAD", "--file", "pkg/ok.py", "--runner-cmd", "true", "--seed", "1", "--label", "../../evil"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			code := verifydiff.Run(context.Background(), tc.args, &stdout, &stderr)
			if code == 0 {
				t.Fatalf("expected failure for %s, got exit 0 (stdout=%s)", tc.name, stdout.String())
			}
		})
	}
}

func TestCheckCleanDetectsLeftoverTempScaffolds(t *testing.T) {
	t.Parallel()

	t.Run("clean repo passes", func(t *testing.T) {
		t.Parallel()
		repo := initTempGitRepo(t)
		writeRepoFile(t, repo, "pkg/calc.py", "def compute(a, b):\n    return a + b\n")
		runGitCmd(t, repo, "add", "pkg/calc.py")
		runGitCmd(t, repo, "commit", "-m", "clean")

		var stdout, stderr bytes.Buffer
		code := verifydiff.Run(context.Background(), []string{"check-clean", "--repo-root", repo}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("expected clean repo to pass, got %d\nstdout=%s\nstderr=%s", code, stdout.String(), stderr.String())
		}
	})

	cases := []struct {
		name    string
		rel     string
		content string
	}{
		{
			name:    "old_scaffold filename",
			rel:     "tests/train_old_scaffold.py",
			content: "def old_main(): pass\n",
		},
		{
			name:    "verify02_old extension",
			rel:     "tests/train.verify02_old.py",
			content: "def old_main(): pass\n",
		},
		{
			name:    "temp marker comment in content",
			rel:     "tests/temp_copy.py",
			content: testScaffoldMarker + "\ndef old_main(): pass\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := initTempGitRepo(t)
			writeRepoFile(t, repo, tc.rel, tc.content)
			runGitCmd(t, repo, "add", tc.rel)
			runGitCmd(t, repo, "commit", "-m", "add leftover scaffold")

			var stdout, stderr bytes.Buffer
			code := verifydiff.Run(context.Background(), []string{"check-clean", "--repo-root", repo}, &stdout, &stderr)
			if code == 0 {
				t.Fatalf("expected check-clean to fail for %s, got exit 0", tc.name)
			}
			combined := stdout.String() + stderr.String()
			if !strings.Contains(combined, tc.rel) {
				t.Fatalf("expected output to name %q, got:\n%s", tc.rel, combined)
			}
		})
	}
}

// TestQuickPropertySanitizeLabelAndSplitEquivalence demonstrates Go stdlib testing/quick
// property-based verification for VERIFY-02:
//  1. SanitizeLabel is idempotent, non-empty, and never emits path separators or "..".
//  2. A monolithic vs split pure Go scoring helper produces identical outputs across
//     randomized quick.Check trials.
func TestQuickPropertySanitizeLabelAndSplitEquivalence(t *testing.T) {
	t.Parallel()

	t.Run("SanitizeLabel invariants", func(t *testing.T) {
		t.Parallel()
		prop := func(raw string) bool {
			got := verifydiff.SanitizeLabel(raw)
			if got == "" {
				return false
			}
			if strings.Contains(got, "/") || strings.Contains(got, "\\") || strings.Contains(got, "..") {
				return false
			}
			// Idempotence: sanitizing an already sanitized label yields the same label.
			return verifydiff.SanitizeLabel(got) == got
		}
		if err := quick.Check(prop, &quick.Config{MaxCount: 200}); err != nil {
			t.Fatalf("SanitizeLabel property failed: %v", err)
		}
	})

	t.Run("Monolithic vs split ClampRank differential property", func(t *testing.T) {
		t.Parallel()
		// Pre-split monolithic function vs post-split staged helpers.
		monolithic := func(requested, nSamples, nFeatures int16) (int, bool) {
			req, ns, nf := int(requested), int(nSamples), int(nFeatures)
			if nf < 2 || req < 1 || ns < 1 {
				return 0, false
			}
			capVal := ns
			if nf < capVal {
				capVal = nf
			}
			if req < capVal {
				return req, true
			}
			return capVal, true
		}

		validateInputs := func(req, ns, nf int) bool { return nf >= 2 && req >= 1 && ns >= 1 }
		matrixRankCap := func(ns, nf int) int {
			if nf < ns {
				return nf
			}
			return ns
		}
		split := func(requested, nSamples, nFeatures int16) (int, bool) {
			req, ns, nf := int(requested), int(nSamples), int(nFeatures)
			if !validateInputs(req, ns, nf) {
				return 0, false
			}
			capVal := matrixRankCap(ns, nf)
			if req < capVal {
				return req, true
			}
			return capVal, true
		}

		prop := func(requested, nSamples, nFeatures int16) bool {
			v1, ok1 := monolithic(requested, nSamples, nFeatures)
			v2, ok2 := split(requested, nSamples, nFeatures)
			if ok1 != ok2 || v1 != v2 {
				return false
			}
			if ok2 && (v2 < 1 || v2 > int(nSamples) || v2 > int(nFeatures) || v2 > int(requested)) {
				return false
			}
			return true
		}
		if err := quick.Check(prop, &quick.Config{MaxCount: 500}); err != nil {
			t.Fatalf("differential quick.Check failed: %v", err)
		}
	})
}

func TestRunCustomOutputPathsAndSequentialAppend(t *testing.T) {
	t.Parallel()
	repo := initTempGitRepo(t)
	writeRepoFile(t, repo, "pkg/calc.py", "def compute(a, b):\n    return a + b\n")
	runGitCmd(t, repo, "add", "pkg/calc.py")
	runGitCmd(t, repo, "commit", "-m", "initial")

	runnerDir := t.TempDir()
	runner := writeDiffRunnerScript(t, runnerDir, "diff_runner.py")

	for _, seed := range []string{"101", "202"} {
		var stdout, stderr bytes.Buffer
		code := verifydiff.Run(context.Background(), []string{
			"run",
			"--repo-root", repo,
			"--base-commit", "HEAD",
			"--file", "pkg/calc.py",
			"--runner-cmd", "python3 " + runner,
			"--seed", seed,
			"--evidence-dir", ".harness/custom-ev",
			"--decisions-file", ".harness/custom-runs/decisions.tsv",
			"--label", "seed-" + seed,
			"--format", "text",
		}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("seed %s failed: %d\nstdout=%s\nstderr=%s", seed, code, stdout.String(), stderr.String())
		}
		if !strings.Contains(stdout.String(), "VERIFY-02 PASS") {
			t.Fatalf("expected text PASS line, got %q", stdout.String())
		}
	}

	rawTSV, err := os.ReadFile(filepath.Join(repo, ".harness", "custom-runs", "decisions.tsv"))
	if err != nil {
		t.Fatalf("read custom decisions.tsv: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(rawTSV)), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected header + 2 rows, got %d:\n%s", len(lines), string(rawTSV))
	}
}

func TestCheckCleanJSONAndHelpUsage(t *testing.T) {
	t.Parallel()
	repo := initTempGitRepo(t)
	writeRepoFile(t, repo, "pkg/ok.py", "x = 1\n")
	runGitCmd(t, repo, "add", "pkg/ok.py")
	runGitCmd(t, repo, "commit", "-m", "ok")

	var stdout, stderr bytes.Buffer
	code := verifydiff.Run(context.Background(), []string{"check-clean", "--repo-root", repo, "--format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, stderr.String())
	}
	var report verifydiff.CleanReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if !report.Clean || report.TrackedCount != 1 || len(report.Findings) != 0 {
		t.Fatalf("unexpected report: %+v", report)
	}

	stdout.Reset()
	stderr.Reset()
	if code := verifydiff.Run(context.Background(), []string{"--help"}, &stdout, &stderr); code != 0 {
		t.Fatalf("--help exit=%d, want 0", code)
	}
	if code := verifydiff.Run(context.Background(), []string{"unknown"}, &stdout, &stderr); code != 2 {
		t.Fatalf("unknown subcommand exit=%d, want 2", code)
	}
}
