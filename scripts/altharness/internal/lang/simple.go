package lang

import (
	"context"
	"fmt"
	"io"
)

// RunApparmor is scripts/lint/apparmor.sh.
func RunApparmor(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return runNamed(ctx, args, stdout, stderr, []namedStep{
		{"bash-n:probe-apparmor-postgres.sh", []string{"bash", "-n", "scripts/probe-apparmor-postgres.sh"}},
		{"bash-n:check-apparmor-profile.sh", []string{"bash", "-n", "scripts/check-apparmor-profile.sh"}},
		{"bash-n:install-apparmor-profile.sh", []string{"bash", "-n", "scripts/install-apparmor-profile.sh"}},
	})
}

// RunE2E is scripts/lint/e2e.sh.
func RunE2E(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return runNamed(ctx, args, stdout, stderr, []namedStep{
		{"check-suites", []string{"bash", "e2e/playwright/_lib/check-suites.sh"}},
		{"e2e-clean-worktree", []string{"python3", "scripts/tests/test-e2e-clean-worktree.py"}},
	})
}

// RunRegistry is scripts/lint/registry.sh.
func RunRegistry(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return runNamed(ctx, args, stdout, stderr, []namedStep{
		{"regen-service-registry", []string{"python3", "scripts/regen-service-registry.py", "--check"}},
		{"compose-image-audit", []string{"python3", "scripts/compose-image-audit.py"}},
	})
}

type namedStep struct {
	label string
	args  []string
}

func runNamed(ctx context.Context, args []string, stdout, stderr io.Writer, steps []namedStep) int {
	ctx, stdout, stderr, code := prepare(ctx, stdout, stderr)
	if code != 0 {
		return code
	}
	root, rc := mustRepo(stderr)
	if rc != 0 {
		return rc
	}
	fail := 0
	for _, step := range steps {
		if ctx.Err() != nil {
			fmt.Fprintf(stderr, "error: %s\n", ctx.Err().Error())
			return 1
		}
		code, err := run(ctx, stdout, stderr, root, nil, step.args...)
		if err != nil {
			fmt.Fprintf(stderr, "ERROR: Required command not found in PATH: %s\n", step.args[0])
			fmt.Fprintf(stdout, "FAIL: %s\n", step.label)
			fail = 1
			continue
		}
		if code == 0 {
			fmt.Fprintf(stdout, "PASS: %s\n", step.label)
			continue
		}
		fmt.Fprintf(stdout, "FAIL: %s\n", step.label)
		fail = 1
	}
	return fail
}
