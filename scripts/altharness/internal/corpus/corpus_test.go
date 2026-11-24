package corpus

import (
	"alt/scripts/altharness/internal/fsperm"
	"alt/scripts/altharness/internal/tomlx"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestCatalogContracts(t *testing.T) {
	t.Parallel()
	if len(UnimplementedCorpusChecks) != 0 {
		t.Fatalf("UNIMPLEMENTED_CORPUS_CHECKS = %v", UnimplementedCorpusChecks)
	}
	for _, id := range []string{
		"ARCH-01", "ARCH-02", "ARCH-03", "ARCH-04", "ARCH-05", "ARCH-06",
		"CHECK-01", "CHECK-02", "CHECK-03", "CHECK-04", "CHECK-05", "CHECK-06", "CHECK-07",
		"VERIFY-01", "LOOP-01", "LOOP-02", "LOOP-03", "EXEC-01", "EXEC-02", "EXEC-03",
	} {
		if !has(ImplementedChecks, id) {
			t.Fatalf("ImplementedChecks missing %s", id)
		}
	}
	for _, id := range []string{"CHECK-01", "CHECK-05", "CHECK-06", "VERIFY-01"} {
		if !has(DiffChecks, id) {
			t.Fatalf("DiffChecks missing %s", id)
		}
	}
	if !has(ProcessChecks, "VERIFY-01") || !has(ProcessChecks, "EXEC-01") {
		t.Fatal("PROCESS_CHECKS must include VERIFY-01 and EXEC-01")
	}
	for id := range UnimplementedCorpusChecks {
		if has(ProcessChecks, id) {
			t.Fatalf("partition overlap %s", id)
		}
	}
	if len(pyreflyFallbackServices) != 9 {
		t.Fatalf("fallback services = %d", len(pyreflyFallbackServices))
	}
}

func TestSchemaRejections(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := t.TempDir()

	t.Run("non-contiguous", func(t *testing.T) {
		t.Parallel()
		dir := filepath.Join(t.TempDir(), "gap")
		writeCase(t, dir, "C-01", meta(metaOpt{id: "C-01"}), map[string]string{"before.py": "a = 1\n", "after.py": "a = 2\n"})
		writeCase(t, dir, "C-03", meta(metaOpt{id: "C-03"}), map[string]string{"before.py": "a = 1\n", "after.py": "a = 2\n"})
		code, _, errOut := runCLI(ctx, dir, repo, 2, false)
		if code != ExitFail || !strings.Contains(errOut, "contiguous") {
			t.Fatalf("code=%d stderr=%s", code, errOut)
		}
	})

	t.Run("identical", func(t *testing.T) {
		t.Parallel()
		dir := filepath.Join(t.TempDir(), "same")
		writeCase(t, dir, "C-01", meta(metaOpt{id: "C-01"}), map[string]string{"before.py": "same = 1\n", "after.py": "same = 1\n"})
		code, _, errOut := runCLI(ctx, dir, repo, 1, false)
		if code != ExitFail || !strings.Contains(errOut, "byte-identical") {
			t.Fatalf("code=%d stderr=%s", code, errOut)
		}
	})

	t.Run("traversal", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "escape.py"), []byte("escaped = 1\n"), fsperm.File); err != nil {
			t.Fatal(err)
		}
		dir := filepath.Join(root, "traversal")
		writeCase(t, dir, "C-01", meta(metaOpt{id: "C-01", before: "../escape.py"}), map[string]string{"after.py": "a = 2\n"})
		code, _, errOut := runCLI(ctx, dir, repo, 1, false)
		if code != ExitFail || !strings.Contains(errOut, "plain relative filename") {
			t.Fatalf("code=%d stderr=%s", code, errOut)
		}
	})

	t.Run("symlink", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		escape := filepath.Join(root, "escape.py")
		if err := os.WriteFile(escape, []byte("escaped = 1\n"), fsperm.File); err != nil {
			t.Fatal(err)
		}
		dir := filepath.Join(root, "symlink")
		caseDir := writeCase(t, dir, "C-01", meta(metaOpt{id: "C-01"}), map[string]string{"after.py": "a = 2\n"})
		if err := os.Symlink(escape, filepath.Join(caseDir, "before.py")); err != nil {
			t.Fatal(err)
		}
		code, _, errOut := runCLI(ctx, dir, repo, 1, false)
		if code != ExitFail || !strings.Contains(errOut, "symlink") {
			t.Fatalf("code=%d stderr=%s", code, errOut)
		}
	})

	t.Run("symlinked case dir and corpus root", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		realCorpus := filepath.Join(root, "outside-case")
		writeCase(t, realCorpus, "C-01", meta(metaOpt{id: "C-01"}), map[string]string{"before.py": "a = 1\n", "after.py": "a = 2\n"})

		dirSym := filepath.Join(root, "dir-symlink")
		if err := os.MkdirAll(dirSym, fsperm.Dir); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(realCorpus, "C-01"), filepath.Join(dirSym, "C-01")); err != nil {
			t.Fatal(err)
		}
		code, _, errOut := runCLI(ctx, dirSym, repo, 1, false)
		if code != ExitFail {
			t.Fatalf("symlinked case dir code=%d stderr=%s", code, errOut)
		}

		rootSym := filepath.Join(root, "corpus-root-symlink")
		if err := os.Symlink(realCorpus, rootSym); err != nil {
			t.Fatal(err)
		}
		code, _, errOut = runCLI(ctx, rootSym, repo, 1, false)
		if code != ExitFail {
			t.Fatalf("symlinked corpus root code=%d stderr=%s", code, errOut)
		}
	})

	t.Run("too many lines", func(t *testing.T) {
		t.Parallel()
		var b strings.Builder
		for i := 0; i < MaxSnippetLines+1; i++ {
			fmtLine(&b, i)
		}
		dir := filepath.Join(t.TempDir(), "lines")
		writeCase(t, dir, "C-01", meta(metaOpt{id: "C-01"}), map[string]string{"before.py": "x = 1\n", "after.py": b.String()})
		code, _, errOut := runCLI(ctx, dir, repo, 1, false)
		if code != ExitFail || !strings.Contains(errOut, "exceeds 120 lines") {
			t.Fatalf("code=%d stderr=%s", code, errOut)
		}
	})

	t.Run("too many bytes", func(t *testing.T) {
		t.Parallel()
		dir := filepath.Join(t.TempDir(), "bytes")
		big := append([]byte{0x7f, 'E', 'L', 'F'}, bytes.Repeat([]byte{'x'}, MaxSnippetBytes)...)
		writeCaseBytes(t, dir, "C-01", meta(metaOpt{id: "C-01", language: "binary", before: "before.bin", after: "after.bin"}), map[string][]byte{
			"before.bin": []byte("a"),
			"after.bin":  big,
		})
		code, _, errOut := runCLI(ctx, dir, repo, 1, false)
		if code != ExitFail || !strings.Contains(errOut, "exceeds 8192 bytes") {
			t.Fatalf("code=%d stderr=%s", code, errOut)
		}
	})

	t.Run("missing runner fails closed", func(t *testing.T) {
		t.Parallel()
		dir := filepath.Join(t.TempDir(), "norunner")
		writeCase(t, dir, "C-01", meta(metaOpt{
			id: "C-01", planned: []string{"ARCH-01"}, active: []string{"ARCH-01"}, caught: true, signal: "missing-runner",
		}), map[string]string{"before.py": "x = 1\n", "after.py": "x = 2\n"})
		code, out, errOut := runCLI(ctx, dir, repo, 1, false)
		if code != ExitFail || !strings.Contains(out+errOut, "no runner registered") {
			t.Fatalf("code=%d stdout=%s stderr=%s", code, out, errOut)
		}
	})

	t.Run("expected-miss capture rate", func(t *testing.T) {
		t.Parallel()
		dir := filepath.Join(t.TempDir(), "miss")
		writeCase(t, dir, "C-01", meta(metaOpt{id: "C-01", planned: []string{"VERIFY-01", "EXEC-01"}}), map[string]string{"before.py": "x = 1\n", "after.py": "x = 2\n"})
		code, out, errOut := runCLI(ctx, dir, repo, 1, false)
		want := "CORPUS CAPTURE RATE: 0/1 (0.0%) [caught=0: none | expected-miss=1 | failures=0]"
		if code != ExitOK || !strings.Contains(out, want) {
			t.Fatalf("code=%d stdout=%s stderr=%s", code, out, errOut)
		}
	})

	t.Run("empty corpus", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		code, _, _ := runCLI(ctx, dir, repo, 1, false)
		if code != ExitFail {
			t.Fatalf("default empty code=%d", code)
		}
		code, out, _ := runCLI(ctx, dir, repo, 1, true)
		if code != ExitOK || !strings.Contains(out, "CORPUS CAPTURE RATE: 0/0 (0.0%)") {
			t.Fatalf("allow empty code=%d out=%s", code, out)
		}
	})

	t.Run("negative min-cases", func(t *testing.T) {
		t.Parallel()
		var stdout, stderr bytes.Buffer
		code := Run(ctx, []string{"--min-cases", "-1"}, &stdout, &stderr)
		if code != ExitUsage || !strings.Contains(stderr.String(), "--min-cases must be >= 0") {
			t.Fatalf("code=%d stderr=%s", code, stderr.String())
		}
	})
}

