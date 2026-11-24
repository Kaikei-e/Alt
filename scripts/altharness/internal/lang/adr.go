package lang

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"alt/scripts/altharness/internal/ledgers"
)

const docdagVersion = "v0.6.0"

// RunAdr is scripts/lint/adr.sh. docdag validate, the supersede gate, and
// docdag lint all run; a failure in one does not skip the others.
func RunAdr(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	ctx, stdout, stderr, code := prepare(ctx, stdout, stderr)
	if code != 0 {
		return code
	}
	if len(args) > 1 || (len(args) == 1 && args[0] != "adr") {
		fmt.Fprintln(stderr, "usage: adr.sh [adr]")
		return 2
	}
	root, rc := mustRepo(stderr)
	if rc != 0 {
		return rc
	}
	for _, name := range []string{"git", "docdag", "go"} {
		if _, err := exec.LookPath(name); err != nil {
			fmt.Fprintf(stderr, "error: required command not found in PATH: %s\n", name)
			return 1
		}
	}
	docdagBin, err := exec.LookPath("docdag")
	if err != nil {
		fmt.Fprintln(stderr, "error: required command not found in PATH: docdag")
		return 1
	}
	modOut, modErr, modCode, modRunErr := capture(ctx, root, nil, "go", "version", "-m", docdagBin)
	if modRunErr != nil || modCode != 0 {
		fmt.Fprintf(stderr, "error: failed to inspect docdag version via 'go version -m %s': %s\n", docdagBin, strings.TrimSpace(modOut+modErr))
		return 1
	}
	actual := "unknown"
	for _, line := range strings.Split(modOut, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[0] == "mod" && fields[1] == "github.com/Kaikei-e/DocDag" {
			actual = fields[2]
			break
		}
	}
	if actual != docdagVersion {
		fmt.Fprintf(stderr, "error: docdag version mismatch: expected %s, got '%s' (%s)\n", docdagVersion, actual, docdagBin)
		return 1
	}
	base, err := resolveAdrBase(ctx, root, stderr)
	if err != nil {
		return 1
	}
	raw := os.Getenv("LINT_BASE")
	shown := raw
	if shown == "" {
		shown = "<unset>"
	}
	fmt.Fprintf(stdout, "==> [adr] LINT_BASE=%s base=%s\n", shown, base)
	statusValidate, statusSupersede, statusLint := "pass", "pass", "pass"
	overall := 0
	fmt.Fprintf(stdout, "==> [adr] docdag validate --immutable-since %s\n", base)
	if code, err := run(ctx, stdout, stderr, root, nil, "docdag", "validate", "--immutable-since", base); err != nil || code != 0 {
		statusValidate = "fail"
		overall = 1
	}
	fmt.Fprintf(stdout, "==> [adr] adr-gate --base %s\n", base)
	if ledgers.RunAdr(ctx, []string{"--base", base, "--dir", filepath.Join(root, "docs", "ADR")}, stdout, stderr) != 0 {
		statusSupersede = "fail"
		overall = 1
	}
	fmt.Fprintln(stdout, "==> [adr] docdag lint --strict")
	if code, err := run(ctx, stdout, stderr, root, nil, "docdag", "lint", "--strict"); err != nil || code != 0 {
		statusLint = "fail"
		overall = 1
	}
	fmt.Fprintf(stdout, "==> [adr] summary (base=%s)\n", base)
	fmt.Fprintf(stdout, "  %-24s %s\n", "docdag validate", statusValidate)
	fmt.Fprintf(stdout, "  %-24s %s\n", "adr_supersede_gate", statusSupersede)
	fmt.Fprintf(stdout, "  %-24s %s\n", "docdag lint --strict", statusLint)
	return overall
}

func resolveAdrBase(ctx context.Context, repo string, stderr io.Writer) (string, error) {
	head, err := gitQuiet(ctx, repo, "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
	if err != nil || head == "" {
		fmt.Fprintln(stderr, "error: cannot resolve HEAD commit")
		return "", fmt.Errorf("cannot resolve HEAD commit")
	}
	raw := os.Getenv("LINT_BASE")
	if raw != "" && !allZero(raw) {
		if target, err := gitQuiet(ctx, repo, "rev-parse", "--verify", "--quiet", raw+"^{commit}"); err == nil && target != "" {
			base, err := mergeLintBase(ctx, repo, target, head, raw)
			if err != nil {
				fmt.Fprintf(stderr, "error: %s\n", err.Error())
				return "", err
			}
			return base, nil
		}
		if !fullSHA(raw) {
			msg := fmt.Sprintf("LINT_BASE '%s' cannot be resolved to a commit", raw)
			fmt.Fprintf(stderr, "error: %s\n", msg)
			return "", fmt.Errorf("%s", msg)
		}
		fmt.Fprintf(stderr, "warn: LINT_BASE '%s' not found (force-push?); falling back to origin/main / HEAD~1\n", raw)
	}
	target, err := gitQuiet(ctx, repo, "rev-parse", "--verify", "--quiet", "origin/main^{commit}")
	if err != nil || target == "" {
		fmt.Fprintln(stderr, "error: cannot resolve origin/main (fetch full history from origin)")
		return "", fmt.Errorf("origin/main")
	}
	mergeBase, err := gitQuiet(ctx, repo, "merge-base", head, target)
	if err != nil || mergeBase == "" {
		fmt.Fprintln(stderr, "error: cannot compute merge-base between HEAD and origin/main (shallow clone or unrelated history)")
		return "", fmt.Errorf("merge-base")
	}
	if mergeBase == head {
		parent, err := gitQuiet(ctx, repo, "rev-parse", "--verify", "--quiet", "HEAD~1^{commit}")
		if err != nil || parent == "" {
			fmt.Fprintln(stderr, "error: HEAD matches merge-base with origin/main and HEAD~1 cannot be resolved (shallow clone?)")
			return "", fmt.Errorf("HEAD~1")
		}
		return parent, nil
	}
	return mergeBase, nil
}

func mergeLintBase(ctx context.Context, repo, target, head, raw string) (string, error) {
	mergeBase, err := gitQuiet(ctx, repo, "merge-base", target, head)
	if err != nil || mergeBase == "" {
		return "", fmt.Errorf("cannot compute merge-base between LINT_BASE '%s' (%s) and HEAD (%s) (shallow clone or unrelated history)", raw, target, head)
	}
	if mergeBase == head {
		parent, err := gitQuiet(ctx, repo, "rev-parse", "--verify", "--quiet", "HEAD~1^{commit}")
		if err != nil || parent == "" {
			return "", fmt.Errorf("LINT_BASE '%s' matches HEAD and HEAD~1 cannot be resolved (shallow clone?)", raw)
		}
		return parent, nil
	}
	return mergeBase, nil
}

func gitQuiet(ctx context.Context, repo string, args ...string) (string, error) {
	out, _, code, err := capture(ctx, repo, nil, append([]string{"git", "-C", repo}, args...)...)
	if err != nil || code != 0 {
		return "", fmt.Errorf("git failed")
	}
	return strings.TrimSpace(out), nil
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
