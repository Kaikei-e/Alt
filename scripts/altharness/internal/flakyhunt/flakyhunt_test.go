package flakyhunt

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"alt/scripts/altharness/internal/fsperm"
)

func pytestCmdForTest(t *testing.T) string {
	t.Helper()
	if p, err := exec.LookPath("pytest"); err == nil {
		return p
	}
	if py, err := exec.LookPath("python3"); err == nil {
		if err := exec.CommandContext(t.Context(), py, "-m", "pytest", "--version").Run(); err == nil {
			return py + " -m pytest"
		}
	}
	t.Skip("neither pytest nor python3 -m pytest found on PATH")
	return ""
}

func makeTempRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "scripts", "altharness"), fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	if err := fsperm.WriteExecutable(filepath.Join(root, "scripts", "altharness", "alt-harness"), []byte("#!/bin/sh\nexit 0\n")); err != nil {
		t.Fatal(err)
	}
	return root
}

func writeGoModule(t *testing.T, repoRoot, relModule, modName, testSrc string) string {
	t.Helper()
	modDir := filepath.Join(repoRoot, filepath.FromSlash(relModule))
	if err := os.MkdirAll(modDir, fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	goMod := "module " + modName + "\n\ngo 1.24.0\n"
	if err := os.WriteFile(filepath.Join(modDir, "go.mod"), []byte(goMod), fsperm.File); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modDir, "sample_test.go"), []byte(testSrc), fsperm.File); err != nil {
		t.Fatal(err)
	}
	return modDir
}

func writePythonModule(t *testing.T, repoRoot, relModule, testSrc string) string {
	t.Helper()
	modDir := filepath.Join(repoRoot, filepath.FromSlash(relModule))
	testsDir := filepath.Join(modDir, "tests")
	if err := os.MkdirAll(testsDir, fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	pyproject := "[project]\nname = \"flaky-fixture\"\nversion = \"0.1.0\"\nrequires-python = \">=3.11\"\n"
	if err := os.WriteFile(filepath.Join(modDir, "pyproject.toml"), []byte(pyproject), fsperm.File); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(testsDir, "test_sample.py"), []byte(testSrc), fsperm.File); err != nil {
		t.Fatal(err)
	}
	return modDir
}

func TestValidationAndPathSecurity(t *testing.T) {
	t.Parallel()
	repo := makeTempRepo(t)
	writeGoModule(t, repo, "svc/app", "example.com/svc", "package svc\nimport \"testing\"\nfunc TestOK(t *testing.T) {}\n")

	// Create an outside directory and a symlink inside repo pointing to it (CWE-59).
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "go.mod"), []byte("module outside\n\ngo 1.24.0\n"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	escapeLink := filepath.Join(repo, "escape-link")
	if err := os.Symlink(outside, escapeLink); err != nil {
		t.Fatal(err)
	}

	// Directory with neither go.mod nor pyproject.toml for auto-detection error.
	unknownDir := filepath.Join(repo, "empty-dir")
	if err := os.MkdirAll(unknownDir, fsperm.Dir); err != nil {
		t.Fatal(err)
	}

	var helpOut, helpErr bytes.Buffer
	if rc := Run(context.Background(), []string{"--help"}, &helpOut, &helpErr); rc != 0 || !strings.Contains(helpOut.String(), "flaky-hunt") {
		t.Fatalf("--help rc=%d stdout=%q stderr=%q", rc, helpOut.String(), helpErr.String())
	}

	cases := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "missing module",
			args:    []string{"--repo-root", repo},
			wantErr: "module",
		},
		{
			name:    "dotdot traversal",
			args:    []string{"../outside", "--repo-root", repo},
			wantErr: "traversal",
		},
		{
			name:    "nested dotdot traversal",
			args:    []string{"svc/../../outside", "--repo-root", repo},
			wantErr: "traversal",
		},
		{
			name:    "absolute module path rejected",
			args:    []string{outside, "--repo-root", repo},
			wantErr: "relative",
		},
		{
			name:    "symlink escape rejected",
			args:    []string{"escape-link", "--repo-root", repo},
			wantErr: "escape",
		},
		{
			name:    "missing directory",
			args:    []string{"does/not/exist", "--repo-root", repo},
			wantErr: "does not exist",
		},
		{
			name:    "undetectable language",
			args:    []string{"empty-dir", "--repo-root", repo},
			wantErr: "detect",
		},
		{
			name:    "invalid rounds",
			args:    []string{"svc/app", "--repo-root", repo, "--rounds", "0"},
			wantErr: "--rounds",
		},
		{
			name:    "invalid count",
			args:    []string{"svc/app", "--repo-root", repo, "--count", "0"},
			wantErr: "--count",
		},
		{
			name:    "invalid lang",
			args:    []string{"svc/app", "--repo-root", repo, "--lang", "ruby"},
			wantErr: "--lang",
		},
		{
			name:    "invalid seed",
			args:    []string{"svc/app", "--repo-root", repo, "--seed", "not-a-seed"},
			wantErr: "--seed",
		},
		{
			name:    "invalid format",
			args:    []string{"svc/app", "--repo-root", repo, "--format", "xml"},
			wantErr: "--format",
		},
		{
			name:    "invalid stress-procs",
			args:    []string{"svc/app", "--repo-root", repo, "--stress-procs", "-1"},
			wantErr: "--stress-procs",
		},
		{
			name:    "unknown flag",
			args:    []string{"svc/app", "--repo-root", repo, "--bogus"},
			wantErr: "unknown",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var out, errb bytes.Buffer
			rc := Run(context.Background(), tc.args, &out, &errb)
			if rc != 2 {
				t.Fatalf("rc=%d want 2 stdout=%q stderr=%q", rc, out.String(), errb.String())
			}
			if !strings.Contains(strings.ToLower(errb.String()), strings.ToLower(tc.wantErr)) {
				t.Fatalf("stderr=%q want substring %q", errb.String(), tc.wantErr)
			}
		})
	}
}