func TestDiffModeExtraRunner(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "diff-mode")
	caseDir := writeCase(t, dir, "C-01", meta(metaOpt{
		id:      "C-01",
		mode:    "diff",
		planned: []string{"REPO-03"},
		active:  []string{"REPO-03"},
		caught:  true,
		signal:  "DIFF-DROPPED-GUARD",
	}), map[string]string{"before.py": "x = 1\n", "after.py": "x = 2\n"})
	loaded, errs := LoadCase(caseDir)
	if loaded == nil || len(errs) > 0 {
		t.Fatalf("load: %v", errs)
	}
	evOK := EvaluateCase(ctx, *loaded, t.TempDir(), map[string]RegisteredRunner{
		"REPO-03": {
			Mode: "diff",
			RunDiff: func(_ context.Context, _ Case, left, right, _ string) CheckOutcome {
				lb, _ := os.ReadFile(left)
				rb, _ := os.ReadFile(right)
				if bytes.Equal(lb, rb) {
					return CheckOutcome{ExitCode: 0, Output: "clean"}
				}
				return CheckOutcome{ExitCode: 1, Output: "DIFF-DROPPED-GUARD"}
			},
		},
	}, 10*time.Second)
	if evOK.Status != "caught" {
		t.Fatalf("evOK=%+v", evOK)
	}
	evFP := EvaluateCase(ctx, *loaded, t.TempDir(), map[string]RegisteredRunner{
		"REPO-03": {
			Mode: "diff",
			RunDiff: func(_ context.Context, _ Case, _, _, _ string) CheckOutcome {
				return CheckOutcome{ExitCode: 1, Output: "spurious diff error"}
			},
		},
	}, 10*time.Second)
	if evFP.Status != "fail" || !strings.Contains(evFP.Detail, "false-positive") {
		t.Fatalf("evFP=%+v", evFP)
	}
}

