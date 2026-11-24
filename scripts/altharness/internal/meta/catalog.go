package meta

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Catalog returns every historical step of the named gate. No check is dropped.
// Read-only steps run in parallel. Steps that need a resolved base are still
// parallel with each other; base resolution happens before the pool starts.
func Catalog(ctx context.Context, repo, name string) ([]Step, error) {
	base := ""
	needsBase := name == "repo" || name == "harness" || name == "both"
	if needsBase {
		var err error
		base, err = ResolveBase(ctx, repo)
		if err != nil {
			return nil, err
		}
	}
	switch name {
	case "repo":
		return repoSteps(repo, base)
	case "harness":
		return harnessSteps(repo, base), nil
	case "both":
		rSteps, err := repoSteps(repo, base)
		if err != nil {
			return nil, err
		}
		return unionSteps(rSteps, harnessSteps(repo, base)), nil
	default:
		return nil, fmt.Errorf("unknown gate %q", name)
	}
}

func repoSteps(repo, base string) ([]Step, error) {
	steps := []Step{
		bin("actionlint-canary", "canary", "actionlint-canary"),
		{Name: "actionlint", Parallel: true, Cache: true, Args: []string{"go", "run", "github.com/rhysd/actionlint/cmd/actionlint@v1.7.12", "-shellcheck="}},
		bin("canary-ledger", "canary", "verify-ledger", "--repo-root", "."),
		{Name: "workflow-safety-tests", Parallel: true, Cache: true, Args: []string{"python3", "-m", "unittest", "discover", "-s", ".github/workflows/scripts", "-p", "test_*.py"}},
		{Name: "workflow-forbidden-triggers-grep", Parallel: true, Cache: true, Run: func(ctx context.Context) (string, int, error) {
			return ForbiddenTriggers(ctx, repo)
		}},
		bin("workflow-safety-check", "workflow"),
		{Name: "lint-run-tests", Parallel: true, Cache: true, Args: []string{"bash", "scripts/tests/test-lint-run.sh"}},
		py("lint-canary-tests", "scripts/tests/test-lint-canary.py"),
		py("suppression-check-tests", "scripts/tests/test-check-suppressions.py"),
		bin("suppression-governance", "suppressions", "--base", base),
		py("suppression-ratchet-tests", "scripts/tests/test-check-ratchet.py"),
		bin("suppression-ratchet", "ratchet"),
		py("suppression-diff-tests", "scripts/tests/test-suppression-diff-report.py"),
	}
	if summary := os.Getenv("GITHUB_STEP_SUMMARY"); summary != "" {
		steps = append(steps, Step{
			Name:     "suppression-diff-summary",
			Parallel: true,
			Cache:    false,
			Args:     []string{"scripts/altharness/alt-harness", "suppression-diff", "--base", base, "--summary-file", summary},
		})
	} else {
		steps = append(steps, Step{Name: "suppression-diff-summary", Parallel: true, Cache: false, Args: []string{"python3", "-c", "print('OK: GITHUB_STEP_SUMMARY unset; skipping PR summary write.')"}})
	}
	steps = append(steps,
		astStep(repo, "ast-grep-rule-tests", "test", "-c", "scripts/lint/ast-grep/sgconfig.yml", "--skip-snapshot-tests"),
		astStep(repo, "ast-grep-scan", "scan", "-c", "scripts/lint/ast-grep/sgconfig.yml"),
		bin("go-canonical-sync", "sync-go", "--check"),
		bin("python-pki-sync", "pki", "--check"),
		bin("python-version-parity", "pyver"),
		py("generated-code-tests", "scripts/tests/test-check-generated-code.py"),
		bin("generated-code-governance", "generated", "--base", base),
		py("worktree-hygiene-tests", "scripts/tests/test-worktree-hygiene.py"),
		bin("worktree-hygiene", "hygiene"),
	)
	return steps, nil
}

