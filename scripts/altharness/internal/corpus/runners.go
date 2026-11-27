package corpus

import (
	"alt/scripts/altharness/internal/canary"
	"alt/scripts/altharness/internal/fsperm"
	"alt/scripts/altharness/internal/pyver"
	"alt/scripts/altharness/internal/suppress"
	"alt/scripts/altharness/internal/verifylive"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func adaptSingle(fn func(context.Context, Case, string, string, time.Duration) Outcome) func(context.Context, Case, string, string, int) Outcome {
	return func(ctx context.Context, c Case, phase, repoRoot string, timeoutSecs int) Outcome {
		if timeoutSecs <= 0 {
			timeoutSecs = SubprocessTimeoutSecond
		}
		return fn(ctx, c, phase, repoRoot, time.Duration(timeoutSecs)*time.Second)
	}
}

var builtinRunners = map[string]runner{
	"pyrefly":   {mode: "after", runSingle: adaptSingle(runPyrefly)},
	"REPO-02":   {mode: "after", runSingle: adaptSingle(runRepo02)},
	"REPO-03":   {mode: "after", runSingle: adaptSingle(runRepo03)},
	"ARCH-02":   {mode: "after", runSingle: adaptSingle(runArch02)},
	"ARCH-03":   {mode: "after", runSingle: adaptSingle(runArch03)},
	"ARCH-06":   {mode: "after", runSingle: adaptSingle(runArch06)},
	"CHECK-01":  {mode: "diff", runDiff: runCheck01Diff},
	"CHECK-05":  {mode: "diff", runDiff: runCheck05Diff},
	"CHECK-06":  {mode: "diff", runDiff: runCheck06Diff},
	"VERIFY-01": {mode: "diff", runDiff: runVerify01Diff},
	"VERIFY-03": {mode: "diff", runDiff: runVerify03Diff},
	"VERIFY-04": {mode: "after", runSingle: adaptSingle(runVerify04)},
}

func phaseTarget(c Case, phase string) string {
	if phase == "before" {
		return c.BeforeFile
	}
	return c.AfterFile
}

func regularFile(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0
}

func resolvePyrefly(repoRoot string) (string, string) {
	bin, errMsg := lookExecutable("PYREFLY_BIN", "pyrefly", nil)
	if errMsg != "" || bin != "" {
		return bin, errMsg
	}
	roots := []string{repoRoot}
	if main := findMainWorktree(repoRoot); main != "" {
		if a, errA := filepath.EvalSymlinks(repoRoot); errA == nil {
			repoRoot = a
		}
		if b, errB := filepath.EvalSymlinks(main); errB == nil {
			main = b
		}
		if main != repoRoot {
			roots = append(roots, main)
		}
	}
	var extras []string
	for _, root := range roots {
		for _, svc := range pyreflyFallbackServices {
			extras = append(extras, filepath.Join(root, svc, ".venv", "bin", "pyrefly"))
		}
	}
	bin, errMsg = lookExecutable("PYREFLY_BIN", "pyrefly", extras)
	if bin != "" {
		return bin, ""
	}
	return "", "pyrefly executable not found (checked PYREFLY_BIN, PATH, and service .venv/bin)"
}

func findMainWorktree(repoRoot string) string {
	git, err := exec.LookPath("git")
	if err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stdout, _, code, errMsg := runCaptured(ctx, 10*time.Second, "", "git worktree", []string{git, "-C", repoRoot, "worktree", "list", "--porcelain"})
	if errMsg != "" || code != 0 {
		return ""
	}
	for _, line := range strings.Split(stdout, "\n") {
		if strings.HasPrefix(line, "worktree ") {
			candidate := strings.TrimSpace(strings.TrimPrefix(line, "worktree "))
			if info, err := os.Stat(candidate); err == nil && info.IsDir() {
				return candidate
			}
		}
	}
	return ""
}

func runPyrefly(ctx context.Context, c Case, phase, repoRoot string, timeout time.Duration) CheckOutcome {
	bin, errMsg := resolvePyrefly(repoRoot)
	if bin == "" {
		return CheckOutcome{ExitCode: -1, RunnerError: errMsg}
	}
	tmp, err := os.MkdirTemp("", "alt-corpus-pyrefly-")
	if err != nil {
		return CheckOutcome{ExitCode: -1, RunnerError: fmt.Sprintf("pyrefly exec error: %v", err)}
	}
	defer os.RemoveAll(tmp)
	target := phaseTarget(c, phase)
	if err := copyFile(filepath.Join(c.CaseDir, target), filepath.Join(tmp, "sample.py")); err != nil {
		return CheckOutcome{ExitCode: -1, RunnerError: fmt.Sprintf("pyrefly exec error: %v", err)}
	}
	for _, sf := range c.SupportFiles {
		if err := copyFile(filepath.Join(c.CaseDir, sf), filepath.Join(tmp, sf)); err != nil {
			return CheckOutcome{ExitCode: -1, RunnerError: fmt.Sprintf("pyrefly exec error: %v", err)}
		}
	}
	cfg := filepath.Join(tmp, "pyrefly.toml")
	if _, ok := c.SupportSet["pyrefly.toml"]; !ok {
		cfg = filepath.Join(tmp, "pyproject.toml")
		if err := os.WriteFile(cfg, []byte("[tool.pyrefly]\npython-version = \"3.14\"\n"), fsperm.File); err != nil {
			return CheckOutcome{ExitCode: -1, RunnerError: fmt.Sprintf("pyrefly exec error: %v", err)}
		}
	}
	return runSimple(ctx, timeout, tmp, "pyrefly", []string{bin, "check", "--config", cfg, filepath.Join(tmp, "sample.py")})
}

func runRepo02(ctx context.Context, c Case, phase, _ string, timeout time.Duration) CheckOutcome {
	var missing []string
	for _, sf := range repo02RequiredSupport {
		if _, ok := c.SupportSet[sf]; !ok {
			missing = append(missing, sf)
		}
	}
	if len(missing) > 0 {
		return CheckOutcome{ExitCode: -1, RunnerError: fmt.Sprintf("REPO-02 case %s missing required support_files: %s", c.CaseID, pyList(missing))}
	}
	gosecName, sarifName := c.BeforeFile, "before.sarif"
	if phase != "before" {
		gosecName, sarifName = c.AfterFile, "after.sarif"
	}
	tmp, err := os.MkdirTemp("", "alt-corpus-repo02-")
	if err != nil {
		return CheckOutcome{ExitCode: -1, RunnerError: fmt.Sprintf("REPO-02 exec error: %v", err)}
	}
	defer os.RemoveAll(tmp)
	proj := filepath.Join(tmp, "proj")
	if err := os.MkdirAll(proj, fsperm.Dir); err != nil {
		return CheckOutcome{ExitCode: -1, RunnerError: fmt.Sprintf("REPO-02 exec error: %v", err)}
	}
	if err := copyFile(filepath.Join(c.CaseDir, "sample.go"), filepath.Join(proj, "sample.go")); err != nil {
		return CheckOutcome{ExitCode: -1, RunnerError: fmt.Sprintf("REPO-02 exec error: %v", err)}
	}
	args := []string{
		"go-verify-scan",
		"--project-dir", proj,
		"--gosec-out", filepath.Join(c.CaseDir, gosecName),
		"--gosec-sarif", filepath.Join(c.CaseDir, sarifName),
	}
	return runInProcess(ctx, timeout, "REPO-02", func(cctx context.Context, stdout, stderr io.Writer) int {
		return canary.RunAudit(cctx, args, stdout, stderr)
	})
}

func normalizeRepo03Dest(targetName, suffix string) string {
	for _, pat := range repo03ConfigPatterns {
		if pat.re.MatchString(targetName) {
			return pat.name
		}
	}
	return "sample" + suffix
}

func runRepo03(ctx context.Context, c Case, phase, repoRoot string, timeout time.Duration) CheckOutcome {
	allow := filepath.Join(repoRoot, "scripts", "lint", "suppression_allowlist.toml")
	git, gitErr := exec.LookPath("git")
	if !regularFile(allow) || gitErr != nil {
		return CheckOutcome{ExitCode: -1, RunnerError: "REPO-03 prerequisites missing (allowlist or git)"}
	}
	target := phaseTarget(c, phase)
	targetSrc := filepath.Join(c.CaseDir, target)
	tmp, err := os.MkdirTemp("", "alt-corpus-repo03-")
	if err != nil {
		return CheckOutcome{ExitCode: -1, RunnerError: fmt.Sprintf("REPO-03 exec error: %v", err)}
	}
	defer os.RemoveAll(tmp)
	init := runSimple(ctx, timeout, "", "REPO-03 git init", []string{git, "-C", tmp, "init", "-q"})
	if init.RunnerError != "" {
		return init
	}
	if init.ExitCode != 0 {
		return CheckOutcome{ExitCode: -1, RunnerError: "git init failed: " + snippet(init.Output)}
	}
	adrParent := filepath.Join(tmp, "docs")
	if err := os.MkdirAll(adrParent, fsperm.Dir); err != nil {
		return CheckOutcome{ExitCode: -1, RunnerError: fmt.Sprintf("REPO-03 exec error: %v", err)}
	}
	realADR := filepath.Join(repoRoot, "docs", "ADR")
	if info, err := os.Stat(realADR); err == nil && info.IsDir() {
		if err := os.Symlink(realADR, filepath.Join(adrParent, "ADR")); err != nil {
			return CheckOutcome{ExitCode: -1, RunnerError: fmt.Sprintf("REPO-03 exec error: %v", err)}
		}
	} else {
		fallback := filepath.Join(adrParent, "ADR")
		if err := os.MkdirAll(fallback, fsperm.Dir); err != nil {
			return CheckOutcome{ExitCode: -1, RunnerError: fmt.Sprintf("REPO-03 exec error: %v", err)}
		}
		if err := os.WriteFile(filepath.Join(fallback, "000979.md"), []byte("# ADR-000979\n"), fsperm.File); err != nil {
			return CheckOutcome{ExitCode: -1, RunnerError: fmt.Sprintf("REPO-03 exec error: %v", err)}
		}
	}
	pkg := filepath.Join(tmp, "pkg")
	if err := os.MkdirAll(pkg, fsperm.Dir); err != nil {
		return CheckOutcome{ExitCode: -1, RunnerError: fmt.Sprintf("REPO-03 exec error: %v", err)}
	}
	if err := copyFile(targetSrc, filepath.Join(pkg, normalizeRepo03Dest(target, filepath.Ext(targetSrc)))); err != nil {
		return CheckOutcome{ExitCode: -1, RunnerError: fmt.Sprintf("REPO-03 exec error: %v", err)}
	}
	for _, sf := range c.SupportFiles {
		if err := copyFile(filepath.Join(c.CaseDir, sf), filepath.Join(pkg, sf)); err != nil {
			return CheckOutcome{ExitCode: -1, RunnerError: fmt.Sprintf("REPO-03 exec error: %v", err)}
		}
	}
	args := []string{"--repo-root", tmp, "--allowlist", allow}
	return runInProcess(ctx, timeout, "REPO-03", func(cctx context.Context, stdout, stderr io.Writer) int {
		return suppress.RunCheck(cctx, args, stdout, stderr)
	})
}

func arch02RulePaths(language, repoRoot string) ([]string, string) {
	rulesDir := filepath.Join(repoRoot, "scripts", "lint", "ast-grep", "rules")
	switch language {
	case "go":
		matches, err := filepath.Glob(filepath.Join(rulesDir, "go-*.yml"))
		if err != nil || len(matches) == 0 {
			return nil, fmt.Sprintf("no go-*.yml ast-grep rules found in %s", rulesDir)
		}
		return matches, ""
	case "python":
		return []string{filepath.Join(rulesDir, "python-no-manual-aclose-in-finally.yml")}, ""
	case "rust":
		return []string{filepath.Join(rulesDir, "rust-no-silent-option-regex.yml")}, ""
	default:
		return nil, fmt.Sprintf("ARCH-02 has no ast-grep rules configured for language %s", pyRepr(language))
	}
}

func resolveAstGrep() (string, string) {
	var extras []string
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		extras = append(extras, filepath.Join(home, ".local", "bin", "ast-grep"))
	}
	bin, errMsg := lookExecutable("AST_GREP_BIN", "ast-grep", extras)
	if bin != "" {
		return bin, ""
	}
	if errMsg != "" {
		return "", errMsg
	}
	return "", "ast-grep executable not found (checked AST_GREP_BIN, PATH, and ~/.local/bin/ast-grep)"
}