func TestInProcessRunners(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := findRepo(t)

	t.Run("REPO-02 passes before and catches zero-file scan after", func(t *testing.T) {
		t.Parallel()
		dir := filepath.Join(t.TempDir(), "repo02")
		sarif := `{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"gosec","rules":[]}},"results":[]}]}`
		writeCase(t, dir, "C-01", meta(metaOpt{
			id:      "C-01",
			lang:    "go",
			before:  "before.out",
			after:   "after.out",
			support: []string{"sample.go", "before.sarif", "after.sarif"},
			planned: []string{"REPO-02"},
			active:  []string{"REPO-02"},
			caught:  true,
			signal:  "ZERO-FILE SCAN DETECTED",
		}), map[string]string{
			"before.out":   "Summary:\n  Files  : 2\n  Issues : 0\n",
			"after.out":    "Summary:\n  Files  : 0\n  Issues : 0\n",
			"sample.go":    "package sample\nfunc F() {}\n",
			"before.sarif": sarif,
			"after.sarif":  sarif,
		})
		code, out, errOut := runCLI(ctx, dir, repo, 1, false)
		if code != ExitOK || !strings.Contains(out, "CORPUS CAPTURE RATE: 1/1 (100.0%)") {
			t.Fatalf("code=%d out=%s err=%s", code, out, errOut)
		}
	})

	t.Run("REPO-02 missing support_files fails closed", func(t *testing.T) {
		t.Parallel()
		dir := filepath.Join(t.TempDir(), "repo02-missing")
		writeCase(t, dir, "C-01", meta(metaOpt{
			id:      "C-01",
			lang:    "go",
			before:  "before.out",
			after:   "after.out",
			planned: []string{"REPO-02"},
			active:  []string{"REPO-02"},
			caught:  true,
			signal:  "ZERO-FILE SCAN DETECTED",
		}), map[string]string{
			"before.out": "Summary:\n  Files  : 2\n",
			"after.out":  "Summary:\n  Files  : 0\n",
		})
		code, out, errOut := runCLI(ctx, dir, repo, 1, false)
		if code != ExitFail || !strings.Contains(out+errOut, "runner-error") {
			t.Fatalf("code=%d out=%s err=%s", code, out, errOut)
		}
	})

	t.Run("CHECK-01 in-process", func(t *testing.T) {
		t.Parallel()
		dir := filepath.Join(t.TempDir(), "check01")
		writeCase(t, dir, "C-01", meta(metaOpt{
			id:      "C-01",
			lang:    "python",
			mode:    "diff",
			planned: []string{"CHECK-01"},
			active:  []string{"CHECK-01"},
			caught:  true,
			signal:  "python.control_flow_return_change",
		}), map[string]string{
			"before.py": "def acquire(flag: bool) -> int:\n    if flag:\n        return 1\n    else:\n        return 2\n",
			"after.py":  "def acquire(flag: bool) -> int:\n    if flag:\n        return 1\n",
		})
		code, out, errOut := runCLI(ctx, dir, repo, 1, false)
		if code != ExitOK || !strings.Contains(out, "CORPUS CAPTURE RATE: 1/1 (100.0%)") {
			t.Fatalf("code=%d out=%s err=%s", code, out, errOut)
		}
	})

	t.Run("CHECK-05 in-process", func(t *testing.T) {
		t.Parallel()
		dir := filepath.Join(t.TempDir(), "check05")
		writeCase(t, dir, "C-01", meta(metaOpt{
			id:      "C-01",
			lang:    "go",
			before:  "before.go",
			after:   "after.go",
			mode:    "diff",
			planned: []string{"CHECK-05"},
			active:  []string{"CHECK-05"},
			caught:  true,
			signal:  "codegen.do-not-edit-modified",
		}), map[string]string{
			"before.go": "// Code generated by oapi-codegen DO NOT EDIT.\npackage openapi\nconst V = 1\n",
			"after.go":  "// Code generated by oapi-codegen DO NOT EDIT.\npackage openapi\nconst V = 2\n",
		})
		code, out, errOut := runCLI(ctx, dir, repo, 1, false)
		if code != ExitOK || !strings.Contains(out, "CORPUS CAPTURE RATE: 1/1 (100.0%)") {
			t.Fatalf("code=%d out=%s err=%s", code, out, errOut)
		}
	})

	t.Run("CHECK-06 in-process zero-byte and ELF", func(t *testing.T) {
		t.Parallel()
		dir := filepath.Join(t.TempDir(), "check06")
		writeCaseBytes(t, dir, "C-01", meta(metaOpt{
			id:      "C-01",
			lang:    "python",
			mode:    "diff",
			planned: []string{"CHECK-06"},
			active:  []string{"CHECK-06"},
			caught:  true,
			signal:  "hygiene.zero-byte-tracked",
		}), map[string][]byte{
			"before.py": []byte("def router() -> str:\n    return 'ok'\n"),
			"after.py":  []byte(""),
		})
		writeCaseBytes(t, dir, "C-02", meta(metaOpt{
			id:      "C-02",
			lang:    "binary",
			before:  "before.go",
			after:   "after.elf",
			mode:    "after",
			planned: []string{"CHECK-06"},
			active:  []string{"CHECK-06"},
			caught:  true,
			signal:  "hygiene.binary-artifact",
		}), map[string][]byte{
			"before.go": []byte("package main\nfunc main() {}\n"),
			"after.elf": append([]byte{0x7f, 'E', 'L', 'F', 2, 1, 1, 0}, bytes.Repeat([]byte{0}, 56)...),
		})
		code, out, errOut := runCLI(ctx, dir, repo, 2, false)
		if code != ExitOK || !strings.Contains(out, "CORPUS CAPTURE RATE: 2/2 (100.0%)") {
			t.Fatalf("code=%d out=%s err=%s", code, out, errOut)
		}
	})

	t.Run("VERIFY-01 in-process sequence pass, baseline false-positive, missing test", func(t *testing.T) {
		t.Parallel()
		charTest := "#!/usr/bin/env bash\nset -euo pipefail\ndir=\"$(cd \"$(dirname \"$0\")\" && pwd)\"\nout=\"$(\"$dir/check-compose-variables.sh\")\"\n[[ \"$out\" == \"ok-no-env-file\" ]]\n"
		beforeSh := "#!/usr/bin/env bash\nset -euo pipefail\necho ok-no-env-file\n"
		afterSh := "#!/usr/bin/env bash\nset -euo pipefail\necho \"ERROR: --env-file is required\" >&2\nexit 2\n"

		dirOK := filepath.Join(t.TempDir(), "verify01-ok")
		writeCase(t, dirOK, "C-01", meta(metaOpt{
			id:          "C-01",
			lang:        "shell",
			before:      "before.sh",
			after:       "after.sh",
			support:     []string{"test_characterization.sh"},
			mode:        "diff",
			originFiles: []string{"scripts/check-compose-variables.sh"},
			planned:     []string{"VERIFY-01"},
			active:      []string{"VERIFY-01"},
			caught:      true,
			signal:      "SEQUENCE_FINAL_TEST_FAILED",
		}), map[string]string{
			"before.sh":                beforeSh,
			"after.sh":                 afterSh,
			"test_characterization.sh": charTest,
		})
		code, out, errOut := runCLI(ctx, dirOK, repo, 1, false)
		if code != ExitOK || !strings.Contains(out, "CORPUS CAPTURE RATE: 1/1 (100.0%)") {
			t.Fatalf("verify01 ok: code=%d out=%s err=%s", code, out, errOut)
		}

		dirFP := filepath.Join(t.TempDir(), "verify01-fp")
		writeCase(t, dirFP, "C-01", meta(metaOpt{
			id:          "C-01",
			lang:        "shell",
			before:      "before.sh",
			after:       "after.sh",
			support:     []string{"test_characterization.sh"},
			mode:        "diff",
			originFiles: []string{"scripts/check-compose-variables.sh"},
			planned:     []string{"VERIFY-01"},
			active:      []string{"VERIFY-01"},
			caught:      true,
			signal:      "SEQUENCE_FINAL_TEST_FAILED",
		}), map[string]string{
			"before.sh":                beforeSh,
			"after.sh":                 afterSh,
			"test_characterization.sh": "#!/usr/bin/env bash\nexit 1\n",
		})
		code, out, errOut = runCLI(ctx, dirFP, repo, 1, false)
		if code != ExitFail || !strings.Contains(out+errOut, "false-positive") || !strings.Contains(out+errOut, "SEQUENCE_BASELINE_TEST_FAILED") {
			t.Fatalf("verify01 fp: code=%d out=%s err=%s", code, out, errOut)
		}

		dirMiss := filepath.Join(t.TempDir(), "verify01-miss")
		writeCase(t, dirMiss, "C-01", meta(metaOpt{
			id:          "C-01",
			lang:        "shell",
			before:      "before.sh",
			after:       "after.sh",
			mode:        "diff",
			originFiles: []string{"scripts/check-compose-variables.sh"},
			planned:     []string{"VERIFY-01"},
			active:      []string{"VERIFY-01"},
			caught:      true,
			signal:      "SEQUENCE_FINAL_TEST_FAILED",
		}), map[string]string{
			"before.sh": beforeSh,
			"after.sh":  afterSh,
		})
		code, out, errOut = runCLI(ctx, dirMiss, repo, 1, false)
		if code != ExitFail || !strings.Contains(out+errOut, "runner-error") {
			t.Fatalf("verify01 missing test: code=%d out=%s err=%s", code, out, errOut)
		}
	})
}

