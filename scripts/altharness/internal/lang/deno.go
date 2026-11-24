package lang

import (
	"alt/scripts/altharness/internal/canary"
	"alt/scripts/altharness/internal/fsperm"
	"alt/scripts/altharness/internal/ratchet"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// RunDeno is scripts/lint/deno.sh. The lint canary runs before deno fmt and deno lint.
func RunDeno(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	ctx, stdout, stderr, code := prepare(ctx, stdout, stderr)
	if code != 0 {
		return code
	}
	if len(args) != 1 || args[0] == "" {
		fmt.Fprintln(stderr, "Usage: deno.sh <project-dir>")
		return 2
	}
	root, rc := mustRepo(stderr)
	if rc != 0 {
		return rc
	}
	rel := strings.TrimSuffix(args[0], "/")
	project := filepath.Join(root, rel)
	if st, err := os.Stat(project); err != nil || !st.IsDir() {
		fmt.Fprintf(stderr, "Project directory does not exist: %s\n", project)
		return 1
	}
	pinFile := filepath.Join(project, ".dvmrc")
	data, err := os.ReadFile(pinFile)
	if err != nil {
		fmt.Fprintf(stderr, "Missing Deno version pin file: %s\n", pinFile)
		return 1
	}
	pinned := strings.TrimPrefix(strings.Join(strings.Fields(string(data)), ""), "v")
	if pinned == "" {
		fmt.Fprintf(stderr, "Empty Deno version in %s\n", pinFile)
		return 1
	}
	bin, rc := resolveDeno(ctx, stderr, pinned, pinFile)
	if rc != 0 {
		return rc
	}
	actual, ok := denoVersion(ctx, bin)
	if !ok || actual != pinned {
		shown := actual
		if shown == "" {
			shown = "unknown"
		}
		fmt.Fprintf(stderr, "Expected Deno %s (from %s), but resolved %s at %s\n", pinned, pinFile, shown, bin)
		return 1
	}
	fmt.Fprintf(stdout, "==> [%s] canary audit (deno lint)\n", rel)
	if canary.RunAudit(ctx, []string{"deno-canary", "--project-dir", project, "--deno-bin", bin}, stdout, stderr) != 0 {
		return 1
	}
	fmtStatus := "pass"
	if code, err := run(ctx, stdout, stderr, project, nil, bin, "fmt", "--check"); err != nil || code != 0 {
		fmtStatus = "fail"
	}
	lintStatus := "pass"
	policy := os.Getenv("LINT_POLICY")
	if policy == "" {
		policy = "fix-existing"
	}
	if policy == "block-new" {
		tmp, err := os.CreateTemp("", "alt-deno-lint-*.json")
		if err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err.Error())
			return 1
		}
		path := tmp.Name()
		defer os.Remove(path)
		code, runErr := run(ctx, tmp, stderr, project, nil, bin, "lint", "--json")
		tmp.Close()
		if runErr != nil {
			lintStatus = "fail"
		} else if code != 0 {
			fp := ratchet.Run(ctx, []string{"check-fingerprints",
				"--repo-root", root, "--project-dir", project, "--deno-json", path, "--require-findings"}, stdout, stderr)
			if fp != 0 {
				lintStatus = "fail"
			} else {
				lintStatus = "pass (block-new)"
			}
		}
	} else if code, err := run(ctx, stdout, stderr, project, nil, bin, "lint"); err != nil || code != 0 {
		lintStatus = "fail"
	}
	fmt.Fprintf(stdout, "=== deno.sh summary (%s, deno %s) ===\n", rel, pinned)
	fmt.Fprintf(stdout, "deno fmt --check: %s\n", fmtStatus)
	fmt.Fprintf(stdout, "deno lint: %s\n", lintStatus)
	if fmtStatus != "pass" || lintStatus == "fail" {
		return 1
	}
	return 0
}

func resolveDeno(ctx context.Context, stderr io.Writer, pinned, pinFile string) (string, int) {
	if path, err := exec.LookPath("deno"); err == nil {
		if ver, ok := denoVersion(ctx, path); ok && ver == pinned {
			return path, 0
		}
	}
	base, err := cacheHome()
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err.Error())
		return "", 1
	}
	cacheDir := filepath.Join(base, "alt-lint", "deno", pinned)
	cached := filepath.Join(cacheDir, "deno")
	if st, err := os.Stat(cached); err == nil && st.Mode()&0o111 != 0 {
		if ver, ok := denoVersion(ctx, cached); ok && ver == pinned {
			return cached, 0
		}
	}
	if _, err := exec.LookPath("deno"); err != nil {
		fmt.Fprintf(stderr, "Deno %s is required (%s), but no deno binary was found on PATH or at %s\n", pinned, pinFile, cached)
		return "", 1
	}
	if err := os.MkdirAll(cacheDir, fsperm.Dir); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err.Error())
		return "", 1
	}
	tmp := filepath.Join(cacheDir, fmt.Sprintf("deno.tmp.%d", os.Getpid()))
	_ = os.Remove(tmp)
	code, runErr := run(ctx, stderr, stderr, cacheDir, nil, "deno", "upgrade", "--force", "--version", pinned, "--output", tmp)
	if runErr != nil || code != 0 {
		fmt.Fprintf(stderr, "error: deno upgrade to %s failed\n", pinned)
		return "", 1
	}
	if err := os.Chmod(tmp, fsperm.Dir); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err.Error())
		return "", 1
	}
	if err := os.Rename(tmp, cached); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err.Error())
		return "", 1
	}
	return cached, 0
}

func denoVersion(ctx context.Context, bin string) (string, bool) {
	out, _, code, err := capture(ctx, "", nil, bin, "--version")
	if err != nil || code != 0 {
		return "", false
	}
	line := out
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return "", false
	}
	return fields[1], true
}
