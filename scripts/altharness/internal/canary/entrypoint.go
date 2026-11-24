package canary

import (
	"alt/scripts/altharness/internal/fsperm"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode"
)

const (
	quickSubprocessTimeout    = 15 * time.Second
	baselineSubprocessTimeout = 180 * time.Second
	lintSubprocessTimeout     = 300 * time.Second
	perLanguageTimeout        = 20 * time.Minute
	maxLanguageWorkers        = 4

	entrypointHelp = `End-to-end isolated entrypoint canary for scripts/lint/run.sh.

flags:
  --repo-root DIR     Repository root (default: auto-detected)
  --languages LIST    Comma-separated subset of go,python,rust,frontend,deno,proto
  --skip-baseline     Skip the unmutated baseline run before injecting violations
  --available-only    Run only languages whose toolchain binaries are on PATH
`
)

var (
	allLanguages = []string{"go", "python", "rust", "frontend", "deno", "proto"}
	copyIgnore   = map[string]struct{}{
		".git": {}, ".venv": {}, "venv": {}, "node_modules": {}, "target": {},
		"_reports": {}, ".svelte-kit": {}, "build": {}, "dist": {},
		"playwright-report": {}, "test-results": {}, "__pycache__": {},
		".ruff_cache": {}, ".mypy_cache": {}, ".pytest_cache": {},
	}
	dockerfileBunPinRE = regexp.MustCompile(`(?m)(^FROM\s+oven/bun:)([0-9][0-9.]*)(-.*)?$`)
	languageTargets    = map[string]string{
		"go":       "docker/otel-relay",
		"python":   "rerank-server",
		"rust":     "plecto",
		"frontend": "alt-frontend-sv",
		"deno":     "auth-token-manager",
		"proto":    "proto",
	}
)

type caseResult struct {
	language string
	target   string
	passed   bool
	duration time.Duration
	detail   string
}

// RunEntrypoint executes the entrypoint_canary.py CLI. args excludes argv[0].
// Language cases run in a bounded worker pool. The summary is printed in the
// requested language order.
func RunEntrypoint(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	st := newStreams(stdout, stderr)
	if ctx == nil {
		st.error("error: nil context")
		return exitUsage
	}
	if hasHelp(args) {
		fmt.Fprint(st.stdout, entrypointHelp)
		return exitOK
	}
	flags, msg := parseFlags(args,
		nil,
		[]string{"repo-root", "languages"},
		[]string{"skip-baseline", "available-only"},
		nil,
	)
	if msg != "" {
		if msg == "help" {
			fmt.Fprint(st.stdout, entrypointHelp)
			return exitOK
		}
		st.error(msg)
		return exitUsage
	}
	repoRoot := flags.string("repo-root")
	var err error
	if repoRoot == "" {
		repoRoot, err = defaultRepoRoot()
	} else {
		repoRoot, err = resolvePath(repoRoot)
	}
	if err != nil {
		st.error("error: " + err.Error())
		return exitFail
	}
	langSpec := flags.string("languages")
	if _, ok := flags.vals["languages"]; !ok {
		langSpec = strings.Join(allLanguages, ",")
	}
	languages := splitLanguages(langSpec)
	var unknown []string
	for _, lang := range languages {
		if _, ok := languageTargets[lang]; !ok {
			unknown = append(unknown, lang)
		}
	}
	if len(unknown) > 0 {
		st.error(fmt.Sprintf(
			"ERROR: unknown language(s): %s (expected subset of %s)",
			pyList(unknown), pyList(allLanguages),
		))
		return exitUsage
	}
	if flags.bools["available-only"] {
		var selected, skipped []string
		for _, lang := range languages {
			if isLanguageAvailable(ctx, repoRoot, lang) {
				selected = append(selected, lang)
			} else {
				skipped = append(skipped, lang)
			}
		}
		if len(skipped) > 0 {
			st.info("  SKIP  unavailable language toolchain(s): " + strings.Join(skipped, ", "))
		}
		if len(selected) == 0 {
			st.error("ERROR: --available-only matched 0 available language toolchains")
			return exitFail
		}
		languages = selected
	}
	if len(languages) == 0 {
		st.error("ERROR: no languages selected")
		return exitFail
	}
	verifyBaseline := !flags.bools["skip-baseline"]
	results := runLanguagePool(ctx, languages, workerLimit(len(languages)), perLanguageTimeout, func(langCtx context.Context, lang string) caseResult {
		return runLanguageCase(langCtx, repoRoot, lang, verifyBaseline)
	})
	failed := 0
	for _, res := range results {
		status := "FAIL"
		if res.passed {
			status = "PASS"
		} else {
			failed++
		}
		fmt.Fprintf(st.stdout, "  %s  [%-8s] %-22s (%5.2fs) %s\n", status, res.language, res.target, res.duration.Seconds(), res.detail)
	}
	fmt.Fprintf(st.stdout, "\nentrypoint canary summary: %d/%d passed\n", len(results)-failed, len(results))
	if failed > 0 {
		return exitFail
	}
	return exitOK
}

