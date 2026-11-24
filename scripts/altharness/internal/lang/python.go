package lang

import (
	"alt/scripts/altharness/internal/canary"
	"alt/scripts/altharness/internal/fsperm"
	"alt/scripts/altharness/internal/pyver"
	"alt/scripts/altharness/internal/ratchet"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// RunPython is scripts/lint/python.sh. Dependency probes and the canary audit
// run before ruff, bandit, and pyrefly.
func RunPython(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	ctx, stdout, stderr, code := prepare(ctx, stdout, stderr)
	if code != 0 {
		return code
	}
	if len(args) != 1 {
		fmt.Fprintln(stderr, "Usage: python.sh <project-dir>")
		return 2
	}
	root, rc := mustRepo(stderr)
	if rc != 0 {
		return rc
	}
	projectRel := strings.TrimSuffix(args[0], "/")
	target := filepath.Join(root, projectRel)
	if _, err := os.Stat(filepath.Join(target, "pyproject.toml")); err != nil {
		fmt.Fprintf(stderr, "Error: %s/pyproject.toml not found\n", target)
		return 2
	}
	work, err := os.MkdirTemp("", "alt-py-canary-")
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err.Error())
		return 1
	}
	defer os.RemoveAll(work)

	var steps []stepResult
	failed := 0
	record := func(name, status string, bad bool) {
		steps = append(steps, stepResult{name, status})
		if bad {
			failed = 1
		}
	}
	runStep := func(name string, argv ...string) {
		fmt.Fprintf(stdout, "==> %s\n", name)
		code, err := run(ctx, stdout, stderr, target, nil, argv...)
		if err != nil {
			record(name, fmt.Sprintf("fail (exit %d)", 127), true)
			fmt.Fprintf(stderr, "ERROR: Required command not found in PATH: %s\n", argv[0])
			return
		}
		if code == 0 {
			record(name, "pass", false)
			return
		}
		record(name, fmt.Sprintf("fail (exit %d)", code), true)
	}

	banditState, banditRC := queryDevDep(ctx, target, "bandit", stderr)
	if banditRC != 0 {
		fmt.Fprintf(stderr, "error: failed to inspect pyproject.toml for bandit (exit %d)\n", banditRC)
		return 1
	}
	checkBandit, banditSuffix, rc := depChoice(stderr, "bandit", banditState)
	if rc != 0 {
		return rc
	}
	pyreflyState, pyreflyRC := queryDevDep(ctx, target, "pyrefly", stderr)
	if pyreflyRC != 0 {
		fmt.Fprintf(stderr, "error: failed to inspect pyproject.toml for pyrefly (exit %d)\n", pyreflyRC)
		return 1
	}
	checkPyrefly, pyreflySuffix, rc := depChoice(stderr, "pyrefly", pyreflyState)
	if rc != 0 {
		return rc
	}

	fmt.Fprintf(stdout, "==> canary audit (ruff%s%s)\n", banditSuffix, pyreflySuffix)
	if canary.RunAudit(ctx, []string{"python-canary",
		"--project-dir", target, "--work-dir", filepath.Join(work, "canary"), "--check-bandit", checkBandit, "--check-pyrefly", checkPyrefly}, stdout, stderr) != 0 {
		return 1
	}
	_ = os.RemoveAll(filepath.Join(work, "canary"))
	record("canary", "pass", false)

	fmt.Fprintln(stdout, "==> source files check")
	if canary.RunAudit(ctx, []string{"python-verify-files", "--project-dir", target}, stdout, stderr) != 0 {
		return 1
	}
	record("source files", "pass", false)

	fmt.Fprintln(stdout, "==> python versions")
	if pvCode := pyver.Run(ctx, []string{"--repo-root", root, "--project-dir", projectRel}, stdout, stderr); pvCode == 0 {
		record("python versions", "pass", false)
	} else {
		record("python versions", fmt.Sprintf("fail (exit %d)", pvCode), true)
	}

	policy := effectivePolicy(ctx)
	if policy == "block-new" {
		ruffJSON := filepath.Join(work, "ruff.json")
		fmt.Fprintln(stdout, "==> ruff check (policy=block-new)")
		ruffOut, err := os.Create(ruffJSON)
		if err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err.Error())
			return 1
		}
		ruffCode, ruffErr := run(ctx, ruffOut, stderr, target, nil, "uv", "run", "--frozen", "--only-group", "dev", "ruff", "check", "--output-format", "json", ".")
		ruffOut.Close()
		switch {
		case ruffErr != nil:
			fmt.Fprintln(stderr, "ERROR: Required command not found in PATH: uv")
			record("ruff check", "fail (ruff exit 127)", true)
		case ruffCode == 0:
			record("ruff check", "pass", false)
		case ruffCode == 1:
			fp := ratchet.Run(ctx, []string{"check-fingerprints",
				"--repo-root", root, "--project-dir", target, "--ruff-json", ruffJSON, "--require-findings"}, stdout, stderr)
			if fp != 0 {
				record("ruff check", fmt.Sprintf("fail (block-new exit %d)", fp), true)
			} else {
				record("ruff check", "pass (block-new)", false)
			}
		default:
			if data, err := os.ReadFile(ruffJSON); err == nil {
				_, _ = stderr.Write(data)
			}
			record("ruff check", fmt.Sprintf("fail (ruff exit %d)", ruffCode), true)
		}
	} else {
		runStep("ruff check", "uv", "run", "--frozen", "--only-group", "dev", "ruff", "check", ".")
	}
	runStep("ruff format", "uv", "run", "--frozen", "--only-group", "dev", "ruff", "format", "--check", ".")

	if banditState == "declared" {
		fmt.Fprintln(stdout, "==> bandit")
		banditPath := filepath.Join(work, "bandit.out")
		var combined strings.Builder
		captureBoth := &stringWriter{&combined}
		banditCode, banditErr := run(ctx, ioMulti(stdout, captureBoth), ioMulti(stderr, captureBoth), target, nil,
			"uv", "run", "--frozen", "--only-group", "dev", "bandit", "-r", ".", "-ll", "-ii",
			"-x", "./.venv,./node_modules,./.mypy_cache,./.pytest_cache,./.ruff_cache,./build,./dist")
		_ = os.WriteFile(banditPath, []byte(combined.String()), fsperm.File)
		if banditErr != nil {
			fmt.Fprintln(stderr, "ERROR: Required command not found in PATH: uv")
			return 1
		}
		if canary.RunAudit(ctx, []string{"python-verify-bandit-scan",
			"--project-dir", target, "--bandit-out", banditPath}, stdout, stderr) != 0 {
			return 1
		}
		if banditCode == 0 {
			record("bandit", "pass", false)
		} else {
			record("bandit", fmt.Sprintf("fail (exit %d)", banditCode), true)
		}
	} else {
		record("bandit", "not declared", false)
	}

	if pyreflyState == "declared" {
		runStep("pyrefly", "uv", "run", "--frozen", "--all-extras", "--group", "dev", "pyrefly", "check")
	} else if pyreflyState == "not-declared" {
		record("pyrefly", "not declared", false)
	}

	if projectRel == "scripts" {
		runStep("python3 compileall", "python3", "-m", "compileall", "-q", "-x", `(\.venv|__pycache__|harness/corpus)`, ".")
		matches := helperTests(target)
		if len(matches) == 0 {
			fmt.Fprintln(stderr, "error: no helper tests matched under scripts/tests/")
			record("scripts helper tests", "fail (none found)", true)
		} else {
			for _, testFile := range matches {
				runStep(testFile, "uv", "run", "--frozen", "--only-group", "dev", "python", testFile)
			}
		}
	}

	fmt.Fprintln(stdout)
	fmt.Fprintf(stdout, "Summary (%s):\n", projectRel)
	for _, step := range steps {
		fmt.Fprintf(stdout, "  %s: %s\n", step.name, step.status)
	}
	return failed
}