func TestRealCorpusExpectedMissStaysZero(t *testing.T) {
	t.Parallel()
	repo := findRepo(t)
	corpusDir := filepath.Join(repo, "scripts", "harness", "corpus")
	cases, errs := DiscoverCases(corpusDir, DefaultMinCases, false)
	if len(errs) > 0 {
		t.Fatalf("real corpus validation errors:\n%s", strings.Join(errs, "\n"))
	}
	if len(cases) < DefaultMinCases {
		t.Fatalf("cases=%d", len(cases))
	}
	miss := 0
	for _, c := range cases {
		if len(c.Detection.ActiveChecks) == 0 {
			miss++
		}
	}
	if miss != 0 {
		t.Fatalf("expected-miss candidates=%d", miss)
	}
}

func TestRealCorpusParallelDeterministicOrder(t *testing.T) {
	t.Parallel()
	repo := findRepo(t)
	corpusDir := filepath.Join(repo, "scripts", "harness", "corpus")
	code, out, errOut := runCLI(context.Background(), corpusDir, repo, DefaultMinCases, false)
	if code != ExitOK {
		t.Fatalf("real corpus code=%d\nstdout:\n%s\nstderr:\n%s", code, out, errOut)
	}
	if !strings.Contains(out, "CORPUS CAPTURE RATE: 22/22 (100.0%)") {
		t.Fatalf("unexpected capture rate:\n%s", out)
	}
	prevPos := -1
	for i := 1; i <= 22; i++ {
		id := "C-" + strconv.Itoa(i)
		if i < 10 {
			id = "C-0" + strconv.Itoa(i)
		}
		pos := strings.Index(out, "  CAUGHT         "+id+"  ")
		if pos < 0 {
			t.Fatalf("missing %s CAUGHT line in:\n%s", id, out)
		}
		if pos <= prevPos {
			t.Fatalf("non-deterministic order at %s:\n%s", id, out)
		}
		prevPos = pos
	}
}

