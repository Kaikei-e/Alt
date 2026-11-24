package meta

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ResolveBase matches scripts/lint/repo.sh resolve_suppression_diff_base.
// An empty or all-zero LINT_BASE is treated as unset. Failure is an error,
// never a silent fallback to HEAD.
func ResolveBase(ctx context.Context, repo string) (string, error) {
	head, err := gitOut(ctx, repo, "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
	if err != nil || head == "" {
		return "", fmt.Errorf("cannot resolve HEAD commit")
	}
	raw := os.Getenv("LINT_BASE")
	if raw != "" && !allZero(raw) {
		if target, err := gitOut(ctx, repo, "rev-parse", "--verify", "--quiet", raw+"^{commit}"); err == nil && target != "" {
			return mergeOrParent(ctx, repo, target, head, raw)
		}
		if !fullSHA(raw) {
			return "", fmt.Errorf("LINT_BASE %q cannot be resolved to a commit", raw)
		}
		fmt.Fprintf(os.Stderr, "warn: LINT_BASE '%s' not found (force-push?); falling back to origin/main / HEAD~1\n", raw)
	}
	target, err := gitOut(ctx, repo, "rev-parse", "--verify", "--quiet", "origin/main^{commit}")
	if err != nil || target == "" {
		return "", fmt.Errorf("cannot resolve origin/main (fetch full history from origin)")
	}
	base, err := mergeOrParent(ctx, repo, target, head, "origin/main")
	if err != nil {
		return "", err
	}
	return base, nil
}

func mergeOrParent(ctx context.Context, repo, target, head, label string) (string, error) {
	mb, err := gitOut(ctx, repo, "merge-base", target, head)
	if err != nil || mb == "" {
		return "", fmt.Errorf("cannot compute merge-base between %s (%s) and HEAD (%s) (shallow clone or unrelated history)", label, target, head)
	}
	if mb == head {
		parent, err := gitOut(ctx, repo, "rev-parse", "--verify", "--quiet", "HEAD~1^{commit}")
		if err != nil || parent == "" {
			return "", fmt.Errorf("%s matches HEAD and HEAD~1 cannot be resolved (shallow clone?)", label)
		}
		return parent, nil
	}
	return mb, nil
}

func gitOut(ctx context.Context, repo string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repo}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func allZero(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c != '0' {
			return false
		}
	}
	return true
}

func fullSHA(s string) bool {
	if len(s) != 40 {
		return false
	}
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}