type stepResult struct {
	name   string
	status string
}

func depChoice(stderr io.Writer, pkg, state string) (string, string, int) {
	switch state {
	case "declared":
		return "true", ", " + pkg, 0
	case "not-declared":
		return "false", "", 0
	default:
		fmt.Fprintf(stderr, "error: unexpected dev-dependency check output for %s: %s\n", pkg, state)
		return "", "", 1
	}
}

func queryDevDep(ctx context.Context, dir, pkg string, stderr io.Writer) (string, int) {
	out, errText, code, err := capture(ctx, dir, nil, "uv", "run", "--frozen", "--only-group", "dev", "python", "-c", devDepScript, pkg)
	if err != nil {
		fmt.Fprintln(stderr, "ERROR: Required command not found in PATH: uv")
		return "", 127
	}
	if code != 0 {
		if errText != "" {
			fmt.Fprint(stderr, errText)
		}
		return "", code
	}
	return strings.TrimSpace(out), 0
}

const devDepScript = `
import re, sys, tomllib

with open("pyproject.toml", "rb") as f:
    data = tomllib.load(f)

dev = data.get("dependency-groups", {}).get("dev", [])
target = re.sub(r"[-_.]+", "-", sys.argv[1]).lower()
for entry in dev:
    if isinstance(entry, str):
        name = re.split(r"[\s\[<>=!~;@]", entry.strip(), maxsplit=1)[0]
        if re.sub(r"[-_.]+", "-", name).lower() == target:
            print("declared")
            sys.exit(0)
print("not-declared")
`

func helperTests(target string) []string {
	patterns := []string{
		"tests/test-*-helpers.py",
		"tests/test-genre-report-on-error-stop.py",
	}
	var matches []string
	seen := map[string]struct{}{}
	for _, pattern := range patterns {
		found, _ := filepath.Glob(filepath.Join(target, pattern))
		for _, path := range found {
			rel, err := filepath.Rel(target, path)
			if err != nil {
				continue
			}
			rel = filepath.ToSlash(rel)
			if _, ok := seen[rel]; ok {
				continue
			}
			seen[rel] = struct{}{}
			matches = append(matches, rel)
		}
	}
	return matches
}

type stringWriter struct{ b *strings.Builder }

func (w stringWriter) Write(p []byte) (int, error) { return w.b.Write(p) }

type multiWriter struct{ ws []io.Writer }

func ioMulti(ws ...io.Writer) io.Writer { return multiWriter{ws} }

func (m multiWriter) Write(p []byte) (int, error) {
	for _, w := range m.ws {
		if _, err := w.Write(p); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}