func splitLanguages(spec string) []string {
	var out []string
	for _, tok := range strings.Split(spec, ",") {
		tok = strings.ToLower(strings.TrimSpace(tok))
		if tok != "" {
			out = append(out, tok)
		}
	}
	return out
}

func workerLimit(n int) int {
	if n < 1 {
		return 1
	}
	limit := runtime.GOMAXPROCS(0)
	if limit < 1 {
		limit = 1
	}
	if limit > maxLanguageWorkers {
		limit = maxLanguageWorkers
	}
	if limit > n {
		return n
	}
	return limit
}

func runLanguagePool(ctx context.Context, langs []string, limit int, timeout time.Duration, run func(context.Context, string) caseResult) []caseResult {
	results := make([]caseResult, len(langs))
	if limit < 1 {
		limit = 1
	}
	if timeout <= 0 {
		timeout = perLanguageTimeout
	}
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	for i := 0; i < len(langs); i++ {
		if err := ctx.Err(); err != nil {
			for j := i; j < len(langs); j++ {
				results[j] = cancelledResult(langs[j], err)
			}
			break
		}
		select {
		case <-ctx.Done():
			for j := i; j < len(langs); j++ {
				results[j] = cancelledResult(langs[j], ctx.Err())
			}
			i = len(langs)
			continue
		case sem <- struct{}{}:
		}
		wg.Add(1)
		go func(i int, lang string) {
			defer wg.Done()
			defer func() { <-sem }()
			defer func() {
				if rec := recover(); rec != nil {
					results[i] = caseResult{
						language: lang,
						target:   languageTargets[lang],
						passed:   false,
						detail:   fmt.Sprintf("internal error: %v", rec),
					}
				}
			}()
			langCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			res := run(langCtx, lang)
			if res.language == "" {
				res.language = lang
			}
			if res.target == "" {
				res.target = languageTargets[lang]
			}
			results[i] = res
		}(i, langs[i])
	}
	wg.Wait()
	return results
}

func cancelledResult(lang string, err error) caseResult {
	return caseResult{
		language: lang,
		target:   languageTargets[lang],
		passed:   false,
		detail:   err.Error(),
	}
}

func runLanguageCase(ctx context.Context, repoRoot, lang string, verifyBaseline bool) caseResult {
	switch lang {
	case "go":
		return runGoCase(ctx, repoRoot, verifyBaseline)
	case "python":
		return runPythonCase(ctx, repoRoot, verifyBaseline)
	case "rust":
		return runRustCase(ctx, repoRoot, verifyBaseline)
	case "frontend":
		return runFrontendCase(ctx, repoRoot, verifyBaseline)
	case "deno":
		return runDenoCase(ctx, repoRoot, verifyBaseline)
	case "proto":
		return runProtoCase(ctx, repoRoot, verifyBaseline)
	default:
		return caseResult{language: lang, passed: false, detail: "unknown language " + lang}
	}
}

