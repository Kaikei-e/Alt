package verifygolden

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"alt/scripts/altharness/internal/fsperm"
)

func TestTargetsRegistryHasAllSevenServices(t *testing.T) {
	t.Parallel()

	targets := Targets()
	if len(targets) != 7 {
		t.Fatalf("Targets() count = %d, want 7", len(targets))
	}

	wantNames := []string{
		"alt-backend",
		"auth-hub",
		"pre-processor",
		"rag-orchestrator",
		"inference-proxy",
		"sidecar-proxy",
		"protovis",
	}
	for i, want := range wantNames {
		if targets[i].Name != want {
			t.Errorf("targets[%d].Name = %q, want %q", i, targets[i].Name, want)
		}
		if targets[i].ModuleDir == "" || targets[i].Package == "" || targets[i].GoldenRelPath == "" {
			t.Errorf("targets[%d] (%s) has empty field: %+v", i, want, targets[i])
		}
		if !strings.HasSuffix(targets[i].GoldenRelPath, "testdata/effective_config.golden.json") {
			t.Errorf("targets[%d].GoldenRelPath = %q, want suffix testdata/effective_config.golden.json", i, targets[i].GoldenRelPath)
		}
	}
}

func writeSyntheticModule(t *testing.T, repoRoot, modRel, testSrc, goldenJSON string) Target {
	t.Helper()
	modDir := filepath.Join(repoRoot, modRel)
	testdataDir := filepath.Join(modDir, "testdata")
	if err := os.MkdirAll(testdataDir, fsperm.Dir); err != nil {
		t.Fatalf("mkdir testdata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(modDir, "go.mod"), []byte("module example.com/synth\n\ngo 1.24.0\n"), fsperm.File); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if testSrc != "" {
		if err := os.WriteFile(filepath.Join(modDir, "effective_config_golden_test.go"), []byte(testSrc), fsperm.File); err != nil {
			t.Fatalf("write test file: %v", err)
		}
	}
	goldenRel := filepath.ToSlash(filepath.Join(modRel, "testdata", "effective_config.golden.json"))
	if goldenJSON != "" {
		if err := os.WriteFile(filepath.Join(repoRoot, goldenRel), []byte(goldenJSON), fsperm.File); err != nil {
			t.Fatalf("write golden: %v", err)
		}
	}
	return Target{
		Name:          "synth",
		ModuleDir:     modRel,
		Package:       ".",
		GoldenRelPath: goldenRel,
		TestPattern:   "TestEffectiveConfigGolden",
	}
}

func TestVerify_PassesValidGoldenAndTest(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	golden := "{\n  \"body_limit_bytes\": 1000000,\n  \"service\": \"synth\"\n}\n"
	testSrc := `package synth

import (
	"os"
	"testing"
)

func TestEffectiveConfigGolden(t *testing.T) {
	got := "{\n  \"body_limit_bytes\": 1000000,\n  \"service\": \"synth\"\n}\n"
	want, err := os.ReadFile("testdata/effective_config.golden.json")
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if got != string(want) {
		t.Fatalf("drift:\ngot:\n%s\nwant:\n%s", got, want)
	}
}
`
	target := writeSyntheticModule(t, tmp, "svc", testSrc, golden)

	rep, err := Verify(t.Context(), tmp, []Target{target}, 1)
	if err != nil {
		t.Fatalf("Verify error: %v", err)
	}
	if !rep.OK {
		t.Fatalf("Verify OK = false, want true; detail=%q output=%q", rep.Targets[0].Detail, rep.Targets[0].Output)
	}
	if !rep.Targets[0].GoldenChecked || !rep.Targets[0].TestsPassed {
		t.Fatalf("unexpected target result: %+v", rep.Targets[0])
	}
}

func TestVerify_FailsOnZeroTestMatch(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	golden := "{\n  \"service\": \"synth\"\n}\n"
	// Test file does NOT define TestEffectiveConfigGolden -> go test exits 0 with "no tests to run".
	testSrc := `package synth

import "testing"

func TestSomethingElse(t *testing.T) {}
`
	target := writeSyntheticModule(t, tmp, "svc", testSrc, golden)

	rep, err := Verify(t.Context(), tmp, []Target{target}, 1)
	if err != nil {
		t.Fatalf("Verify unexpected error: %v", err)
	}
	if rep.OK {
		t.Fatalf("Verify OK = true on 0-test match, want false (anti-FP-01)")
	}
	if rep.Targets[0].TestsPassed {
		t.Errorf("TestsPassed = true on 0-test match, want false")
	}
	if !strings.Contains(rep.Targets[0].Detail, "TestEffectiveConfigGolden") {
		t.Errorf("detail = %q, want mention of missing TestEffectiveConfigGolden execution", rep.Targets[0].Detail)
	}
}

func TestVerify_FailsOnGoldenDrift(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	golden := "{\n  \"body_limit_bytes\": 1000000\n}\n"
	testSrc := `package synth

import (
	"os"
	"testing"
)

func TestEffectiveConfigGolden(t *testing.T) {
	got := "{\n  \"body_limit_bytes\": 1048576\n}\n"
	want, err := os.ReadFile("testdata/effective_config.golden.json")
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if got != string(want) {
		t.Fatalf("golden drift: got %s want %s", got, want)
	}
}
`
	target := writeSyntheticModule(t, tmp, "svc", testSrc, golden)

	rep, err := Verify(t.Context(), tmp, []Target{target}, 1)
	if err != nil {
		t.Fatalf("Verify unexpected error: %v", err)
	}
	if rep.OK {
		t.Fatalf("Verify OK = true on golden drift, want false")
	}
	if rep.Targets[0].TestsPassed {
		t.Errorf("TestsPassed = true on golden drift, want false")
	}
}

func TestVerify_FailsOnNonCanonicalOrMissingGolden(t *testing.T) {
	t.Parallel()

	t.Run("missing golden file", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		target := writeSyntheticModule(t, tmp, "svc", `package synth
import "testing"
func TestEffectiveConfigGolden(t *testing.T) {}
`, "")
		rep, err := Verify(t.Context(), tmp, []Target{target}, 1)
		if err != nil {
			t.Fatalf("Verify error: %v", err)
		}
		if rep.OK {
			t.Fatalf("expected failure for missing golden file")
		}
		if rep.Targets[0].GoldenChecked {
			t.Errorf("GoldenChecked = true for missing golden file")
		}
	})

	t.Run("non-canonical unsorted JSON", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		// Keys out of alphabetical order ("z" before "a").
		unsorted := "{\n  \"z\": 1,\n  \"a\": 2\n}\n"
		target := writeSyntheticModule(t, tmp, "svc", `package synth
import "testing"
func TestEffectiveConfigGolden(t *testing.T) {}
`, unsorted)
		rep, err := Verify(t.Context(), tmp, []Target{target}, 1)
		if err != nil {
			t.Fatalf("Verify error: %v", err)
		}
		if rep.OK {
			t.Fatalf("expected failure for non-canonical golden JSON")
		}
		if !strings.Contains(rep.Targets[0].Detail, "canonical") {
			t.Errorf("detail = %q, want mention of canonical JSON", rep.Targets[0].Detail)
		}
	})
}

