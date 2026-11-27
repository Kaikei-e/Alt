package flakyhunt

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	goShuffleSeedRE = regexp.MustCompile(`^-test\.shuffle[= ](\d+)\b`)
	goFailLineRE    = regexp.MustCompile(`^\s*--- FAIL: (\S+)`)
)

type goTestEvent struct {
	Time    string  `json:"Time"`
	Action  string  `json:"Action"`
	Package string  `json:"Package"`
	Test    string  `json:"Test"`
	Output  string  `json:"Output"`
	Elapsed float64 `json:"Elapsed"`
}

func runGoHunt(ctx context.Context, cfg config, relModule, modDir, evidenceDir string, baseEnv map[string]string, rep *Report) error {
	goBin, err := exec.LookPath("go")
	if err != nil {
		return fmt.Errorf("go executable not found on PATH: %w", err)
	}

	modImportPath := readGoModulePath(filepath.Join(modDir, "go.mod"))
	pkgs := cfg.pkgs
	if len(pkgs) == 0 {
		pkgs = []string{defaultGoPkg}
	}

	shuffleArg := "on"
	if cfg.seed != "" {
		shuffleArg = cfg.seed
	}

	envMap := make(map[string]string, len(baseEnv)+1)
	for k, v := range baseEnv {
		envMap[k] = v
	}
	if cfg.stressProcs > 0 {
		envMap["GOMAXPROCS"] = strconv.Itoa(cfg.stressProcs)
	}
	envSlice := envMapToSlice(envMap)

	for round := 1; round <= cfg.rounds; round++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		cmdArgs := []string{
			"test",
			"-json",
			fmt.Sprintf("-count=%d", cfg.count),
			fmt.Sprintf("-shuffle=%s", shuffleArg),
		}
		if cfg.race {
			cmdArgs = append(cmdArgs, "-race")
		}
		cmdArgs = append(cmdArgs, cfg.extraArgs...)
		cmdArgs = append(cmdArgs, pkgs...)

		roundStart := time.Now()
		var stdoutBuf, stderrBuf bytes.Buffer
		cmd := exec.CommandContext(ctx, goBin, cmdArgs...)
		cmd.Dir = modDir
		cmd.Env = envSlice
		cmd.Stdout = &stdoutBuf
		cmd.Stderr = &stderrBuf

		exitCode := 0
		runErr := cmd.Run()
		if runErr != nil {
			var exitErr *exec.ExitError
			if errors.As(runErr, &exitErr) {
				exitCode = exitErr.ExitCode()
			} else {
				return fmt.Errorf("round %d go test invocation failed: %w", round, runErr)
			}
		}

		parsed := parseGoTestJSON(stdoutBuf.String(), stderrBuf.String(), exitCode, round, relModule, modImportPath, pkgs, cfg)
		summary := RoundSummary{
			Round:        round,
			ExitCode:     exitCode,
			Passed:       exitCode == 0 && len(parsed.failures) == 0,
			DurationMS:   time.Since(roundStart).Milliseconds(),
			PackageSeeds: parsed.packageSeeds,
		}
		if len(parsed.packageSeeds) == 1 {
			for _, s := range parsed.packageSeeds {
				summary.Seed = s
			}
		} else if cfg.seed != "" && cfg.seed != "on" {
			summary.Seed = cfg.seed
		}

		if !summary.Passed {
			rep.FailedRounds++
			logPath, lerr := writeFailureLog(evidenceDir, round, parsed.rawLog)
			if lerr != nil {
				return fmt.Errorf("write failure log for round %d: %w", round, lerr)
			}
			summary.LogPath = logPath
			if len(parsed.failures) == 0 {
				seed := summary.Seed
				if seed == "" {
					seed = firstSeed(parsed.packageSeeds)
				}
				pkgName := strings.Join(pkgs, " ")
				parsed.failures = append(parsed.failures, Failure{
					Round:        round,
					Seed:         seed,
					TestName:     "<package>",
					Package:      pkgName,
					Kind:         "exit_error",
					ReproCommand: buildGoReproCommand(relModule, pkgName, seed, cfg),
					Snippet:      trimSnippet(parsed.rawLog),
				})
			}
			for i := range parsed.failures {
				parsed.failures[i].LogPath = logPath
				rep.Failures = append(rep.Failures, parsed.failures[i])
			}
		} else {
			rep.PassedRounds++
		}
		rep.RoundResults = append(rep.RoundResults, summary)
	}

	return nil
}