func runGoCase(ctx context.Context, repoRoot string, verifyBaseline bool) caseResult {
	const target = "docker/otel-relay"
	start := time.Now()
	fail := func(detail string) caseResult {
		return caseResult{"go", target, false, time.Since(start), detail}
	}
	if err := ctx.Err(); err != nil {
		return fail(err.Error())
	}
	staged, cleanup, err := makeStage(ctx, "alt-ep-go-", repoRoot, target)
	if err != nil {
		return fail(err.Error())
	}
	defer cleanup()
	if verifyBaseline {
		base := runStagedLint(ctx, staged.root, target, nil)
		if base.err != nil || base.code != 0 {
			return fail(fmt.Sprintf("baseline failed (rc=%d): %s", base.code, snippetTail(base.combined())))
		}
	}
	violation := filepath.Join(staged.targetDir, "entrypoint_canary_violation.go")
	body := "package main\n\nimport \"crypto/tls\"\n\n" +
		"func entrypointCanaryViolation() *tls.Config {\n" +
		"    x:=1\n" +
		"\tx = 2\n" +
		"\t_ = x\n" +
		"\treturn &tls.Config{InsecureSkipVerify: true}\n" +
		"}\n"
	if err := os.WriteFile(violation, []byte(body), fsperm.File); err != nil {
		return fail(err.Error())
	}
	mut := runStagedLint(ctx, staged.root, target, nil)
	if mut.err != nil {
		return fail("lint execution error: " + mut.err.Error())
	}
	combined := mut.combined()
	missing := missingMarkers(combined, []string{
		"canary audit passed (go)",
		"Run gofmt -w on these files:",
		"entrypoint_canary_violation.go",
		"G402",
	})
	if mut.code == 0 || len(missing) > 0 {
		return fail(fmt.Sprintf("expected rc!=0 and markers (missing=%s, rc=%d): %s", pyList(missing), mut.code, snippetTail(combined)))
	}
	return caseResult{"go", target, true, time.Since(start), "baseline pass + gofmt and G402/ineffassign caught"}
}

func runPythonCase(ctx context.Context, repoRoot string, verifyBaseline bool) caseResult {
	const target = "rerank-server"
	start := time.Now()
	fail := func(detail string) caseResult {
		return caseResult{"python", target, false, time.Since(start), detail}
	}
	if err := ctx.Err(); err != nil {
		return fail(err.Error())
	}
	td, err := os.MkdirTemp("", "alt-ep-python-")
	if err != nil {
		return fail(err.Error())
	}
	defer os.RemoveAll(td)
	stagedRoot := filepath.Join(td, "repo")
	if err := os.MkdirAll(stagedRoot, fsperm.Dir); err != nil {
		return fail(err.Error())
	}
	if err := stageCommon(ctx, repoRoot, stagedRoot); err != nil {
		return fail(err.Error())
	}
	targetDir, err := stageTargetDir(ctx, repoRoot, stagedRoot, target)
	if err != nil {
		return fail(err.Error())
	}
	extra := map[string]string{
		"UV_PROJECT_ENVIRONMENT": filepath.Join(td, "venv"),
		"UV_LINK_MODE":           "copy",
	}
	if verifyBaseline {
		base := runStagedLint(ctx, stagedRoot, target, extra)
		if base.err != nil || base.code != 0 {
			return fail(fmt.Sprintf("baseline failed (rc=%d): %s", base.code, snippetTail(base.combined())))
		}
	}
	violation := filepath.Join(targetDir, "entrypoint_canary_violation.py")
	body := "\"\"\"Entrypoint canary injected violation.\"\"\"\n\n" +
		"import os\n\n\n" +
		"def entrypoint_canary_violation() -> int:\n" +
		"    bad_value: int = \"not_an_int\"\n" +
		"    return bad_value\n"
	if err := os.WriteFile(violation, []byte(body), fsperm.File); err != nil {
		return fail(err.Error())
	}
	mut := runStagedLint(ctx, stagedRoot, target, extra)
	if mut.err != nil {
		return fail("lint execution error: " + mut.err.Error())
	}
	combined := mut.combined()
	missing := missingMarkers(combined, []string{
		"canary audit passed (python)",
		"entrypoint_canary_violation.py",
		"F401",
		"bad-assignment",
	})
	if mut.code == 0 || len(missing) > 0 {
		return fail(fmt.Sprintf("expected rc!=0 and markers (missing=%s, rc=%d): %s", pyList(missing), mut.code, snippetTail(combined)))
	}
	return caseResult{"python", target, true, time.Since(start), "baseline pass + F401/bad-assignment caught"}
}