func TestGoCleanSuiteCapturesSeedsAndWritesEvidence(t *testing.T) {
	t.Parallel()
	repo := makeTempRepo(t)
	writeGoModule(t, repo, "clean-svc/app", "example.com/cleansvc", `package cleansvc

import (
	"runtime"
	"testing"
)

func TestAlpha(t *testing.T) {
	if got := runtime.GOMAXPROCS(0); got != 2 {
		t.Fatalf("GOMAXPROCS=%d want 2", got)
	}
	// Logging a line containing "panic:" in a passing test must not false-positive.
	t.Log("panic: harmless log line in passing test")
}

func TestBeta(t *testing.T) {}
`)

	var stdout, stderr bytes.Buffer
	rc := Run(context.Background(), []string{
		"clean-svc/app",
		"--repo-root", repo,
		"--lang", "auto",
		"--rounds", "2",
		"--count", "1",
		"--race=false",
		"--stress-procs", "2",
		"--format", "json",
	}, &stdout, &stderr)
	if rc != 0 {
		t.Fatalf("rc=%d want 0 stdout=%q stderr=%q", rc, stdout.String(), stderr.String())
	}

	var rep Report
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatalf("invalid json stdout=%q err=%v", stdout.String(), err)
	}
	if rep.Module != "clean-svc/app" || rep.Lang != "go" || rep.Rounds != 2 || rep.Status != "PASS" {
		t.Fatalf("unexpected report header: %+v", rep)
	}
	if rep.PassedRounds != 2 || rep.FailedRounds != 0 || len(rep.Failures) != 0 {
		t.Fatalf("unexpected round/failure counts: %+v", rep)
	}
	if len(rep.RoundResults) != 2 {
		t.Fatalf("round_results len=%d want 2", len(rep.RoundResults))
	}
	for _, rr := range rep.RoundResults {
		if !rr.Passed || len(rr.PackageSeeds) == 0 {
			t.Fatalf("round %d expected passed with captured package seeds: %+v", rr.Round, rr)
		}
		for pkg, seed := range rr.PackageSeeds {
			if _, err := strconv.ParseInt(seed, 10, 64); err != nil {
				t.Fatalf("round %d pkg %s invalid seed %q: %v", rr.Round, pkg, seed, err)
			}
		}
	}

	evidenceReport := filepath.Join(repo, ".harness", "evidence", "flaky-hunt", "clean-svc__app", "report.json")
	if _, err := os.Stat(evidenceReport); err != nil {
		t.Fatalf("expected evidence report at %s: %v", evidenceReport, err)
	}
}