type goRoundParse struct {
	packageSeeds map[string]string
	failures     []Failure
	rawLog       string
}

type failKey struct {
	pkg  string
	test string
}

func parseGoTestJSON(stdoutText, stderrText string, exitCode, round int, relModule, modImportPath string, pkgs []string, cfg config) goRoundParse {
	pkgSeeds := make(map[string]string)
	lastRunningTest := make(map[string]string)
	pkgFailed := make(map[string]bool)
	pkgPassed := make(map[string]bool)
	pkgBuildFailed := make(map[string]bool)
	pkgSnippets := make(map[string][]string)
	pkgLogs := make(map[string]*strings.Builder)
	var pkgLogOrder []string

	appendPkgLog := func(pkg, text string) {
		b, ok := pkgLogs[pkg]
		if !ok {
			b = &strings.Builder{}
			pkgLogs[pkg] = b
			pkgLogOrder = append(pkgLogOrder, pkg)
		}
		b.WriteString(text)
		if !strings.HasSuffix(text, "\n") {
			b.WriteByte('\n')
		}
	}

	// Track failures keyed by (pkg, test) preserving discovery order.
	var failOrder []failKey
	failMap := make(map[failKey]*Failure)
	var pendingOutputFails []struct {
		pkg     string
		test    string
		kind    string
		snippet string
	}

	addOrUpgrade := func(pkg, testName, kind, snippetLine string) {
		if pkg == "" {
			pkg = strings.Join(pkgs, " ")
		}
		if testName == "" {
			if cur := lastRunningTest[pkg]; cur != "" {
				testName = cur
			} else {
				testName = "<package>"
			}
		}
		key := failKey{pkg: pkg, test: testName}
		existing, ok := failMap[key]
		if !ok {
			f := &Failure{
				Round:    round,
				TestName: testName,
				Package:  pkg,
				Kind:     kind,
				Snippet:  strings.TrimSpace(snippetLine),
			}
			failMap[key] = f
			failOrder = append(failOrder, key)
			return
		}
		// Upgrade generic test_fail to more specific root kind (data_race / panic / timeout).
		if kindPriority(kind) > kindPriority(existing.Kind) {
			existing.Kind = kind
		}
		if existing.Snippet == "" && strings.TrimSpace(snippetLine) != "" {
			existing.Snippet = strings.TrimSpace(snippetLine)
		}
	}

	var renderedLog strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(stdoutText))
	// Allow long Go test output lines (e.g. big diffs or stack traces).
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		var ev goTestEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			renderedLog.WriteString(line)
			renderedLog.WriteByte('\n')
			continue
		}

		if ev.Output != "" {
			renderedLog.WriteString(ev.Output)
			if !strings.HasSuffix(ev.Output, "\n") {
				renderedLog.WriteByte('\n')
			}
			if ev.Package != "" {
				appendPkgLog(ev.Package, ev.Output)
			}
		}

		switch ev.Action {
		case "run":
			if ev.Package != "" && ev.Test != "" {
				lastRunningTest[ev.Package] = ev.Test
			}
		case "pass":
			if ev.Package != "" {
				if ev.Test == "" {
					pkgPassed[ev.Package] = true
				} else if lastRunningTest[ev.Package] == ev.Test {
					delete(lastRunningTest, ev.Package)
				}
			}
		case "skip":
			if ev.Package != "" && ev.Test != "" && lastRunningTest[ev.Package] == ev.Test {
				delete(lastRunningTest, ev.Package)
			}
		case "fail":
			if ev.Test != "" {
				addOrUpgrade(ev.Package, ev.Test, "test_fail", "")
				if lastRunningTest[ev.Package] == ev.Test {
					delete(lastRunningTest, ev.Package)
				}
			} else if ev.Package != "" {
				pkgFailed[ev.Package] = true
			}
		case "build-fail":
			if ev.Package != "" {
				pkgBuildFailed[ev.Package] = true
			}
		case "output", "build-output":
			trimmed := strings.TrimSpace(ev.Output)
			if m := goShuffleSeedRE.FindStringSubmatch(trimmed); len(m) == 2 && ev.Package != "" {
				pkgSeeds[ev.Package] = m[1]
			}
			if m := goFailLineRE.FindStringSubmatch(ev.Output); len(m) == 2 {
				addOrUpgrade(ev.Package, m[1], "test_fail", trimmed)
			}
			if strings.Contains(ev.Output, "WARNING: DATA RACE") {
				testName := ev.Test
				if testName == "" {
					testName = lastRunningTest[ev.Package]
				}
				pendingOutputFails = append(pendingOutputFails, struct {
					pkg, test, kind, snippet string
				}{ev.Package, testName, "data_race", trimmed})
			}
			if strings.HasPrefix(trimmed, "panic:") || strings.HasPrefix(trimmed, "fatal error:") {
				testName := ev.Test
				if testName == "" {
					testName = lastRunningTest[ev.Package]
				}
				pendingOutputFails = append(pendingOutputFails, struct {
					pkg, test, kind, snippet string
				}{ev.Package, testName, "panic", trimmed})
			}
			if strings.Contains(trimmed, "test timed out after") {
				testName := ev.Test
				if testName == "" {
					testName = lastRunningTest[ev.Package]
				}
				pendingOutputFails = append(pendingOutputFails, struct {
					pkg, test, kind, snippet string
				}{ev.Package, testName, "timeout", trimmed})
			}
			if ev.Package != "" && trimmed != "" && len(pkgSnippets[ev.Package]) < 40 {
				pkgSnippets[ev.Package] = append(pkgSnippets[ev.Package], trimmed)
			}
		}
	}

	// Only apply output-line heuristics (panic/race/timeout) for packages that
	// actually failed or when the overall go test process exited non-zero without
	// that package passing, avoiding false positives from passing tests that log
	// strings starting with "panic:".
	for _, item := range pendingOutputFails {
		if item.kind == "data_race" || pkgFailed[item.pkg] || pkgBuildFailed[item.pkg] || (exitCode != 0 && !pkgPassed[item.pkg]) {
			addOrUpgrade(item.pkg, item.test, item.kind, item.snippet)
		}
	}

	if strings.TrimSpace(stderrText) != "" {
		renderedLog.WriteString("--- STDERR ---\n")
		renderedLog.WriteString(stderrText)
		if !strings.HasSuffix(stderrText, "\n") {
			renderedLog.WriteByte('\n')
		}
		if exitCode != 0 {
			for _, line := range strings.Split(stderrText, "\n") {
				trimmed := strings.TrimSpace(line)
				if strings.Contains(trimmed, "WARNING: DATA RACE") {
					addOrUpgrade("", "", "data_race", trimmed)
				} else if strings.HasPrefix(trimmed, "panic:") || strings.HasPrefix(trimmed, "fatal error:") {
					addOrUpgrade("", "", "panic", trimmed)
				}
			}
		}
	}

	for pkg := range pkgBuildFailed {
		addOrUpgrade(pkg, "<build>", "build_error", strings.Join(pkgSnippets[pkg], " | "))
	}

	// Ensure any failed package without a specific test failure still records a failure.
	hasPkgFailure := make(map[string]bool)
	for k := range failMap {
		hasPkgFailure[k.pkg] = true
	}
	failedPkgs := make([]string, 0, len(pkgFailed))
	for pkg := range pkgFailed {
		failedPkgs = append(failedPkgs, pkg)
	}
	sort.Strings(failedPkgs)
	for _, pkg := range failedPkgs {
		if !hasPkgFailure[pkg] {
			addOrUpgrade(pkg, "<package>", "test_fail", strings.Join(pkgSnippets[pkg], " | "))
		}
	}

	// Prune parent test entries when a child subtest (Parent/Sub) also failed in the same package.
	hasChildSubtest := make(map[failKey]bool)
	for _, k := range failOrder {
		if idx := strings.IndexByte(k.test, '/'); idx > 0 {
			parent := k.test[:idx]
			hasChildSubtest[failKey{pkg: k.pkg, test: parent}] = true
		}
	}

	var failures []Failure
	for _, k := range failOrder {
		f := failMap[k]
		if hasChildSubtest[k] && f.Kind == "test_fail" {
			continue
		}
		seed := pkgSeeds[f.Package]
		if seed == "" && cfg.seed != "" && cfg.seed != "on" {
			seed = cfg.seed
		}
		f.Seed = seed
		relPkg := packageToRelative(f.Package, modImportPath, pkgs)
		f.ReproCommand = buildGoReproCommand(relModule, relPkg, seed, cfg)
		if len(f.Snippet) > 300 {
			f.Snippet = f.Snippet[:300]
		}
		failures = append(failures, *f)
	}

	// Build a focused failure log containing only failing packages + stderr when
	// possible, keeping multi-package module failure logs concise and readable.
	var focusedLog strings.Builder
	for _, pkg := range pkgLogOrder {
		if pkgFailed[pkg] || pkgBuildFailed[pkg] || hasPkgFailure[pkg] {
			focusedLog.WriteString("=== PACKAGE: " + pkg + " ===\n")
			focusedLog.WriteString(pkgLogs[pkg].String())
		}
	}
	if strings.TrimSpace(stderrText) != "" {
		focusedLog.WriteString("--- STDERR ---\n")
		focusedLog.WriteString(stderrText)
		if !strings.HasSuffix(stderrText, "\n") {
			focusedLog.WriteByte('\n')
		}
	}
	rawLog := focusedLog.String()
	if strings.TrimSpace(rawLog) == "" {
		rawLog = renderedLog.String()
	}

	return goRoundParse{
		packageSeeds: pkgSeeds,
		failures:     failures,
		rawLog:       rawLog,
	}
}