func TestEvaluateCorpusCase_DetectsC11C12C13C14C15AndPassesBaseline(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRootForTest(t)
	cases := []struct {
		id            string
		wantDimension string
	}{
		{id: "C-11", wantDimension: "http_servers"},
		{id: "C-12", wantDimension: "file_modes"},
		{id: "C-13", wantDimension: "output_escapers"},
		{id: "C-14", wantDimension: "reverse_proxies"},
		{id: "C-15", wantDimension: "body_limits"},
	}

	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			t.Parallel()
			caseDir := filepath.Join(repoRoot, "scripts", "harness", "corpus", tc.id)
			beforePath := filepath.Join(caseDir, "before.go")
			afterPath := filepath.Join(caseDir, "after.go")

			// Baseline: before vs before must match with no diff.
			baseRes, err := EvaluateCorpusCase(t.Context(), tc.id, beforePath, beforePath)
			if err != nil {
				t.Fatalf("baseline EvaluateCorpusCase(%s) error: %v", tc.id, err)
			}
			if !baseRes.Equal {
				t.Fatalf("baseline %s Equal = false, want true; diff:\n%s", tc.id, baseRes.Diff)
			}

			// Regression: before vs after must detect effective config diff.
			regRes, err := EvaluateCorpusCase(t.Context(), tc.id, beforePath, afterPath)
			if err != nil {
				t.Fatalf("regression EvaluateCorpusCase(%s) error: %v", tc.id, err)
			}
			if regRes.Equal {
				t.Fatalf("regression %s Equal = true, want false; json=%s", tc.id, regRes.BeforeJSON)
			}
			if regRes.Signal != CorpusDiffSignal {
				t.Errorf("regression %s Signal = %q, want %q", tc.id, regRes.Signal, CorpusDiffSignal)
			}
			foundDim := false
			for _, d := range regRes.Dimensions {
				if d == tc.wantDimension {
					foundDim = true
					break
				}
			}
			if !foundDim {
				t.Errorf("regression %s Dimensions = %v, want %q", tc.id, regRes.Dimensions, tc.wantDimension)
			}
			if strings.TrimSpace(regRes.Diff) == "" {
				t.Errorf("regression %s Diff is empty", tc.id)
			}
		})
	}
}