func runRustCase(ctx context.Context, repoRoot string, verifyBaseline bool) caseResult {
	const target = "plecto"
	start := time.Now()
	fail := func(detail string) caseResult {
		return caseResult{"rust", target, false, time.Since(start), detail}
	}
	if err := ctx.Err(); err != nil {
		return fail(err.Error())
	}
	shared := filepath.Join(cacheRoot(), "alt-lint", "entrypoint-canary-rust-target")
	if err := os.MkdirAll(shared, fsperm.Dir); err != nil {
		return fail(err.Error())
	}
	extra := map[string]string{"CARGO_TARGET_DIR": shared}
	staged, cleanup, err := makeStage(ctx, "alt-ep-rust-", repoRoot, target)
	if err != nil {
		return fail(err.Error())
	}
	defer cleanup()
	if verifyBaseline {
		base := runStagedLint(ctx, staged.root, target, extra)
		if base.err != nil || base.code != 0 {
			return fail(fmt.Sprintf("baseline failed (rc=%d): %s", base.code, snippetTail(base.combined())))
		}
	}
	libRS := filepath.Join(staged.targetDir, "src", "lib.rs")
	original, err := os.ReadFile(libRS)
	if err != nil {
		return fail(err.Error())
	}
	updated := strings.TrimRightFunc(string(original), unicode.IsSpace) + "\n\npub fn entrypoint_canary_violation(x: i32) -> bool {\n    x == x\n}\n"
	if err := os.WriteFile(libRS, []byte(updated), fsperm.File); err != nil {
		return fail(err.Error())
	}
	mut := runStagedLint(ctx, staged.root, target, extra)
	if mut.err != nil {
		return fail("lint execution error: " + mut.err.Error())
	}
	combined := mut.combined()
	missing := missingMarkers(combined, []string{"canary audit passed (rust)", "eq_op", "clippy: fail"})
	if mut.code == 0 || len(missing) > 0 {
		return fail(fmt.Sprintf("expected rc!=0 and markers (missing=%s, rc=%d): %s", pyList(missing), mut.code, snippetTail(combined)))
	}
	return caseResult{"rust", target, true, time.Since(start), "baseline pass + clippy::eq_op caught"}
}

