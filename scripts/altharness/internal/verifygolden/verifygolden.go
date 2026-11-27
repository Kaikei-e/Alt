// Package verifygolden implements VERIFY-03 (Effective Runtime Configuration
// Golden Tests): it verifies committed canonical golden files across all target
// Go modules, runs each module's TestEffectiveConfigGolden suite with
// anti-FP-01 execution proof, and provides before/after effective-config
// diffing for corpus regression cases (C-11, C-12, C-13, C-14, C-15).
package verifygolden

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"alt/scripts/altharness/internal/execx"
)

const (
	exitOK    = 0
	exitFail  = 1
	exitUsage = 2

	defaultTestPattern = "TestEffectiveConfigGolden"
	// CorpusDiffSignal is the stable signal token emitted when before and after
	// effective configurations diverge in corpus evaluation mode.
	CorpusDiffSignal = "VERIFY-03:effective_config_diff"
)

// Target describes one VERIFY-03 effective-config golden test target.
type Target struct {
	Name          string `json:"name"`
	ModuleDir     string `json:"module_dir"`
	Package       string `json:"package"`
	GoldenRelPath string `json:"golden_path"`
	TestPattern   string `json:"test_pattern"`
}

// TargetResult records the verification result for one target.
type TargetResult struct {
	Target        Target `json:"target"`
	OK            bool   `json:"ok"`
	GoldenChecked bool   `json:"golden_checked"`
	TestsPassed   bool   `json:"tests_passed"`
	Detail        string `json:"detail,omitempty"`
	Output        string `json:"output,omitempty"`
}

// Report is the deterministic verification report across selected targets.
type Report struct {
	OK      bool           `json:"ok"`
	Targets []TargetResult `json:"targets"`
}

var canonicalTargets = []Target{
	{
		Name:          "alt-backend",
		ModuleDir:     "alt-backend/app",
		Package:       "./cmd/backend",
		GoldenRelPath: "alt-backend/app/cmd/backend/testdata/effective_config.golden.json",
		TestPattern:   defaultTestPattern,
	},
	{
		Name:          "auth-hub",
		ModuleDir:     "auth-hub",
		Package:       "./cmd/auth-hub",
		GoldenRelPath: "auth-hub/cmd/auth-hub/testdata/effective_config.golden.json",
		TestPattern:   defaultTestPattern,
	},
	{
		Name:          "pre-processor",
		ModuleDir:     "pre-processor/app",
		Package:       "./bootstrap",
		GoldenRelPath: "pre-processor/app/bootstrap/testdata/effective_config.golden.json",
		TestPattern:   defaultTestPattern,
	},
	{
		Name:          "rag-orchestrator",
		ModuleDir:     "rag-orchestrator",
		Package:       "./cmd/server",
		GoldenRelPath: "rag-orchestrator/cmd/server/testdata/effective_config.golden.json",
		TestPattern:   defaultTestPattern,
	},
	{
		Name:          "inference-proxy",
		ModuleDir:     "docker/inference-proxy",
		Package:       ".",
		GoldenRelPath: "docker/inference-proxy/testdata/effective_config.golden.json",
		TestPattern:   defaultTestPattern,
	},
	{
		Name:          "sidecar-proxy",
		ModuleDir:     "alt-backend/sidecar-proxy",
		Package:       "./pkg/proxy",
		GoldenRelPath: "alt-backend/sidecar-proxy/pkg/proxy/testdata/effective_config.golden.json",
		TestPattern:   defaultTestPattern,
	},
	{
		Name:          "protovis",
		ModuleDir:     "proto/tools/protovis",
		Package:       ".",
		GoldenRelPath: "proto/tools/protovis/testdata/effective_config.golden.json",
		TestPattern:   defaultTestPattern,
	},
}

// Targets returns a defensive copy of the canonical 7-target VERIFY-03 registry.
func Targets() []Target {
	out := make([]Target, len(canonicalTargets))
	copy(out, canonicalTargets)
	return out
}