func kindPriority(kind string) int {
	switch kind {
	case "data_race":
		return 4
	case "panic":
		return 3
	case "timeout":
		return 2
	case "build_error":
		return 2
	case "test_fail":
		return 1
	default:
		return 0
	}
}

func readGoModulePath(goModPath string) string {
	raw, err := os.ReadFile(goModPath)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "module ") {
			fields := strings.Fields(trimmed)
			if len(fields) >= 2 {
				return strings.Trim(fields[1], `"`)
			}
		}
	}
	return ""
}

func packageToRelative(pkg, modImportPath string, fallbackPkgs []string) string {
	if pkg != "" && modImportPath != "" {
		if pkg == modImportPath {
			return "."
		}
		if strings.HasPrefix(pkg, modImportPath+"/") {
			return "./" + strings.TrimPrefix(pkg, modImportPath+"/")
		}
	}
	if pkg != "" && strings.HasPrefix(pkg, "./") {
		return pkg
	}
	if len(fallbackPkgs) > 0 {
		return strings.Join(fallbackPkgs, " ")
	}
	return defaultGoPkg
}

func buildGoReproCommand(relModule, relPkg, seed string, cfg config) string {
	var parts []string
	if cfg.stressProcs > 0 {
		parts = append(parts, fmt.Sprintf("GOMAXPROCS=%d", cfg.stressProcs))
	}
	parts = append(parts, "go", "-C", relModule, "test", fmt.Sprintf("-count=%d", cfg.count))
	if seed != "" {
		parts = append(parts, fmt.Sprintf("-shuffle=%s", seed))
	} else {
		parts = append(parts, "-shuffle=on")
	}
	if cfg.race {
		parts = append(parts, "-race")
	}
	parts = append(parts, cfg.extraArgs...)
	parts = append(parts, relPkg)
	return strings.Join(parts, " ")
}

func envMapToSlice(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+env[k])
	}
	return out
}

func firstSeed(m map[string]string) string {
	if len(m) == 0 {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return m[keys[0]]
}

func trimSnippet(s string) string {
	trimmed := strings.TrimSpace(s)
	if len(trimmed) > 300 {
		return trimmed[:300]
	}
	return trimmed
}