func runFrontendCase(ctx context.Context, repoRoot string, verifyBaseline bool) caseResult {
	const target = "alt-frontend-sv"
	start := time.Now()
	fail := func(detail string) caseResult {
		return caseResult{"frontend", target, false, time.Since(start), detail}
	}
	if err := ctx.Err(); err != nil {
		return fail(err.Error())
	}
	nodeModules := resolveFrontendNodeModules(ctx, repoRoot, target)
	if nodeModules == "" {
		return fail(fmt.Sprintf("missing %s/node_modules (run `bun install --frozen-lockfile` in %s)", target, target))
	}
	pinnedBun := resolveLocalPinnedBun(ctx, filepath.Join(repoRoot, target, "Dockerfile"))
	baselineBun := pinnedBun
	if baselineBun == "" {
		baselineBun = look("bun")
	}
	if baselineBun == "" {
		return fail("bun executable not found (checked pinned cache and PATH)")
	}
	staged, cleanup, err := makeStage(ctx, "alt-ep-frontend-", repoRoot, target)
	if err != nil {
		return fail(err.Error())
	}
	defer cleanup()
	if err := os.Symlink(nodeModules, filepath.Join(staged.targetDir, "node_modules")); err != nil {
		return fail(err.Error())
	}
	if verifyBaseline {
		base := runCmd(ctx, baselineSubprocessTimeout, baselineBun, []string{"run", "lint:check"}, staged.targetDir, nil)
		if base.err != nil || base.code != 0 {
			return fail(fmt.Sprintf("baseline lint:check failed (rc=%d): %s", base.code, snippetTail(base.combined())))
		}
	}
	violation := filepath.Join(staged.targetDir, "src", "lib", "entrypoint_canary_violation.ts")
	if err := os.MkdirAll(filepath.Dir(violation), fsperm.Dir); err != nil {
		return fail(err.Error())
	}
	body := "export function entrypointCanaryViolation(value: number): number {\n\tdebugger;\n\treturn value;\n}\n"
	if err := os.WriteFile(violation, []byte(body), fsperm.File); err != nil {
		return fail(err.Error())
	}
	mut := runStagedLint(ctx, staged.root, target, nil)
	if mut.err != nil {
		return fail("lint execution error: " + mut.err.Error())
	}
	combined := mut.combined()
	pinNote := ""
	if pinnedBun == "" && os.Getenv("ALT_LINT_BUN_BIN") == "" && strings.Contains(strings.ToLower(combined), "failed to download bun") {
		note, alignErr := alignStagedBunPin(ctx, filepath.Join(staged.targetDir, "Dockerfile"))
		if alignErr != nil {
			return fail(alignErr.Error())
		}
		if note != "" {
			pinNote = note
			mut = runStagedLint(ctx, staged.root, target, nil)
			if mut.err != nil {
				return fail("lint execution error: " + mut.err.Error())
			}
			combined = mut.combined()
		}
	}
	missing := missingMarkers(combined, []string{
		"canary audit passed (frontend)",
		"entrypoint_canary_violation.ts",
		"noDebugger",
		"lint:check: fail",
	})
	if mut.code == 0 || len(missing) > 0 {
		return fail(fmt.Sprintf("expected rc!=0 and markers (missing=%s, rc=%d): %s", pyList(missing), mut.code, snippetTail(combined)))
	}
	suffix := ""
	if pinNote != "" {
		suffix = " (" + pinNote + ")"
	}
	return caseResult{"frontend", target, true, time.Since(start), "baseline lint:check pass + Biome noDebugger caught" + suffix}
}

func runDenoCase(ctx context.Context, repoRoot string, verifyBaseline bool) caseResult {
	const target = "auth-token-manager"
	start := time.Now()
	fail := func(detail string) caseResult {
		return caseResult{"deno", target, false, time.Since(start), detail}
	}
	if err := ctx.Err(); err != nil {
		return fail(err.Error())
	}
	staged, cleanup, err := makeStage(ctx, "alt-ep-deno-", repoRoot, target)
	if err != nil {
		return fail(err.Error())
	}
	defer cleanup()
	if verifyBaseline {
		base := runStagedLint(ctx, staged.root, target, nil)
		if base.err != nil || base.code != 0 {
			return fail(fmt.Sprintf("baseline failed (rc=%d): %s", base.code, snippetTail(base.combined())))
		}
	}
	violation := filepath.Join(staged.targetDir, "src", "entrypoint_canary_violation.ts")
	if err := os.MkdirAll(filepath.Dir(violation), fsperm.Dir); err != nil {
		return fail(err.Error())
	}
	body := "export function entrypointCanaryViolation(value: number): number {\n  debugger;\n  return value;\n}\n"
	if err := os.WriteFile(violation, []byte(body), fsperm.File); err != nil {
		return fail(err.Error())
	}
	mut := runStagedLint(ctx, staged.root, target, nil)
	if mut.err != nil {
		return fail("lint execution error: " + mut.err.Error())
	}
	combined := mut.combined()
	missing := missingMarkers(combined, []string{
		"canary audit passed (deno)",
		"entrypoint_canary_violation.ts",
		"no-debugger",
		"deno lint: fail",
	})
	if mut.code == 0 || len(missing) > 0 {
		return fail(fmt.Sprintf("expected rc!=0 and markers (missing=%s, rc=%d): %s", pyList(missing), mut.code, snippetTail(combined)))
	}
	return caseResult{"deno", target, true, time.Since(start), "baseline pass + deno no-debugger caught"}
}