func runAstGrep(ctx context.Context, c Case, phase string, rulePaths []string, timeout time.Duration) CheckOutcome {
	bin, errMsg := resolveAstGrep()
	if bin == "" {
		return CheckOutcome{ExitCode: -1, RunnerError: errMsg}
	}
	var missing []string
	for _, rp := range rulePaths {
		if !regularFile(rp) {
			missing = append(missing, rp)
		}
	}
	if len(rulePaths) == 0 || len(missing) > 0 {
		return CheckOutcome{ExitCode: -1, RunnerError: fmt.Sprintf("missing ast-grep rule file(s): %s", pyList(missing))}
	}
	target := phaseTarget(c, phase)
	tmp, err := os.MkdirTemp("", "alt-corpus-astgrep-")
	if err != nil {
		return CheckOutcome{ExitCode: -1, RunnerError: fmt.Sprintf("ast-grep exec error: %v", err)}
	}
	defer os.RemoveAll(tmp)
	srcDir := filepath.Join(tmp, "src")
	rulesDir := filepath.Join(tmp, "rules")
	if err := os.MkdirAll(srcDir, fsperm.Dir); err != nil || os.MkdirAll(rulesDir, fsperm.Dir) != nil {
		return CheckOutcome{ExitCode: -1, RunnerError: fmt.Sprintf("ast-grep exec error: %v", err)}
	}
	if err := copyFile(filepath.Join(c.CaseDir, target), filepath.Join(srcDir, target)); err != nil {
		return CheckOutcome{ExitCode: -1, RunnerError: fmt.Sprintf("ast-grep exec error: %v", err)}
	}
	for _, sf := range c.SupportFiles {
		if err := copyFile(filepath.Join(c.CaseDir, sf), filepath.Join(srcDir, sf)); err != nil {
			return CheckOutcome{ExitCode: -1, RunnerError: fmt.Sprintf("ast-grep exec error: %v", err)}
		}
	}
	for _, rp := range rulePaths {
		if err := copyFile(rp, filepath.Join(rulesDir, filepath.Base(rp))); err != nil {
			return CheckOutcome{ExitCode: -1, RunnerError: fmt.Sprintf("ast-grep exec error: %v", err)}
		}
	}
	if err := os.WriteFile(filepath.Join(tmp, "sgconfig.yml"), []byte("ruleDirs:\n  - rules\n"), fsperm.File); err != nil {
		return CheckOutcome{ExitCode: -1, RunnerError: fmt.Sprintf("ast-grep exec error: %v", err)}
	}
	return runSimple(ctx, timeout, tmp, "ast-grep", []string{bin, "scan", "-c", filepath.Join(tmp, "sgconfig.yml"), "src"})
}