func TestTOMLMatchesTomlx(t *testing.T) {
	t.Parallel()
	repo := findRepo(t)
	paths := []string{filepath.Join(repo, "scripts", "lint", "ratchet", "suppressions.toml")}
	matches, _ := filepath.Glob(filepath.Join(repo, "scripts", "harness", "corpus", "C-*", "meta.toml"))
	paths = append(paths, matches...)
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		got, err := ParseTOML(string(raw))
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		var want map[string]any
		if err := tomlx.Unmarshal(raw, &want); err != nil {
			t.Fatalf("tomlx %s: %v", path, err)
		}
		if !tomlEqual(got, want) {
			t.Fatalf("%s mismatch\ngot=%#v\nwant=%#v", path, got, want)
		}
	}
}

func tomlEqual(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		if !valueEqual(av, b[k]) {
			return false
		}
	}
	return true
}

func valueEqual(a, b any) bool {
	switch av := a.(type) {
	case map[string]any:
		bv, ok := b.(map[string]any)
		return ok && tomlEqual(av, bv)
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !valueEqual(av[i], bv[i]) {
				return false
			}
		}
		return true
	case string:
		bs, ok := b.(string)
		return ok && av == bs
	case bool:
		bb, ok := b.(bool)
		return ok && av == bb
	case int64:
		switch bv := b.(type) {
		case int64:
			return av == bv
		case int:
			return av == int64(bv)
		default:
			return false
		}
	default:
		return false
	}
}