func runProtoCase(ctx context.Context, repoRoot string, verifyBaseline bool) caseResult {
	const target = "proto"
	start := time.Now()
	fail := func(detail string) caseResult {
		return caseResult{"proto", target, false, time.Since(start), detail}
	}
	if err := ctx.Err(); err != nil {
		return fail(err.Error())
	}
	staged, cleanup, err := makeStage(ctx, "alt-ep-proto-", repoRoot, target)
	if err != nil {
		return fail(err.Error())
	}
	defer cleanup()
	for _, rel := range []string{
		"alt-butterfly-facade/internal/server/allowlist_gen.go",
		"alt-frontend-sv/src/lib/gen/allowlist.ts",
	} {
		src := filepath.Join(repoRoot, rel)
		if !isFile(src) {
			continue
		}
		dst := filepath.Join(staged.root, rel)
		if err := copyFile(ctx, src, dst); err != nil {
			return fail(err.Error())
		}
	}
	extra := map[string]string{
		"ALT_PROTO_BREAKING_AGAINST": filepath.Join(repoRoot, target),
	}
	if verifyBaseline {
		base := runStagedLint(ctx, staged.root, target, extra)
		if base.err != nil || base.code != 0 {
			return fail(fmt.Sprintf("baseline failed (rc=%d): %s", base.code, snippetTail(base.combined())))
		}
	}
	violation := filepath.Join(staged.targetDir, "entrypoint_canary_violation.proto")
	body := "syntax = \"proto3\";\n\npackage badpkg;\n\nenum bad_entrypoint_enum {\n  bad_entrypoint_value = 0;\n}\n"
	if err := os.WriteFile(violation, []byte(body), fsperm.File); err != nil {
		return fail(err.Error())
	}
	mut := runStagedLint(ctx, staged.root, target, extra)
	if mut.err != nil {
		return fail("lint execution error: " + mut.err.Error())
	}
	combined := mut.combined()
	missing := missingMarkers(combined, []string{
		"canary audit passed (proto)",
		"entrypoint_canary_violation.proto",
		"FAIL: buf-lint",
	})
	if mut.code == 0 || len(missing) > 0 {
		return fail(fmt.Sprintf("expected rc!=0 and markers (missing=%s, rc=%d): %s", pyList(missing), mut.code, snippetTail(combined)))
	}
	return caseResult{"proto", target, true, time.Since(start), "baseline pass + buf lint STANDARD caught"}
}

type stage struct {
	root      string
	targetDir string
}

func makeStage(ctx context.Context, prefix, repoRoot, target string) (stage, func(), error) {
	td, err := os.MkdirTemp("", prefix)
	if err != nil {
		return stage{}, nil, err
	}
	cleanup := func() { _ = os.RemoveAll(td) }
	if err := stageCommon(ctx, repoRoot, td); err != nil {
		cleanup()
		return stage{}, nil, err
	}
	targetDir, err := stageTargetDir(ctx, repoRoot, td, target)
	if err != nil {
		cleanup()
		return stage{}, nil, err
	}
	return stage{root: td, targetDir: targetDir}, cleanup, nil
}

func stageCommon(ctx context.Context, repoRoot, stagedRoot string) error {
	if err := copyTree(ctx, filepath.Join(repoRoot, "scripts", "lint"), filepath.Join(stagedRoot, "scripts", "lint")); err != nil {
		return err
	}
	if err := stageAltharness(ctx, repoRoot, stagedRoot); err != nil {
		return err
	}
	golangci := filepath.Join(repoRoot, ".golangci.yml")
	if isFile(golangci) {
		if err := copyFile(ctx, golangci, filepath.Join(stagedRoot, ".golangci.yml")); err != nil {
			return err
		}
	}
	return nil
}