// Run is the alt-harness CLI entry point for verify-golden.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if ctx == nil {
		_, _ = fmt.Fprintln(stderr, "ERROR: nil context")
		return exitUsage
	}

	var (
		repoRoot       string
		targetNames    []string
		format         = "text"
		concurrency    int
		corpusCase     string
		beforePath     string
		afterPath      string
		checkFilesOnly bool
	)

	rest := args
	for len(rest) > 0 {
		flagName := rest[0]
		rest = rest[1:]
		popValue := func() (string, bool) {
			if len(rest) == 0 {
				_, _ = fmt.Fprintf(stderr, "ERROR: %s requires a value\n", flagName)
				return "", false
			}
			v := rest[0]
			rest = rest[1:]
			return v, true
		}

		switch flagName {
		case "--repo-root":
			v, ok := popValue()
			if !ok {
				return exitUsage
			}
			repoRoot = v
		case "--target":
			v, ok := popValue()
			if !ok {
				return exitUsage
			}
			for _, part := range strings.Split(v, ",") {
				if trimmed := strings.TrimSpace(part); trimmed != "" {
					targetNames = append(targetNames, trimmed)
				}
			}
		case "--format":
			v, ok := popValue()
			if !ok {
				return exitUsage
			}
			format = v
		case "--concurrency":
			v, ok := popValue()
			if !ok {
				return exitUsage
			}
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 {
				_, _ = fmt.Fprintf(stderr, "ERROR: --concurrency must be a positive integer, got %q\n", v)
				return exitUsage
			}
			concurrency = n
		case "--check-corpus-case":
			v, ok := popValue()
			if !ok {
				return exitUsage
			}
			corpusCase = v
		case "--before":
			v, ok := popValue()
			if !ok {
				return exitUsage
			}
			beforePath = v
		case "--after":
			v, ok := popValue()
			if !ok {
				return exitUsage
			}
			afterPath = v
		case "--check-files-only":
			checkFilesOnly = true
		case "-h", "--help":
			_, _ = fmt.Fprintln(stdout, "usage: verify-golden [--repo-root DIR] [--target NAME] [--check-files-only] [--format text|json] [--concurrency N]")
			_, _ = fmt.Fprintln(stdout, "       verify-golden --check-corpus-case ID --before FILE --after FILE [--format text|json]")
			return exitOK
		default:
			_, _ = fmt.Fprintf(stderr, "ERROR: unknown argument %q\n", flagName)
			return exitUsage
		}
	}

	if format != "text" && format != "json" {
		_, _ = fmt.Fprintf(stderr, "ERROR: unsupported --format %q (expected text or json)\n", format)
		return exitUsage
	}

	if corpusCase != "" || beforePath != "" || afterPath != "" {
		return runCorpusCaseCLI(ctx, corpusCase, beforePath, afterPath, format, stdout, stderr)
	}

	if repoRoot == "" {
		repoRoot = defaultRepoRoot(ctx)
	}
	absRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR: resolve repo root: %v\n", err)
		return exitUsage
	}

	selected, err := selectTargets(targetNames)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return exitUsage
	}

	var report Report
	if checkFilesOnly {
		report, err = VerifyFilesOnly(ctx, absRoot, selected)
	} else {
		report, err = Verify(ctx, absRoot, selected, concurrency)
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return exitFail
	}

	if err := writeReport(stdout, report, format); err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR: write output: %v\n", err)
		return exitFail
	}
	if !report.OK {
		return exitFail
	}
	return exitOK
}

