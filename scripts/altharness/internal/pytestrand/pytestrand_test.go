package pytestrand

import (
	"alt/scripts/altharness/internal/fsperm"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

const (
	testCaseCount = 12
	seedA         = 12345
	seedB         = 67890
)

func pytestCmdForTest(t *testing.T) string {
	t.Helper()
	if p, err := exec.LookPath("pytest"); err == nil {
		return p
	}
	if py, err := exec.LookPath("python3"); err == nil {
		return py + " -m pytest"
	}
	t.Skip("neither pytest nor python3 found on PATH")
	return ""
}

func writeOrderRecordingSuite(t *testing.T, root string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "pyproject.toml"),
		[]byte("[project]\nname = \"rand-fixture\"\nversion = \"0.1.0\"\nrequires-python = \">=3.11\"\n"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	testsDir := filepath.Join(root, "tests")
	if err := os.MkdirAll(testsDir, fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	var lines []string
	lines = append(lines, "import os", "import pathlib", "")
	for idx := 0; idx < testCaseCount; idx++ {
		name := fmt.Sprintf("test_case_%02d", idx)
		lines = append(lines,
			fmt.Sprintf("def %s() -> None:", name),
			"    order_file = pathlib.Path(os.environ['ORDER_FILE'])",
			"    with order_file.open('a', encoding='utf-8') as fh:",
			fmt.Sprintf("        fh.write('%s\\n')", name),
			"",
		)
	}
	if err := os.WriteFile(filepath.Join(testsDir, "test_order.py"), []byte(strings.Join(lines, "\n")), fsperm.File); err != nil {
		t.Fatal(err)
	}
}

func baseEnvWithOrderFile(orderFile string) map[string]string {
	env := map[string]string{}
	for _, kv := range os.Environ() {
		if k, v, ok := strings.Cut(kv, "="); ok {
			env[k] = v
		}
	}
	if orderFile != "" {
		env["ORDER_FILE"] = orderFile
	}
	return env
}

func readLines(path string) []string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []string
	for _, ln := range strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n") {
		if ln != "" {
			out = append(out, ln)
		}
	}
	return out
}

func TestDeterministicSeedAndShuffling(t *testing.T) {
	t.Parallel()
	pytestCmd := pytestCmdForTest(t)
	root := t.TempDir()
	orderA := filepath.Join(root, "order_a.txt")
	orderB := filepath.Join(root, "order_b.txt")
	orderC := filepath.Join(root, "order_c.txt")
	seedLog := filepath.Join(root, "seeds.jsonl")

	writeOrderRecordingSuite(t, root)

	var outA, errA bytes.Buffer
	rcA := RunWithOptions(context.Background(), []string{
		"--pytest-cmd", pytestCmd,
		root,
		"--seed", fmt.Sprint(seedA),
		"--seed-log", seedLog,
		"--", "-q",
	}, Options{Stdout: &outA, Stderr: &errA, Env: baseEnvWithOrderFile(orderA)})

	if rcA != 0 || !strings.Contains(outA.String(), fmt.Sprintf("PYTEST_RANDOM_SEED=%d", seedA)) {
		t.Fatalf("runA rc=%d stdout=%q stderr=%q", rcA, outA.String(), errA.String())
	}
	linesA := readLines(orderA)

	var outB, errB bytes.Buffer
	rcB := RunWithOptions(context.Background(), []string{
		"--pytest-cmd", pytestCmd,
		root,
		"--seed", fmt.Sprint(seedA),
		"--", "-q",
	}, Options{Stdout: &outB, Stderr: &errB, Env: baseEnvWithOrderFile(orderB)})
	linesB := readLines(orderB)

	if rcB != 0 || len(linesA) != testCaseCount || !reflect.DeepEqual(linesA, linesB) {
		t.Fatalf("same seed mismatch rcB=%d a=%v b=%v", rcB, linesA, linesB)
	}

	var outC, errC bytes.Buffer
	rcC := RunWithOptions(context.Background(), []string{
		"--pytest-cmd", pytestCmd,
		root,
		"--seed", fmt.Sprint(seedB),
		"--", "-q",
	}, Options{Stdout: &outC, Stderr: &errC, Env: baseEnvWithOrderFile(orderC)})
	linesC := readLines(orderC)

	sortedA := append([]string(nil), linesA...)
	sort.Strings(sortedA)
	sortedC := append([]string(nil), linesC...)
	sort.Strings(sortedC)

	if rcC != 0 || reflect.DeepEqual(linesA, linesC) || !reflect.DeepEqual(sortedA, sortedC) {
		t.Fatalf("different seed mismatch rcC=%d a=%v c=%v", rcC, linesA, linesC)
	}
	if reflect.DeepEqual(linesA, sortedA) {
		t.Fatalf("expected shuffled order to differ from sorted order: %v", linesA)
	}

	logLines := readLines(seedLog)
	if len(logLines) != 1 {
		t.Fatalf("seed log lines=%d want 1", len(logLines))
	}
	var rec map[string]any
	if err := json.Unmarshal([]byte(logLines[0]), &rec); err != nil {
		t.Fatalf("seed log invalid JSON: %v", err)
	}
	if int(rec["seed"].(float64)) != seedA || int(rec["exit_code"].(float64)) != 0 || rec["target"] != root {
		t.Fatalf("unexpected seed log record: %#v", rec)
	}
	if ts, ok := rec["timestamp"].(string); !ok || ts == "" {
		t.Fatalf("missing timestamp: %#v", rec)
	}
}

