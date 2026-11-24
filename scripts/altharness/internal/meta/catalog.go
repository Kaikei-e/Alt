package meta

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"alt/scripts/altharness/internal/canary"
	"alt/scripts/altharness/internal/codegen"
	"alt/scripts/altharness/internal/corpus"
	"alt/scripts/altharness/internal/generated"
	"alt/scripts/altharness/internal/hygiene"
	"alt/scripts/altharness/internal/ledgers"
	"alt/scripts/altharness/internal/packet"
	"alt/scripts/altharness/internal/pki"
	"alt/scripts/altharness/internal/pyver"
	"alt/scripts/altharness/internal/ratchet"
	"alt/scripts/altharness/internal/selfcheck"
	"alt/scripts/altharness/internal/suppress"
	"alt/scripts/altharness/internal/workflow"
)

// Catalog returns every historical step of the named gate. No check is dropped.
// Read-only steps run in parallel in-process on the worker pool with zero
// Python subprocesses. Base resolution happens before the pool starts.
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
		inProc("actionlint-canary", "canary:actionlint-canary", true, canary.RunAudit, "actionlint-canary"),
		{Name: "actionlint", Parallel: true, Cache: true, Args: []string{"go", "run", "github.com/rhysd/actionlint/cmd/actionlint@v1.7.12", "-shellcheck="}},
		inProc("canary-ledger", "canary:verify-ledger:"+repo, true, canary.RunAudit, "verify-ledger", "--repo-root", repo),
		{Name: "workflow-forbidden-triggers-grep", Key: "forbidden-triggers:" + repo, Parallel: true, Cache: true, Run: func(ctx context.Context) (string, int, error) {
			return ForbiddenTriggers(ctx, repo)
		}},
		inProc("workflow-safety-check", "workflow:"+repo, true, workflow.Run, "--repo-root", repo),
		{Name: "lint-run-tests", Parallel: true, Cache: true, Args: []string{"bash", "scripts/tests/test-lint-run.sh"}},
		inProc("suppression-governance", "suppressions:"+repo+":"+base, true, suppress.RunCheck, "--repo-root", repo, "--base", base),
		inProc("suppression-ratchet", "ratchet:"+repo, true, ratchet.Run, "--repo-root", repo),
	}
	if summary := os.Getenv("GITHUB_STEP_SUMMARY"); summary != "" {
		steps = append(steps, inProc("suppression-diff-summary", "suppression-diff:"+repo+":"+base+":"+summary, false, suppress.RunDiff, "--repo-root", repo, "--base", base, "--summary-file", summary))
	} else {
		steps = append(steps, Step{
			Name:     "suppression-diff-summary",
			Key:      "suppression-diff:unset",
			Parallel: true,
			Cache:    false,
			Run: func(ctx context.Context) (string, int, error) {
				return "OK: GITHUB_STEP_SUMMARY unset; skipping PR summary write.\n", 0, nil
			},
		})
	}
	steps = append(steps,
		astStep(repo, "ast-grep-rule-tests", "test", "-c", "scripts/lint/ast-grep/sgconfig.yml", "--skip-snapshot-tests"),
		astStep(repo, "ast-grep-scan", "scan", "-c", "scripts/lint/ast-grep/sgconfig.yml"),
		inProc("go-canonical-sync", "sync-go:--check:"+repo, true, codegen.Run, "--check", "--repo-root", repo),
		inProc("python-pki-sync", "pki:--check:"+repo, true, pki.Run, "--check", "--repo-root", repo),
		inProc("python-version-parity", "pyver:"+repo, true, pyver.Run, "--repo-root", repo),
		inProc("generated-code-governance", "generated:"+repo+":"+base, true, generated.Run, "--repo-root", repo, "--base", base),
		inProc("worktree-hygiene", "hygiene:"+repo, true, hygiene.Run, "--repo-root", repo),
	)
	return steps, nil
}