func runCorpusCaseCLI(ctx context.Context, caseID, beforePath, afterPath, format string, stdout, stderr io.Writer) int {
	if beforePath == "" || afterPath == "" {
		_, _ = fmt.Fprintln(stderr, "ERROR: --check-corpus-case requires both --before and --after")
		return exitUsage
	}
	res, err := EvaluateCorpusCase(ctx, caseID, beforePath, afterPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return exitUsage
	}
	if format == "json" {
		raw, err := CanonicalizeAny(res)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "ERROR: marshal result: %v\n", err)
			return exitFail
		}
		_, _ = stdout.Write(raw)
	} else if res.Equal {
		_, _ = fmt.Fprintf(stdout, "OK: VERIFY-03 effective config unchanged (%s)\n", caseID)
	} else {
		_, _ = fmt.Fprintf(stdout, "%s case=%s dimensions=%s\n%s\n",
			res.Signal, res.CaseID, strings.Join(res.Dimensions, ","), res.Diff)
	}
	if !res.Equal {
		return exitFail
	}
	return exitOK
}

func selectTargets(names []string) ([]Target, error) {
	all := Targets()
	if len(names) == 0 {
		return all, nil
	}
	byName := make(map[string]Target, len(all))
	for _, t := range all {
		byName[t.Name] = t
	}
	seen := make(map[string]struct{}, len(names))
	selected := make([]Target, 0, len(names))
	for _, name := range names {
		t, ok := byName[name]
		if !ok {
			var valid []string
			for _, candidate := range all {
				valid = append(valid, candidate.Name)
			}
			return nil, fmt.Errorf("unknown target %q (valid targets: %s)", name, strings.Join(valid, ", "))
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		selected = append(selected, t)
	}
	return selected, nil
}

// Verify checks every target's canonical golden file and executes its
// TestEffectiveConfigGolden suite with bounded concurrency while returning
// results in deterministic input order.
func Verify(ctx context.Context, repoRoot string, targets []Target, concurrency int) (Report, error) {
	if ctx == nil {
		return Report{}, errors.New("verifygolden: nil context")
	}
	if err := ctx.Err(); err != nil {
		return Report{}, err
	}
	if len(targets) == 0 {
		return Report{}, errors.New("verifygolden: no targets provided")
	}
	absRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return Report{}, fmt.Errorf("verifygolden: resolve repo root: %w", err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return Report{}, fmt.Errorf("verifygolden: repo root not accessible: %w", err)
	}
	info, err := os.Stat(resolvedRoot)
	if err != nil || !info.IsDir() {
		return Report{}, fmt.Errorf("verifygolden: repo root is not a directory: %s", absRoot)
	}

	if concurrency <= 0 {
		concurrency = runtime.GOMAXPROCS(0)
		if concurrency > 4 {
			concurrency = 4
		}
		if concurrency < 1 {
			concurrency = 1
		}
	}
	if concurrency > len(targets) {
		concurrency = len(targets)
	}

	results := make([]TargetResult, len(targets))
	jobs := make(chan int, len(targets))
	for i := range targets {
		jobs <- i
	}
	close(jobs)

	var wg sync.WaitGroup
	for range concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				results[idx] = verifyOne(ctx, resolvedRoot, targets[idx])
			}
		}()
	}
	wg.Wait()

	ok := true
	for _, r := range results {
		if !r.OK {
			ok = false
		}
	}
	return Report{OK: ok, Targets: results}, nil
}

var goldenTestDeclRE = regexp.MustCompile(`(?m)^func\s+TestEffectiveConfigGolden\s*\(`)