func TestRun_CorpusCaseCLIMode(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRootForTest(t)
	c11Before := filepath.Join(repoRoot, "scripts", "harness", "corpus", "C-11", "before.go")
	c11After := filepath.Join(repoRoot, "scripts", "harness", "corpus", "C-11", "after.go")

	var out, errBuf bytes.Buffer
	code := Run(t.Context(), []string{
		"--check-corpus-case", "C-11",
		"--before", c11Before,
		"--after", c11Before,
	}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("baseline CLI exit=%d want 0; stdout=%q stderr=%q", code, out.String(), errBuf.String())
	}

	out.Reset()
	errBuf.Reset()
	code = Run(t.Context(), []string{
		"--check-corpus-case", "C-11",
		"--before", c11Before,
		"--after", c11After,
		"--format", "json",
	}, &out, &errBuf)
	if code != 1 {
		t.Fatalf("regression CLI exit=%d want 1; stdout=%q stderr=%q", code, out.String(), errBuf.String())
	}
	var parsed CorpusResult
	if err := json.Unmarshal(out.Bytes(), &parsed); err != nil {
		t.Fatalf("unmarshal CLI JSON output: %v\nraw: %s", err, out.String())
	}
	if parsed.Equal || parsed.Signal != CorpusDiffSignal {
		t.Errorf("unexpected JSON result: %+v", parsed)
	}
}

func TestRun_UnknownTargetReturnsUsageError(t *testing.T) {
	t.Parallel()

	var out, errBuf bytes.Buffer
	code := Run(t.Context(), []string{"--target", "nonexistent-service"}, &out, &errBuf)
	if code != 2 {
		t.Fatalf("exit=%d want 2; stderr=%q", code, errBuf.String())
	}
}

func TestRun_ContextCancelledFailsClosed(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	tmp := t.TempDir()
	golden := "{\n  \"service\": \"synth\"\n}\n"
	target := writeSyntheticModule(t, tmp, "svc", `package synth
import "testing"
func TestEffectiveConfigGolden(t *testing.T) {}
`, golden)
	rep, err := Verify(ctx, tmp, []Target{target}, 1)
	if err == nil && rep.OK {
		t.Fatalf("expected failure when context is already cancelled")
	}
}

func TestExtractEffectiveConfigFromGo_ProductionInferenceProxyPreservesXFF(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRootForTest(t)
	mainPath := filepath.Join(repoRoot, "docker", "inference-proxy", "main.go")
	if _, err := os.Stat(mainPath); err != nil {
		t.Skipf("sparse checkout without %s", mainPath)
	}
	cfg, err := ExtractEffectiveConfigFromGo(mainPath)
	if err != nil {
		t.Fatalf("ExtractEffectiveConfigFromGo(%s): %v", mainPath, err)
	}
	if len(cfg.ReverseProxies) == 0 {
		t.Fatalf("expected reverse proxy entry in %s", mainPath)
	}
	rp := cfg.ReverseProxies[0]
	if !rp.PreservesInboundXFF || rp.SynthesizesXForwardedHost || rp.SynthesizesXForwardedProto {
		t.Errorf("unexpected forwarding semantics for production inference-proxy: %+v", rp)
	}
}

func TestVerifyFilesOnly_RealRepoSevenTargets(t *testing.T) {
	t.Parallel()
	repoRoot := findRepoRootForTest(t)
	var out, errBuf bytes.Buffer
	code := Run(t.Context(), []string{"--check-files-only", "--repo-root", repoRoot, "--format", "text"}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("Run(--check-files-only) exited %d, stdout=%q stderr=%q", code, out.String(), errBuf.String())
	}
	if !strings.Contains(out.String(), "OK: verify-golden passed (7/7 targets)") {
		t.Fatalf("unexpected output:\n%s", out.String())
	}
}

func TestVerify_RealRepoSevenTargets(t *testing.T) {
	if os.Getenv("VERIFY_GOLDEN_REAL_REPO") != "1" {
		t.Skip("set VERIFY_GOLDEN_REAL_REPO=1 to run all 7 real repo targets")
	}

	repoRoot := findRepoRootForTest(t)
	var out, errBuf bytes.Buffer
	code := Run(t.Context(), []string{"--repo-root", repoRoot, "--format", "text"}, &out, &errBuf)
	t.Log(out.String())
	if code != 0 {
		t.Fatalf("Run(--repo-root %s) exited %d, stderr=%q", repoRoot, code, errBuf.String())
	}
	if !strings.Contains(out.String(), "OK: verify-golden passed (7/7 targets)") {
		t.Fatalf("unexpected summary output:\n%s", out.String())
	}
}

func findRepoRootForTest(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "scripts", "harness", "corpus")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("repo root not found from %s", dir)
		}
		dir = parent
	}
}
