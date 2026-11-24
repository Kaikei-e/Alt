package lang

import (
	"alt/scripts/altharness/internal/fsperm"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// RunCompose is scripts/lint/compose.sh. It stages a checkout, then runs the
// compose audits in the shell's order. Disk cleanup is left to run.sh.
func RunCompose(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	ctx, stdout, stderr, code := prepare(ctx, stdout, stderr)
	if code != 0 {
		return code
	}
	root, rc := mustRepo(stderr)
	if rc != 0 {
		return rc
	}
	for _, item := range []struct{ name, hint string }{
		{"docker", "install Docker CLI with Compose plugin"},
		{"lkal", "install via 'make install-lkal'"},
		{"kisage", "install via 'make install-kisage'"},
	} {
		if _, err := exec.LookPath(item.name); err != nil {
			commandMissingHint(stderr, item.name, item.hint)
			return 1
		}
	}
	installer := filepath.Join(root, "scripts", "ci-install-compose.sh")
	data, err := os.ReadFile(installer)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: Missing compose installer: %s\n", installer)
		return 1
	}
	pin := regexp.MustCompile(`(?m)^COMPOSE_VERSION="\$\{COMPOSE_VERSION:-([^}]*)\}"`).FindSubmatch(data)
	if pin == nil {
		fmt.Fprintf(stderr, "ERROR: Could not parse COMPOSE_VERSION pin from %s\n", installer)
		return 1
	}
	expected := string(pin[1])
	verOut, verErr, verCode, verRunErr := capture(ctx, root, nil, "docker", "compose", "version")
	if verRunErr != nil || verCode != 0 {
		fmt.Fprintf(stderr, "ERROR: failed to query docker compose version: %s\n", strings.TrimSpace(verOut+verErr))
		return 1
	}
	fields := strings.Fields(strings.TrimSpace(verOut))
	actual := ""
	if len(fields) > 0 {
		actual = fields[len(fields)-1]
	}
	if actual != expected {
		fmt.Fprintf(stderr, "ERROR: docker compose version mismatch: expected %s, got '%s'\n", expected, strings.TrimSpace(verOut))
		return 1
	}
	work, err := os.MkdirTemp("", "alt-compose-lint-")
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %s\n", err.Error())
		return 1
	}
	defer os.RemoveAll(work)
	env, rc := composePython(ctx, stderr, work)
	if rc != 0 {
		return rc
	}
	if _, err := os.Stat(filepath.Join(root, ".env.template")); err != nil {
		fmt.Fprintf(stderr, "ERROR: Missing .env.template at %s\n", filepath.Join(root, ".env.template"))
		return 1
	}
	stage := filepath.Join(work, "stage")
	if err := stageCompose(ctx, root, stage); err != nil {
		fmt.Fprintf(stderr, "ERROR: %s\n", err.Error())
		return 1
	}
	template, err := os.ReadFile(filepath.Join(root, ".env.template"))
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %s\n", err.Error())
		return 1
	}
	dummy := filepath.Join(stage, ".env")
	body := string(template) + "\nDOCKER_GROUP_ID=0\nKRATOS_COOKIE_DOMAIN=\nTRUSTED_PROXY_CIDRS=none\n"
	if err := os.WriteFile(dummy, []byte(body), fsperm.File); err != nil {
		fmt.Fprintf(stderr, "ERROR: %s\n", err.Error())
		return 1
	}
	if err := copyFile(dummy, filepath.Join(stage, "compose", ".env")); err != nil {
		fmt.Fprintf(stderr, "ERROR: %s\n", err.Error())
		return 1
	}
	steps := composeSteps(root, stage, dummy)
	names := make([]string, 0, len(steps))
	results := make([]string, 0, len(steps))
	overall := 0
	for _, step := range steps {
		if ctx.Err() != nil {
			fmt.Fprintf(stderr, "error: %s\n", ctx.Err().Error())
			return 1
		}
		fmt.Fprintf(stdout, "==> %s\n", step.name)
		stepEnv := append(append([]string{}, env...), step.env...)
		code, err := run(ctx, stdout, stderr, root, stepEnv, step.args...)
		names = append(names, step.name)
		if err != nil || code != 0 {
			exit := code
			if err != nil {
				exit = 127
				fmt.Fprintf(stderr, "ERROR: Required command not found in PATH: %s\n", step.args[0])
			}
			results = append(results, fmt.Sprintf("FAIL (exit %d)", exit))
			overall = 1
			continue
		}
		results = append(results, "PASS")
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "==> Summary")
	for i, name := range names {
		fmt.Fprintf(stdout, "  %-42s %s\n", name, results[i])
	}
	return overall
}