func harnessSteps(repo, base string) []Step {
	return []Step{
		bin("harness_selfcheck", "selfcheck"),
		bin("postmortem_ledger", "postmortem", "--check", "--base", base),
		bin("migration_destructive_gate", "migration-gate", "--base", base),
		bin("adr_supersede_gate", "adr-gate", "--base", base),
		py("guard_ops_commands_branches", "scripts/tests/test-guard-ops-commands-branches.py"),
		bin("task_packet_templates", "packet", "check-templates"),
		py("task_packet_tests", "scripts/tests/test-task-packet.py"),
		bin("go_canonical_sync", "sync-go", "--check"),
		py("go_canonical_tests", "scripts/tests/test-sync-go-canonical.py"),
		bin("python_pki_sync", "pki", "--check"),
		py("python_pki_tests", "scripts/tests/test-sync-python-pki.py"),
		py("python_version_tests", "scripts/tests/test-check-python-versions.py"),
		py("pytest_randomized_tests", "scripts/tests/test-pytest-randomized.py"),
		py("behavior_screener_tests", "scripts/tests/test-behavior-screener.py"),
		bin("suppression_ratchet", "ratchet"),
		py("suppression_ratchet_tests", "scripts/tests/test-check-ratchet.py"),
		bin("suppression_diff_report", "suppression-diff", "--base", base),
		py("suppression_diff_tests", "scripts/tests/test-suppression-diff-report.py"),
		bin("generated_code", "generated", "--base", base),
		py("generated_code_tests", "scripts/tests/test-check-generated-code.py"),
		bin("worktree_hygiene", "hygiene"),
		py("worktree_hygiene_tests", "scripts/tests/test-worktree-hygiene.py"),
		bin("canary_ledger", "canary", "verify-ledger", "--repo-root", "."),
		bin("entrypoint_canary", "entrypoint-canary", "--languages", "proto", "--skip-baseline"),
		py("exec_guards_and_runs_tests", "scripts/tests/test-exec-guards-and-runs.py"),
		gofmtStep(repo, "altharness_gofmt", "scripts/altharness"),
		{Name: "altharness_govet", Parallel: true, Cache: true, Args: []string{"go", "-C", "scripts/altharness", "vet", "./..."}},
		{Name: "altharness_go_test_race", Parallel: true, Cache: true, Args: []string{"go", "-C", "scripts/altharness", "test", "-race", "-count=1", "./..."}},
		py("corpus", "scripts/tests/test-corpus.py"),
	}
}

func py(name string, args ...string) Step {
	return Step{Name: name, Parallel: true, Cache: true, Args: append([]string{"python3"}, args...)}
}

func bin(name string, args ...string) Step {
	return Step{Name: name, Parallel: true, Cache: true, Args: append([]string{"scripts/altharness/alt-harness"}, args...)}
}

func unionSteps(groups ...[]Step) []Step {
	seen := map[string]struct{}{}
	var out []Step
	for _, group := range groups {
		for _, step := range group {
			envPart := strings.Join(step.Env, "\x00")
			argPart := strings.Join(step.Args, "\x00")
			key := step.Name + "\x00" + step.Dir + "\x00" + envPart + "\x00" + argPart
			if _, ok := seen[key]; ok {
				continue
			}
			// Same argv/dir/env under a different label is the repo/harness duplicate.
			if argPart != "" {
				argKey := step.Dir + "\x00" + envPart + "\x00" + argPart
				if _, ok := seen[argKey]; ok {
					continue
				}
				seen[argKey] = struct{}{}
			}
			seen[key] = struct{}{}
			out = append(out, step)
		}
	}
	return out
}

func lookAstGrep() (string, error) {
	if p, err := exec.LookPath("ast-grep"); err == nil {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err == nil {
		p := filepath.Join(home, ".local", "bin", "ast-grep")
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("ast-grep not found in PATH or ~/.local/bin/ast-grep")
}