func TestGoSyntheticFlakyOrderCapturesSeedAndReproducesDeterministically(t *testing.T) {
	t.Parallel()
	repo := makeTempRepo(t)
	writeGoModule(t, repo, "flaky-go/app", "example.com/flakygo", `package flakygo

import "testing"

var poisoned bool

func TestPoison(t *testing.T) {
	poisoned = true
}

func TestVictim(t *testing.T) {
	if poisoned {
		t.Fatal("order-dependent flake triggered: TestPoison ran before TestVictim")
	}
}

func TestPadding1(t *testing.T) {}
func TestPadding2(t *testing.T) {}
`)

	var stdout, stderr bytes.Buffer
	rc := Run(context.Background(), []string{
		"flaky-go/app",
		"--repo-root", repo,
		"--rounds", "16",
		"--count", "1",
		"--race=false",
		"--format", "json",
	}, &stdout, &stderr)
	if rc != 1 {
		t.Fatalf("expected synthetic flaky suite to fail at least once in 16 rounds, rc=%d stdout=%q stderr=%q", rc, stdout.String(), stderr.String())
	}

	var rep Report
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatalf("invalid json: %v\nstdout=%s", err, stdout.String())
	}
	if rep.Status != "FAIL" || rep.FailedRounds == 0 || len(rep.Failures) == 0 {
		t.Fatalf("expected failures in report: %+v", rep)
	}

	var victimFailure *Failure
	for i := range rep.Failures {
		if rep.Failures[i].TestName == "TestVictim" {
			victimFailure = &rep.Failures[i]
			break
		}
	}
	if victimFailure == nil {
		t.Fatalf("expected TestVictim in failures, got: %+v", rep.Failures)
	}
	if _, err := strconv.ParseInt(victimFailure.Seed, 10, 64); err != nil {
		t.Fatalf("expected numeric shuffle seed, got %q", victimFailure.Seed)
	}
	if !strings.Contains(victimFailure.ReproCommand, "-shuffle="+victimFailure.Seed) {
		t.Fatalf("repro_command missing captured seed: %q", victimFailure.ReproCommand)
	}
	if victimFailure.LogPath == "" {
		t.Fatalf("expected non-empty log_path in failure: %+v", victimFailure)
	}
	logBytes, err := os.ReadFile(victimFailure.LogPath)
	if err != nil || !strings.Contains(string(logBytes), "order-dependent flake triggered") {
		t.Fatalf("failure log missing expected output: err=%v content=%q", err, string(logBytes))
	}

	// Deterministic reproduction: pinning the captured seed must fail 2/2 rounds.
	var reproOut, reproErr bytes.Buffer
	reproRC := Run(context.Background(), []string{
		"flaky-go/app",
		"--repo-root", repo,
		"--rounds", "2",
		"--count", "1",
		"--seed", victimFailure.Seed,
		"--race=false",
		"--format", "json",
	}, &reproOut, &reproErr)
	if reproRC != 1 {
		t.Fatalf("repro with seed=%s rc=%d want 1 stdout=%q stderr=%q", victimFailure.Seed, reproRC, reproOut.String(), reproErr.String())
	}
	var reproRep Report
	if err := json.Unmarshal(reproOut.Bytes(), &reproRep); err != nil {
		t.Fatalf("repro invalid json: %v", err)
	}
	if reproRep.FailedRounds != 2 || reproRep.PassedRounds != 0 {
		t.Fatalf("expected deterministic 2/2 failures for seed %s, got passed=%d failed=%d",
			victimFailure.Seed, reproRep.PassedRounds, reproRep.FailedRounds)
	}
}

func TestGoDetectsDataRaceAndPanic(t *testing.T) {
	t.Parallel()

	t.Run("data race", func(t *testing.T) {
		t.Parallel()
		repo := makeTempRepo(t)
		writeGoModule(t, repo, "race-go/app", "example.com/racego", `package racego

import (
	"sync"
	"testing"
)

func TestDataRace(t *testing.T) {
	shared := 0
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		shared = 1
	}()
	go func() {
		defer wg.Done()
		shared = 2
	}()
	wg.Wait()
	_ = shared
}
`)
		var stdout, stderr bytes.Buffer
		rc := Run(context.Background(), []string{
			"race-go/app",
			"--repo-root", repo,
			"--rounds", "1",
			"--race=true",
			"--format", "json",
		}, &stdout, &stderr)
		if rc != 1 {
			t.Fatalf("expected race failure rc=1, got %d stdout=%q stderr=%q", rc, stdout.String(), stderr.String())
		}
		var rep Report
		if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
			t.Fatalf("invalid json: %v", err)
		}
		foundRace := false
		for _, f := range rep.Failures {
			if f.Kind == "data_race" && f.TestName == "TestDataRace" && f.Seed != "" {
				foundRace = true
			}
		}
		if !foundRace {
			t.Fatalf("expected data_race failure for TestDataRace, got %+v", rep.Failures)
		}
	})

	t.Run("panic", func(t *testing.T) {
		t.Parallel()
		repo := makeTempRepo(t)
		writeGoModule(t, repo, "panic-go/app", "example.com/panicgo", `package panicgo

import "testing"

func TestBoom(t *testing.T) {
	panic("synthetic boom")
}
`)
		var stdout, stderr bytes.Buffer
		rc := Run(context.Background(), []string{
			"panic-go/app",
			"--repo-root", repo,
			"--rounds", "1",
			"--race=false",
			"--format", "text",
		}, &stdout, &stderr)
		if rc != 1 {
			t.Fatalf("expected panic failure rc=1, got %d stdout=%q stderr=%q", rc, stdout.String(), stderr.String())
		}
		out := stdout.String()
		if !strings.Contains(out, "TestBoom") || !strings.Contains(out, "panic") || !strings.Contains(out, "-shuffle=") {
			t.Fatalf("text output missing panic details: %q", out)
		}
	})
}