// VerifyFilesOnly performs a fast offline structural verification of all target
// golden JSON files and corresponding Go test declarations without invoking
// `go test` (used in CI harness gates where service module caches are absent).
func VerifyFilesOnly(ctx context.Context, repoRoot string, targets []Target) (Report, error) {
	if ctx == nil {
		return Report{}, errors.New("verifygolden: nil context")
	}
	if err := ctx.Err(); err != nil {
		return Report{}, err
	}
	if len(targets) == 0 {
		return Report{}, errors.New("verifygolden: no targets provided")
	}
	absRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return Report{}, fmt.Errorf("verifygolden: resolve repo root: %w", err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return Report{}, fmt.Errorf("verifygolden: repo root not accessible: %w", err)
	}
	results := make([]TargetResult, len(targets))
	allOK := true
	for i, target := range targets {
		res := TargetResult{Target: target}
		goldenAbs, gerr := safeRepoPath(resolvedRoot, target.GoldenRelPath)
		if gerr != nil {
			res.Detail = fmt.Sprintf("invalid golden path %q: %v", target.GoldenRelPath, gerr)
			allOK = false
			results[i] = res
			continue
		}
		if verr := validateCanonicalGoldenFile(goldenAbs); verr != nil {
			res.Detail = verr.Error()
			allOK = false
			results[i] = res
			continue
		}
		res.GoldenChecked = true

		pkgDir := filepath.Dir(filepath.Dir(target.GoldenRelPath))
		testRel := filepath.ToSlash(filepath.Join(pkgDir, "effective_config_golden_test.go"))
		testAbs, terr := safeRepoPath(resolvedRoot, testRel)
		if terr != nil {
			res.Detail = fmt.Sprintf("missing golden test file %q: %v", testRel, terr)
			allOK = false
			results[i] = res
			continue
		}
		raw, rerr := os.ReadFile(testAbs)
		if rerr != nil {
			res.Detail = fmt.Sprintf("read golden test file %q: %v", testRel, rerr)
			allOK = false
			results[i] = res
			continue
		}
		if !goldenTestDeclRE.Match(raw) {
			res.Detail = fmt.Sprintf("golden test file %q missing func TestEffectiveConfigGolden(", testRel)
			allOK = false
			results[i] = res
			continue
		}
		res.OK = true
		results[i] = res
	}
	return Report{OK: allOK, Targets: results}, nil
}

var passTestLineRE = regexp.MustCompile(`(?m)^--- PASS:\s+TestEffectiveConfigGolden\b`)

func verifyOne(ctx context.Context, resolvedRoot string, target Target) TargetResult {
	res := TargetResult{Target: target}

	if err := ctx.Err(); err != nil {
		res.Detail = fmt.Sprintf("context cancelled: %v", err)
		return res
	}

	goldenAbs, err := safeRepoPath(resolvedRoot, target.GoldenRelPath)
	if err != nil {
		res.Detail = fmt.Sprintf("invalid golden path %q: %v", target.GoldenRelPath, err)
		return res
	}
	if err := validateCanonicalGoldenFile(goldenAbs); err != nil {
		res.Detail = err.Error()
		return res
	}
	res.GoldenChecked = true

	modAbs, err := safeRepoPath(resolvedRoot, target.ModuleDir)
	if err != nil {
		res.Detail = fmt.Sprintf("invalid module dir %q: %v", target.ModuleDir, err)
		return res
	}
	pattern := target.TestPattern
	if strings.TrimSpace(pattern) == "" {
		pattern = defaultTestPattern
	}

	stdout, stderr, code, runErr := execx.Combined(ctx, execx.Cmd{
		Args: []string{"go", "-C", modAbs, "test", "-v", "-count=1", "-run", pattern, target.Package},
	})
	combined := strings.TrimSpace(joinStreams(stdout, stderr))
	res.Output = combined

	if runErr != nil {
		res.Detail = fmt.Sprintf("go test execution error: %v", runErr)
		return res
	}
	if code != 0 {
		res.Detail = fmt.Sprintf("go test exited %d (golden drift or test failure)", code)
		return res
	}
	if strings.Contains(combined, "testing: warning: no tests to run") || strings.Contains(combined, "[no test files]") {
		res.Detail = "silent 0-test pass blocked (anti-FP-01): TestEffectiveConfigGolden did not run"
		return res
	}
	if !passTestLineRE.MatchString(combined) {
		res.Detail = "silent 0-test pass blocked (anti-FP-01): output missing '--- PASS: TestEffectiveConfigGolden'"
		return res
	}

	res.TestsPassed = true
	res.OK = true
	return res
}