func harnessSteps(repo, base string) []Step {
	return []Step{
		inProc("harness_selfcheck", "selfcheck:"+repo, true, selfcheck.Run, "--root", repo),
		inProc("postmortem_ledger", "postmortem:--check:"+repo+":"+base, true, ledgers.RunPostmortem, "--check", "--root", repo, "--base", base),
		inProc("migration_destructive_gate", "migration-gate:"+repo+":"+base, true, ledgers.RunMigration, "--root", repo, "--base", base),
		inProc("adr_supersede_gate", "adr-gate:"+repo+":"+base, true, ledgers.RunAdr, "--dir", filepath.Join(repo, "docs", "ADR"), "--base", base),
		inProc("task_packet_templates", "packet:check-templates:"+repo, true, packet.Run, "check-templates", "--repo-root", repo),
		inProc("go_canonical_sync", "sync-go:--check:"+repo, true, codegen.Run, "--check", "--repo-root", repo),
		inProc("python_pki_sync", "pki:--check:"+repo, true, pki.Run, "--check", "--repo-root", repo),
		inProc("suppression_ratchet", "ratchet:"+repo, true, ratchet.Run, "--repo-root", repo),
		inProc("suppression_diff_report", "suppression-diff:"+repo+":"+base, true, suppress.RunDiff, "--repo-root", repo, "--base", base),
		inProc("generated_code", "generated:"+repo+":"+base, true, generated.Run, "--repo-root", repo, "--base", base),
		inProc("worktree_hygiene", "hygiene:"+repo, true, hygiene.Run, "--repo-root", repo),
		inProc("canary_ledger", "canary:verify-ledger:"+repo, true, canary.RunAudit, "verify-ledger", "--repo-root", repo),
		inProc("entrypoint_canary", "entrypoint-canary:proto:--skip-baseline:"+repo, true, canary.RunEntrypoint, "--repo-root", repo, "--languages", "proto", "--skip-baseline"),
		gofmtStep(repo, "altharness_gofmt", "scripts/altharness"),
		{Name: "altharness_govet", Parallel: true, Cache: true, Args: []string{"go", "-C", "scripts/altharness", "vet", "./..."}},
		{Name: "altharness_go_test_race", Parallel: true, Cache: true, Args: []string{"go", "-C", "scripts/altharness", "test", "-race", "-count=1", "./..."}},
		inProc("corpus", "corpus:"+repo, true, corpus.Run, "--repo-root", repo),
	}
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func inProc(name, key string, cache bool, fn func(ctx context.Context, args []string, stdout, stderr io.Writer) int, args ...string) Step {
	argsCopy := append([]string(nil), args...)
	return Step{
		Name:     name,
		Key:      key,
		Parallel: true,
		Cache:    cache,
		Run: func(ctx context.Context) (string, int, error) {
			var out syncBuffer
			code := fn(ctx, argsCopy, &out, &out)
			text := out.String()
			if ctx.Err() != nil {
				return text, 1, ctx.Err()
			}
			if code != 0 {
				return text + fmt.Sprintf("EXIT %d\n", code), code, nil
			}
			return text, 0, nil
		},
	}
}

func unionSteps(groups ...[]Step) []Step {
	seen := map[string]struct{}{}
	var out []Step
	for _, group := range groups {
		for _, step := range group {
			envPart := strings.Join(step.Env, "\x00")
			argPart := strings.Join(step.Args, "\x00")
			key := step.Name + "\x00" + step.Key + "\x00" + step.Dir + "\x00" + envPart + "\x00" + argPart
			if _, ok := seen[key]; ok {
				continue
			}
			// Same Key or same argv/dir/env under a different label is the repo/harness duplicate.
			if step.Key != "" {
				inProcKey := step.Dir + "\x00" + envPart + "\x00key:" + step.Key
				if _, ok := seen[inProcKey]; ok {
					continue
				}
				seen[inProcKey] = struct{}{}
			}
			if argPart != "" {
				argKey := step.Dir + "\x00" + envPart + "\x00arg:" + argPart
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