type composeStep struct {
	name string
	env  []string
	args []string
}

func composeSteps(root, stage, dummy string) []composeStep {
	py := func(name string, args ...string) composeStep {
		return composeStep{name: name, args: append([]string{"python3"}, args...)}
	}
	sh := func(name string, env []string, args ...string) composeStep {
		return composeStep{name: name, env: env, args: append([]string{"bash"}, args...)}
	}
	return []composeStep{
		py("test_ci_install_compose", "-m", "unittest", "discover", "-s", "tests/scripts", "-p", "test_ci_install_compose.py"),
		py("compose-healthcheck-audit", "scripts/compose-healthcheck-audit.py", "--repo-root", stage, "--env-file", dummy),
		py("test-compose-port-audit", "scripts/tests/test-compose-port-audit.py"),
		py("compose-port-audit", "scripts/compose-port-audit.py", "--repo-root", stage, "--overlays"),
		sh("check-compose-variables", []string{"COMPOSE_FILE=" + filepath.Join(stage, "compose", "compose.yaml")}, "scripts/check-compose-variables.sh"),
		py("test-compose-netns-cascade-audit", "scripts/tests/test-compose-netns-cascade-audit.py"),
		py("compose-netns-cascade-audit", "scripts/compose-netns-cascade-audit.py"),
		py("test-pki-wave4-acceptance", "scripts/tests/test-pki-wave4-acceptance.py"),
		py("test-pki-subject-scoped-provisioners", "scripts/tests/test-pki-subject-scoped-provisioners.py"),
		py("test-pki-agent-recovery-runbook", "scripts/tests/test-pki-agent-recovery-runbook.py"),
		py("test-retire-pki-writers", "scripts/tests/test-retire-pki-writers.py"),
		py("test-health-deep-contract", "scripts/tests/test-health-deep-contract.py"),
		py("test-p2-doc-contracts", "scripts/tests/test-p2-doc-contracts.py"),
		sh("test_deploy", nil, "tests/scripts/test_deploy.sh"),
		sh("test_cascade_pki_sidecars", nil, "tests/scripts/test_cascade_pki_sidecars.sh"),
		py("test-compose-file-bind-audit", "scripts/tests/test-compose-file-bind-audit.py"),
		py("compose-file-bind-audit", "scripts/compose-file-bind-audit.py"),
		py("test-compose-init-edge-audit", "scripts/tests/test-compose-init-edge-audit.py"),
		py("test-compose-pre-start-audit", "scripts/tests/test-compose-pre-start-audit.py"),
		py("compose-init-edge-audit", "scripts/compose-init-edge-audit.py"),
		py("compose-pre-start-audit", "scripts/compose-pre-start-audit.py"),
		py("test-compose-ops-surface-audit", "scripts/tests/test-compose-ops-surface-audit.py"),
		py("compose-ops-surface-audit", "scripts/compose-ops-surface-audit.py"),
		py("test-compose-restart-policy-audit", "scripts/tests/test-compose-restart-policy-audit.py"),
		py("compose-restart-policy-audit", "scripts/compose-restart-policy-audit.py"),
		py("test-compose-feature-gate", "scripts/tests/test-compose-feature-gate.py"),
		py("compose-feature-gate", "scripts/compose-feature-gate.py", "--gate-report"),
		py("test-compose-prod-env-wiring", "scripts/tests/test-compose-prod-env-wiring.py"),
		sh("test_compose_wiring", []string{"COMPOSE_REPO_ROOT=" + stage, "COMPOSE_ENV_FILE=" + dummy}, "scripts/tests/test_compose_wiring.sh"),
		py("test-compose-sovereign-knobs", "scripts/tests/test-compose-sovereign-knobs.py"),
		py("test-recap-evaluator-auth-wiring", "scripts/tests/test-recap-evaluator-auth-wiring.py"),
		py("test-check-env-overrides", "scripts/tests/test-check-env-overrides.py"),
		py("test-group1-security", "scripts/tests/test-group1-security.py"),
		py("test-compose-reachability-audit", "scripts/tests/test-compose-reachability-audit.py"),
		py("compose-reachability-audit", "scripts/compose-reachability-audit.py", "--repo-root", stage, "-f", filepath.Join(stage, "compose", "compose.yaml"), "--env-file", dummy),
		{name: "lkal-check", args: []string{"lkal", "check", "--json", "deploy/desired-env.lkal"}},
		py("kisage-env-verify", "scripts/kisage-env.py", "verify"),
		py("test-kisage-env", "scripts/tests/test-kisage-env.py"),
		py("test-rag-orchestrator-callers", "scripts/tests/test-rag-orchestrator-callers.py"),
		py("test_kisage_clean_checkout", "-m", "unittest", "discover", "-s", "tests/scripts", "-p", "test_kisage_clean_checkout.py"),
	}
}