func TestPythonSyntheticFlakyOrderCapturesSeedAndReproducesDeterministically(t *testing.T) {
	t.Parallel()
	pytestCmd := pytestCmdForTest(t)
	repo := makeTempRepo(t)
	writePythonModule(t, repo, "flaky-py/app", `POISONED = False


def test_poison() -> None:
    global POISONED
    POISONED = True


def test_victim() -> None:
    assert not POISONED, "order-dependent python flake triggered"


def test_padding_1() -> None:
    assert True


def test_padding_2() -> None:
    assert True
`)

	var stdout, stderr bytes.Buffer
	rc := Run(context.Background(), []string{
		"flaky-py/app",
		"--repo-root", repo,
		"--lang", "auto",
		"--rounds", "16",
		"--pytest-cmd", pytestCmd,
		"--format", "json",
	}, &stdout, &stderr)
	if rc != 1 {
		t.Fatalf("expected synthetic python flaky suite to fail at least once in 16 rounds, rc=%d stdout=%q stderr=%q", rc, stdout.String(), stderr.String())
	}

	var rep Report
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatalf("invalid json: %v\nstdout=%s", err, stdout.String())
	}
	if rep.Lang != "python" || rep.Status != "FAIL" || len(rep.Failures) == 0 {
		t.Fatalf("unexpected python report: %+v", rep)
	}

	var victimFailure *Failure
	for i := range rep.Failures {
		if strings.Contains(rep.Failures[i].TestName, "test_victim") {
			victimFailure = &rep.Failures[i]
			break
		}
	}
	if victimFailure == nil {
		t.Fatalf("expected test_victim in failures, got: %+v", rep.Failures)
	}
	if _, err := strconv.ParseInt(victimFailure.Seed, 10, 64); err != nil {
		t.Fatalf("expected numeric seed, got %q", victimFailure.Seed)
	}
	if !strings.Contains(victimFailure.ReproCommand, "--seed "+victimFailure.Seed) {
		t.Fatalf("repro_command missing captured seed: %q", victimFailure.ReproCommand)
	}
	if victimFailure.LogPath == "" {
		t.Fatalf("expected log_path for python failure")
	}

	// Deterministic reproduction: pinning the captured seed must fail 2/2 rounds.
	var reproOut, reproErr bytes.Buffer
	reproRC := Run(context.Background(), []string{
		"flaky-py/app",
		"--repo-root", repo,
		"--lang", "python",
		"--rounds", "2",
		"--seed", victimFailure.Seed,
		"--pytest-cmd", pytestCmd,
		"--format", "json",
	}, &reproOut, &reproErr)
	if reproRC != 1 {
		t.Fatalf("python repro seed=%s rc=%d want 1 stdout=%q stderr=%q", victimFailure.Seed, reproRC, reproOut.String(), reproErr.String())
	}
	var reproRep Report
	if err := json.Unmarshal(reproOut.Bytes(), &reproRep); err != nil {
		t.Fatalf("repro invalid json: %v", err)
	}
	if reproRep.FailedRounds != 2 || reproRep.PassedRounds != 0 {
		t.Fatalf("expected deterministic 2/2 python failures for seed %s, got passed=%d failed=%d",
			victimFailure.Seed, reproRep.PassedRounds, reproRep.FailedRounds)
	}
}