func stageAltharness(ctx context.Context, repoRoot, stagedRoot string) error {
	srcMod := filepath.Join(repoRoot, "scripts", "altharness")
	if !isDir(srcMod) {
		return nil
	}
	dstMod := filepath.Join(stagedRoot, "scripts", "altharness")
	if err := os.MkdirAll(dstMod, fsperm.Dir); err != nil {
		return err
	}
	for _, rel := range []string{"alt-harness", "go.mod"} {
		src := filepath.Join(srcMod, rel)
		if isFile(src) {
			if err := copyFile(ctx, src, filepath.Join(dstMod, rel)); err != nil {
				return err
			}
		}
	}
	for _, sub := range []string{"cmd", "internal"} {
		srcSub := filepath.Join(srcMod, sub)
		if isDir(srcSub) {
			if err := copyTree(ctx, srcSub, filepath.Join(dstMod, sub)); err != nil {
				return err
			}
		}
	}
	prebuilt := filepath.Join(srcMod, "bin", "alt-harness")
	if isFile(prebuilt) {
		dstBinDir := filepath.Join(dstMod, "bin")
		if err := os.MkdirAll(dstBinDir, fsperm.Dir); err != nil {
			return err
		}
		if err := os.Symlink(prebuilt, filepath.Join(dstBinDir, "alt-harness")); err != nil {
			return err
		}
		stamp := filepath.Join(srcMod, "bin", ".alt-harness.stamp")
		if isFile(stamp) {
			if err := copyFile(ctx, stamp, filepath.Join(dstBinDir, ".alt-harness.stamp")); err != nil {
				return err
			}
		}
	}
	return nil
}

func stageTargetDir(ctx context.Context, repoRoot, stagedRoot, target string) (string, error) {
	dst := filepath.Join(stagedRoot, target)
	if err := copyTree(ctx, filepath.Join(repoRoot, target), dst); err != nil {
		return "", err
	}
	return dst, nil
}

func copyTree(ctx context.Context, src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stage %s: %w", src, err)
	}
	if !info.IsDir() {
		return copyFile(ctx, src, dst)
	}
	return filepath.WalkDir(src, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		name := d.Name()
		if rel != "." {
			if _, skip := copyIgnore[name]; skip {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, fsperm.Dir)
		}
		if d.Type()&os.ModeSymlink != 0 {
			return copyFile(ctx, path, target)
		}
		return copyFile(ctx, path, target)
	})
}

func copyFile(ctx context.Context, src, dst string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), fsperm.Dir); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	_ = os.Chtimes(dst, info.ModTime(), info.ModTime())
	return nil
}

func runStagedLint(ctx context.Context, stagedRoot, target string, extra map[string]string) cmdResult {
	bashBin := look("bash")
	if bashBin == "" {
		return cmdResult{code: 1, err: fmt.Errorf("bash executable not found in PATH")}
	}
	script := filepath.Join(stagedRoot, "scripts", "lint", "run.sh")
	return runCmd(ctx, lintSubprocessTimeout, bashBin, []string{script, target}, stagedRoot, mergedEnv(extra))
}

func missingMarkers(output string, markers []string) []string {
	clean := stripANSI(output)
	var missing []string
	for _, marker := range markers {
		if !strings.Contains(clean, marker) {
			missing = append(missing, marker)
		}
	}
	return missing
}

func isLanguageAvailable(ctx context.Context, repoRoot, lang string) bool {
	switch lang {
	case "go":
		return look("go") != "" && look("jq") != ""
	case "python":
		return look("uv") != ""
	case "rust":
		return look("cargo") != ""
	case "frontend":
		pinned := resolveLocalPinnedBun(ctx, filepath.Join(repoRoot, "alt-frontend-sv", "Dockerfile"))
		return (pinned != "" || look("bun") != "") && resolveFrontendNodeModules(ctx, repoRoot, "alt-frontend-sv") != ""
	case "deno":
		return look("deno") != ""
	case "proto":
		return look("buf") != "" && look("go") != ""
	default:
		return false
	}
}

func resolveFrontendNodeModules(ctx context.Context, repoRoot, target string) string {
	direct := filepath.Join(repoRoot, target, "node_modules")
	if isDir(direct) {
		return direct
	}
	if main := findMainWorktree(ctx, repoRoot); main != "" {
		fallback := filepath.Join(main, target, "node_modules")
		if isDir(fallback) {
			return fallback
		}
	}
	return ""
}