func safeRepoPath(resolvedRoot, rel string) (string, error) {
	if filepath.IsAbs(rel) {
		return "", errors.New("path must be relative to repo root")
	}
	clean := filepath.Clean(rel)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("path escapes repo root")
	}
	joined := filepath.Join(resolvedRoot, clean)
	st, err := os.Lstat(joined)
	if err != nil {
		return "", err
	}
	if st.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("symlink not allowed")
	}
	resolved, err := filepath.EvalSymlinks(joined)
	if err != nil {
		return "", err
	}
	relCheck, err := filepath.Rel(resolvedRoot, resolved)
	if err != nil || relCheck == ".." || strings.HasPrefix(relCheck, ".."+string(filepath.Separator)) {
		return "", errors.New("resolved path escapes repo root")
	}
	return resolved, nil
}

func readRegularFileRooted(path string) ([]byte, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("empty file path")
	}
	clean := filepath.Clean(path)
	dir := filepath.Dir(clean)
	base := filepath.Base(clean)
	if base == "." || base == ".." || strings.ContainsRune(base, filepath.Separator) {
		return nil, fmt.Errorf("invalid file path %q", path)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	f, err := root.Open(base)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return io.ReadAll(f)
}

func validateCanonicalGoldenFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("golden file missing or unreadable: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("golden file must be a regular non-symlink file: %s", path)
	}
	raw, err := readRegularFileRooted(path)
	if err != nil {
		return fmt.Errorf("read golden file: %w", err)
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return fmt.Errorf("golden file is empty: %s", path)
	}
	canonical, err := CanonicalizeJSON(raw)
	if err != nil {
		return fmt.Errorf("golden file is not valid JSON: %w", err)
	}
	if !bytes.Equal(raw, canonical) {
		return fmt.Errorf("golden file is not canonical sorted JSON (re-generate with -update or UPDATE_GOLDEN=1): %s", path)
	}
	return nil
}

// CanonicalizeJSON parses raw JSON (preserving exact number tokens) and
// re-encodes it with sorted map keys, 2-space indentation, unescaped HTML
// characters, and a single trailing newline.
func CanonicalizeJSON(raw []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var parsed any
	if err := dec.Decode(&parsed); err != nil {
		return nil, err
	}
	// Reject trailing garbage after the root JSON value.
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, errors.New("unexpected trailing tokens after JSON document")
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(parsed); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// CanonicalizeAny marshals v to JSON and canonicalizes key order and formatting.
func CanonicalizeAny(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return CanonicalizeJSON(raw)
}

func writeReport(w io.Writer, report Report, format string) error {
	if format == "json" {
		raw, err := CanonicalizeAny(report)
		if err != nil {
			return err
		}
		_, err = w.Write(raw)
		return err
	}
	passed := 0
	for _, tr := range report.Targets {
		if tr.OK {
			passed++
			if _, err := fmt.Fprintf(w, "PASS %s (%s)\n", tr.Target.Name, tr.Target.GoldenRelPath); err != nil {
				return err
			}
		} else {
			if _, err := fmt.Fprintf(w, "FAIL %s (%s): %s\n", tr.Target.Name, tr.Target.GoldenRelPath, tr.Detail); err != nil {
				return err
			}
			if tr.Output != "" {
				for _, line := range strings.Split(tr.Output, "\n") {
					if _, err := fmt.Fprintf(w, "  | %s\n", line); err != nil {
						return err
					}
				}
			}
		}
	}
	if report.OK {
		_, err := fmt.Fprintf(w, "OK: verify-golden passed (%d/%d targets)\n", passed, len(report.Targets))
		return err
	}
	_, err := fmt.Fprintf(w, "FAIL: verify-golden failed (%d/%d targets passed)\n", passed, len(report.Targets))
	return err
}

func joinStreams(stdout, stderr string) string {
	switch {
	case stdout == "":
		return stderr
	case stderr == "":
		return stdout
	default:
		return stdout + "\n" + stderr
	}
}

func defaultRepoRoot(ctx context.Context) string {
	out, err := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel").Output()
	if err == nil {
		if s := strings.TrimSpace(string(out)); s != "" {
			return s
		}
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}