func TestGeneratedSeedAndFailureExitCode(t *testing.T) {
	t.Parallel()
	pytestCmd := pytestCmdForTest(t)
	root := t.TempDir()
	testsDir := filepath.Join(root, "tests")
	if err := os.MkdirAll(testsDir, fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(testsDir, "test_fail.py"),
		[]byte("def test_always_fails() -> None:\n    assert False\n"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	seedLog := filepath.Join(root, "seeds.jsonl")

	var outBuf, errBuf bytes.Buffer
	rc := RunWithOptions(context.Background(), []string{
		"--pytest-cmd", pytestCmd,
		root,
		"--seed-log", seedLog,
		"--", "-q",
	}, Options{Stdout: &outBuf, Stderr: &errBuf, Env: baseEnvWithOrderFile("")})

	if rc != 1 {
		t.Fatalf("rc=%d want 1 stdout=%q stderr=%q", rc, outBuf.String(), errBuf.String())
	}
	if !strings.Contains(outBuf.String(), "PYTEST_RANDOM_SEED=") {
		t.Fatalf("stdout missing PYTEST_RANDOM_SEED=: %q", outBuf.String())
	}
	logLines := readLines(seedLog)
	if len(logLines) != 1 {
		t.Fatalf("seed log lines=%d want 1", len(logLines))
	}
	var rec map[string]any
	if err := json.Unmarshal([]byte(logLines[0]), &rec); err != nil {
		t.Fatal(err)
	}
	if int(rec["exit_code"].(float64)) != 1 {
		t.Fatalf("expected exit_code=1, got %#v", rec)
	}
	if _, ok := rec["seed"].(float64); !ok {
		t.Fatalf("expected numeric seed, got %#v", rec)
	}
}

func TestValidationAndUsageErrors(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	var helpOut, helpErr bytes.Buffer
	if rc := RunWithOptions(context.Background(), []string{"--help"}, Options{Stdout: &helpOut, Stderr: &helpErr}); rc != 0 || !strings.Contains(helpOut.String(), "pytest-rand") {
		t.Fatalf("--help rc=%d out=%q err=%q", rc, helpOut.String(), helpErr.String())
	}

	cases := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"missing target", nil, "usage:"},
		{"nonexistent target", []string{filepath.Join(root, "does-not-exist")}, "target directory does not exist"},
		{"empty pytest-cmd", []string{"--pytest-cmd", "   ", root}, "--pytest-cmd must not be empty"},
		{"invalid seed", []string{"--seed", "abc", root}, "invalid --seed"},
		{"unknown flag", []string{"--bogus", root}, "unknown flag"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var out, errb bytes.Buffer
			rc := RunWithOptions(context.Background(), tc.args, Options{Stdout: &out, Stderr: &errb})
			if rc != 2 || !strings.Contains(strings.ToLower(errb.String()), strings.ToLower(tc.wantErr)) {
				t.Fatalf("rc=%d stderr=%q want %q", rc, errb.String(), tc.wantErr)
			}
		})
	}
}