type metaOpt struct {
	id          string
	lang        string
	language    string
	before      string
	after       string
	support     []string
	mode        string
	originFiles []string
	planned     []string
	active      []string
	caught      bool
	signal      string
}

func meta(opt metaOpt) string {
	lang := opt.language
	if lang == "" {
		lang = opt.lang
	}
	if lang == "" {
		lang = "python"
	}
	if opt.before == "" {
		opt.before = "before.py"
	}
	if opt.after == "" {
		opt.after = "after.py"
	}
	if opt.mode == "" {
		opt.mode = "after"
	}
	if opt.originFiles == nil {
		opt.originFiles = []string{"scripts/sample.py"}
	}
	if opt.planned == nil {
		opt.planned = []string{"VERIFY-01"}
	}
	if opt.active == nil {
		opt.active = []string{}
	}
	caught := "false"
	if opt.caught {
		caught = "true"
	}
	lines := []string{
		"id = " + pyRepr(opt.id),
		"title = \"Sample failure case\"",
		"category = \"autofix-logic-corruption\"",
		"language = " + pyRepr(lang),
		"before_file = " + pyRepr(opt.before),
		"after_file = " + pyRepr(opt.after),
		"support_files = " + tomlStringList(opt.support),
		"evaluation_mode = " + pyRepr(opt.mode),
		"",
		"[origin]",
		"branch = \"chore/lint-rollout\"",
		"base_commit = \"6cae3ef6f\"",
		"files = " + tomlStringList(opt.originFiles),
		"commits = [\"d8ce79dec\"]",
		"source_ref = \"docs/review/lint-rollout-harness-learnings-2026-11-21.md#2.1\"",
		"notes = \"Minimal deterministic failure reproduction snippet.\"",
		"",
		"[detection]",
		"planned_checks = " + tomlStringList(opt.planned),
		"active_checks = " + tomlStringList(opt.active),
		"currently_caught = " + caught,
		"current_method = \"diff-review-only\"",
		"expected_signal = " + pyRepr(opt.signal),
		"",
	}
	return strings.Join(lines, "\n")
}