func findMainWorktree(ctx context.Context, repoRoot string) string {
	gitBin := look("git")
	if gitBin == "" {
		return ""
	}
	res := runCmd(ctx, quickSubprocessTimeout, gitBin, []string{"-C", repoRoot, "worktree", "list", "--porcelain"}, "", nil)
	if res.err != nil || res.code != 0 {
		return ""
	}
	for _, line := range strings.Split(res.stdout, "\n") {
		if !strings.HasPrefix(line, "worktree ") {
			continue
		}
		candidate := strings.TrimSpace(strings.TrimPrefix(line, "worktree "))
		if isDir(candidate) {
			return candidate
		}
	}
	return ""
}

func readDockerfileBunPin(dockerfile string) string {
	data, err := os.ReadFile(dockerfile)
	if err != nil {
		return ""
	}
	match := dockerfileBunPinRE.FindStringSubmatch(string(data))
	if match == nil {
		return ""
	}
	return match[2]
}

func bunVersion(ctx context.Context, bunBin string) string {
	res := runCmd(ctx, quickSubprocessTimeout, bunBin, []string{"--version"}, "", nil)
	if res.err != nil || res.code != 0 {
		return ""
	}
	version := strings.TrimSpace(res.stdout)
	return version
}

func executableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	return info.Mode().Perm()&0o111 != 0
}

func resolveLocalPinnedBun(ctx context.Context, dockerfile string) string {
	pinned := readDockerfileBunPin(dockerfile)
	if pinned == "" {
		return ""
	}
	if override := os.Getenv("ALT_LINT_BUN_BIN"); override != "" {
		if executableFile(override) && bunVersion(ctx, override) == pinned {
			return override
		}
		return ""
	}
	if pathBun := look("bun"); pathBun != "" && bunVersion(ctx, pathBun) == pinned {
		return pathBun
	}
	cached := filepath.Join(cacheRoot(), "alt-lint", "bun", pinned, "bun")
	if executableFile(cached) && bunVersion(ctx, cached) == pinned {
		return cached
	}
	return ""
}

func alignStagedBunPin(ctx context.Context, dockerfile string) (string, error) {
	bunBin := look("bun")
	if bunBin == "" || !isFile(dockerfile) {
		return "", nil
	}
	hostVer := bunVersion(ctx, bunBin)
	if hostVer == "" {
		return "", nil
	}
	data, err := os.ReadFile(dockerfile)
	if err != nil {
		return "", err
	}
	content := string(data)
	match := dockerfileBunPinRE.FindStringSubmatch(content)
	if match == nil || match[2] == hostVer {
		return "", nil
	}
	pinned := match[2]
	updated := dockerfileBunPinRE.ReplaceAllStringFunc(content, func(line string) string {
		sub := dockerfileBunPinRE.FindStringSubmatch(line)
		if sub == nil {
			return line
		}
		suffix := ""
		if len(sub) > 3 {
			suffix = sub[3]
		}
		return sub[1] + hostVer + suffix
	})
	if err := os.WriteFile(dockerfile, []byte(updated), fsperm.File); err != nil {
		return "", err
	}
	return fmt.Sprintf("staged Dockerfile bun pin aligned %s->%s for offline host bun", pinned, hostVer), nil
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func defaultRepoRoot() (string, error) {
	if _, file, _, ok := runtime.Caller(0); ok {
		if root, found := findRepoRoot(filepath.Dir(file)); found {
			return root, nil
		}
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("cannot locate repository root (pass --repo-root): %w", err)
	}
	if root, found := findRepoRoot(cwd); found {
		return root, nil
	}
	return "", fmt.Errorf("cannot locate repository root (pass --repo-root)")
}

func findRepoRoot(start string) (string, bool) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", false
	}
	for {
		marker := filepath.Join(dir, "scripts", "lint", "run.sh")
		if isFile(marker) {
			return resolveExisting(dir), true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}