func writeFakePytestScript(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	script := "#!/usr/bin/env bash\nset -euo pipefail\nprintf 'PWD=%s\\nSEED=%s\\nARGS=%s\\n' \"$PWD\" \"${ALT_PYTEST_RANDOM_SEED:-}\" \"$*\"\n"
	if err := os.WriteFile(path, []byte(script), fsperm.Dir); err != nil {
		t.Fatal(err)
	}
}

func TestRelativePytestCmdResolution(t *testing.T) {
	t.Parallel()

	t.Run("target-dir relative .venv/bin/pytest", func(t *testing.T) {
		t.Parallel()
		target := t.TempDir()
		writeFakePytestScript(t, filepath.Join(target, ".venv", "bin", "pytest"))

		var out, errb bytes.Buffer
		rc := RunWithOptions(context.Background(), []string{
			"--pytest-cmd", ".venv/bin/pytest",
			"--seed", "42",
			target,
			"--", "-q",
		}, Options{Stdout: &out, Stderr: &errb})
		if rc != 0 {
			t.Fatalf("rc=%d want 0 stdout=%q stderr=%q", rc, out.String(), errb.String())
		}
		if !strings.Contains(out.String(), "SEED=42") || !strings.Contains(out.String(), "-p "+pluginModuleName+" -q") {
			t.Fatalf("unexpected stdout: %q", out.String())
		}
	})

	t.Run("caller-cwd relative scripts/custom-pytest", func(t *testing.T) {
		t.Parallel()
		wd, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		target := t.TempDir()
		callerTools := t.TempDir()
		scriptAbs := filepath.Join(callerTools, "scripts", "custom-pytest")
		writeFakePytestScript(t, scriptAbs)
		relFromWd, err := filepath.Rel(wd, scriptAbs)
		if err != nil {
			t.Fatal(err)
		}

		var out, errb bytes.Buffer
		rc := RunWithOptions(context.Background(), []string{
			"--pytest-cmd", relFromWd,
			"--seed", "77",
			target,
			"--", "-k", "smoke",
		}, Options{Stdout: &out, Stderr: &errb})
		if rc != 0 {
			t.Fatalf("rc=%d want 0 stdout=%q stderr=%q", rc, out.String(), errb.String())
		}
		if !strings.Contains(out.String(), "SEED=77") || !strings.Contains(out.String(), "-k smoke") {
			t.Fatalf("unexpected stdout: %q", out.String())
		}
	})

	t.Run("relative PATH entry resolves to absolute path before cmd.Dir", func(t *testing.T) {
		t.Parallel()
		wd, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		target := t.TempDir()
		binDir := t.TempDir()
		writeFakePytestScript(t, filepath.Join(binDir, "my-pytest"))
		relBinDir, err := filepath.Rel(wd, binDir)
		if err != nil {
			t.Fatal(err)
		}

		env := baseEnvWithOrderFile("")
		env["PATH"] = relBinDir + string(os.PathListSeparator) + env["PATH"]
		var out, errb bytes.Buffer
		rc := RunWithOptions(context.Background(), []string{
			"--pytest-cmd", "my-pytest",
			"--seed", "99",
			target,
		}, Options{Stdout: &out, Stderr: &errb, Env: env})
		if rc != 0 {
			t.Fatalf("rc=%d want 0 stdout=%q stderr=%q", rc, out.String(), errb.String())
		}
		if !strings.Contains(out.String(), "SEED=99") {
			t.Fatalf("unexpected stdout: %q", out.String())
		}
	})
}