func runArch02(ctx context.Context, c Case, phase, repoRoot string, timeout time.Duration) CheckOutcome {
	paths, errMsg := arch02RulePaths(c.Language, repoRoot)
	if errMsg != "" {
		return CheckOutcome{ExitCode: -1, RunnerError: errMsg}
	}
	return runAstGrep(ctx, c, phase, paths, timeout)
}

func selectPhasePyproject(c Case, phase string) string {
	for _, name := range []string{
		phase + ".pyproject.toml",
		phase + "_pyproject.toml",
		phase + "-pyproject.toml",
		"pyproject.toml",
	} {
		if _, ok := c.SupportSet[name]; ok {
			return name
		}
	}
	return ""
}

func runArch03(ctx context.Context, c Case, phase, _ string, timeout time.Duration) CheckOutcome {
	tmp, err := os.MkdirTemp("", "alt-corpus-arch03-")
	if err != nil {
		return CheckOutcome{ExitCode: -1, RunnerError: fmt.Sprintf("ARCH-03 exec error: %v", err)}
	}
	defer os.RemoveAll(tmp)
	target := phaseTarget(c, phase)
	targetSrc := filepath.Join(c.CaseDir, target)
	other := "after"
	if phase != "before" {
		other = "before"
	}
	pyprojectDest := filepath.Join(tmp, "pyproject.toml")
	var sources []string
	if filepath.Ext(targetSrc) == ".toml" {
		if err := copyFile(targetSrc, pyprojectDest); err != nil {
			return CheckOutcome{ExitCode: -1, RunnerError: fmt.Sprintf("ARCH-03 exec error: %v", err)}
		}
	} else {
		name := selectPhasePyproject(c, phase)
		if name == "" {
			return CheckOutcome{ExitCode: -1, RunnerError: fmt.Sprintf("ARCH-03 case %s missing phase pyproject.toml for %s", c.CaseID, pyRepr(phase))}
		}
		if err := copyFile(filepath.Join(c.CaseDir, name), pyprojectDest); err != nil {
			return CheckOutcome{ExitCode: -1, RunnerError: fmt.Sprintf("ARCH-03 exec error: %v", err)}
		}
		destPy := filepath.Join(tmp, target)
		if err := copyFile(targetSrc, destPy); err != nil {
			return CheckOutcome{ExitCode: -1, RunnerError: fmt.Sprintf("ARCH-03 exec error: %v", err)}
		}
		sources = append(sources, destPy)
	}
	for _, sf := range c.SupportFiles {
		if strings.HasSuffix(sf, ".toml") || strings.HasPrefix(sf, other+".") || strings.HasPrefix(sf, other+"_") || strings.HasPrefix(sf, other+"-") {
			continue
		}
		dest := filepath.Join(tmp, sf)
		if err := copyFile(filepath.Join(c.CaseDir, sf), dest); err != nil {
			return CheckOutcome{ExitCode: -1, RunnerError: fmt.Sprintf("ARCH-03 exec error: %v", err)}
		}
		if strings.HasSuffix(sf, ".py") {
			sources = append(sources, dest)
		}
	}
	args := []string{"--repo-root", tmp, "--pyproject", pyprojectDest}
	for _, src := range sources {
		args = append(args, "--source", src)
	}
	return runInProcess(ctx, timeout, "ARCH-03", func(cctx context.Context, stdout, stderr io.Writer) int {
		return pyver.Run(cctx, args, stdout, stderr)
	})
}

func runArch06(ctx context.Context, c Case, phase, repoRoot string, timeout time.Duration) CheckOutcome {
	rule := filepath.Join(repoRoot, "scripts", "lint", "ast-grep", "rules", "python-no-direct-sys-modules-assign.yml")
	return runAstGrep(ctx, c, phase, []string{rule}, timeout)
}

func runVerify04(ctx context.Context, c Case, phase, _ string, timeout time.Duration) CheckOutcome {
	targetPath := filepath.Join(c.CaseDir, phaseTarget(c, phase))
	ok, signal, detail := verifylive.EvaluateC07Subject(ctx, targetPath, timeout)
	if ok {
		return CheckOutcome{ExitCode: 0, Output: detail}
	}
	if signal == "" {
		return CheckOutcome{ExitCode: -1, RunnerError: fmt.Sprintf("VERIFY-04 exec error: %s", detail)}
	}
	return CheckOutcome{ExitCode: 1, Output: fmt.Sprintf("%s: %s", signal, detail)}
}