func composePython(ctx context.Context, stderr io.Writer, work string) ([]string, int) {
	const probe = `import sys, yaml
ver = getattr(yaml, "__version__", "")
sys.exit(0 if sys.version_info >= (3, 12) and ver.split(".", 1)[0] == "6" else 1)`
	if _, err := exec.LookPath("python3"); err == nil {
		code, runErr := run(ctx, io.Discard, io.Discard, work, nil, "python3", "-c", probe)
		if runErr == nil && code == 0 {
			return nil, 0
		}
	}
	if _, err := exec.LookPath("uv"); err != nil {
		fmt.Fprintln(stderr, "ERROR: Python >= 3.12 with PyYAML 6.x (or uv) is required")
		return nil, 1
	}
	bin := filepath.Join(work, "bin")
	if err := os.MkdirAll(bin, fsperm.Dir); err != nil {
		fmt.Fprintf(stderr, "ERROR: %s\n", err.Error())
		return nil, 1
	}
	script := "#!/usr/bin/env bash\nexec uv run --quiet --no-project --python 3.14 --with 'pyyaml==6.*' python \"$@\"\n"
	path := filepath.Join(bin, "python3")
	if err := os.WriteFile(path, []byte(script), fsperm.Dir); err != nil {
		fmt.Fprintf(stderr, "ERROR: %s\n", err.Error())
		return nil, 1
	}
	return []string{"PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH")}, 0
}

func stageCompose(ctx context.Context, root, stage string) error {
	if err := os.MkdirAll(filepath.Join(stage, "compose"), fsperm.Dir); err != nil {
		return err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == "compose" || name == ".env" {
			continue
		}
		if err := os.Symlink(filepath.Join(root, name), filepath.Join(stage, name)); err != nil {
			return err
		}
	}
	composeDir := filepath.Join(root, "compose")
	children, err := os.ReadDir(composeDir)
	if err != nil {
		return err
	}
	for _, entry := range children {
		if entry.Name() == ".env" {
			continue
		}
		src := filepath.Join(composeDir, entry.Name())
		dst := filepath.Join(stage, "compose", entry.Name())
		if code, err := run(ctx, io.Discard, io.Discard, root, nil, "cp", "-R", src, dst); err != nil || code != 0 {
			return fmt.Errorf("copy %s: %v", entry.Name(), err)
		}
	}
	return nil
}