func tomlStringList(items []string) string {
	parts := make([]string, len(items))
	for i, item := range items {
		parts[i] = strconvQuote(item)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func strconvQuote(s string) string {
	return "\"" + strings.ReplaceAll(s, "\"", "\\\"") + "\""
}

func writeCase(t *testing.T, corpusDir, id, metaText string, files map[string]string) string {
	t.Helper()
	raw := map[string][]byte{}
	for name, body := range files {
		raw[name] = []byte(body)
	}
	return writeCaseBytes(t, corpusDir, id, metaText, raw)
}

func writeCaseBytes(t *testing.T, corpusDir, id, metaText string, files map[string][]byte) string {
	t.Helper()
	dir := filepath.Join(corpusDir, id)
	if err := os.MkdirAll(dir, fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "meta.toml"), []byte(metaText), fsperm.File); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), body, fsperm.File); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func runCLI(ctx context.Context, corpusDir, repo string, min int, allowEmpty bool) (int, string, string) {
	args := []string{"--corpus-dir", corpusDir, "--repo-root", repo, "--min-cases", itoa(min)}
	if allowEmpty {
		args = append(args, "--allow-empty-corpus")
	}
	var stdout, stderr bytes.Buffer
	code := Run(ctx, args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

func fmtLine(b *strings.Builder, i int) {
	b.WriteString("x_")
	b.WriteString(itoa(i))
	b.WriteString(" = ")
	b.WriteString(itoa(i))
	b.WriteByte('\n')
}

func findRepo(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "scripts", "altharness", "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repo root not found")
		}
		dir = parent
	}
}
