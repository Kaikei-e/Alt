package lang

import (
	"alt/scripts/altharness/internal/canary"
	"alt/scripts/altharness/internal/ratchet"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// RunRust is scripts/lint/rust.sh. The clippy canary runs before cargo fmt and clippy.
func RunRust(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	ctx, stdout, stderr, code := prepare(ctx, stdout, stderr)
	if code != 0 {
		return code
	}
	if len(args) != 1 || args[0] == "" {
		fmt.Fprintln(stderr, "usage: rust.sh <project-dir>")
		return 2
	}
	root, rc := mustRepo(stderr)
	if rc != 0 {
		return rc
	}
	rel := strings.TrimSuffix(strings.TrimPrefix(args[0], "./"), "/")
	if invalidRustRel(rel) {
		fmt.Fprintf(stderr, "error: invalid project directory: %s\n", args[0])
		return 2
	}
	project := filepath.Join(root, rel)
	if st, err := os.Stat(project); err != nil || !st.IsDir() {
		fmt.Fprintf(stderr, "error: %s is not a Rust project directory (missing Cargo.toml)\n", rel)
		return 2
	}
	if _, err := os.Stat(filepath.Join(project, "Cargo.toml")); err != nil {
		fmt.Fprintf(stderr, "error: %s is not a Rust project directory (missing Cargo.toml)\n", rel)
		return 2
	}
	toolchain, ok := findToolchain(project, root)
	if !ok {
		fmt.Fprintf(stderr, "error: rust-toolchain.toml not found in %s (or ancestors up to repo root)\n", rel)
		return 1
	}
	data, err := os.ReadFile(toolchain)
	if err != nil {
		fmt.Fprintf(stderr, "error: failed to read channel from %s\n", trimRepo(root, toolchain))
		return 1
	}
	channel := firstSubmatch(`(?m)^\s*channel\s*=\s*"([^"]*)"`, string(data))
	if channel == "" {
		fmt.Fprintf(stderr, "error: failed to read channel from %s\n", trimRepo(root, toolchain))
		return 1
	}
	targets := rustTargets(string(data))
	versionLine, versionErr, versionCode, runErr := capture(ctx, project, nil, "cargo", "--version")
	if runErr != nil || versionCode != 0 {
		fmt.Fprintf(stderr, "error: failed to run 'cargo --version' in %s (expected channel '%s' from %s):\n", rel, channel, trimRepo(root, toolchain))
		if strings.TrimSpace(versionLine+versionErr) == "" {
			fmt.Fprintln(stderr, "ERROR: Required command not found in PATH: cargo")
		} else {
			fmt.Fprint(stderr, versionLine)
			fmt.Fprint(stderr, versionErr)
		}
		return 1
	}
	fields := strings.Fields(strings.TrimSpace(versionLine))
	active := ""
	if len(fields) >= 2 {
		active = fields[1]
	}
	if active != channel {
		fmt.Fprintf(stderr, "error: active cargo version '%s' does not match channel '%s' in %s (%s)\n", active, channel, trimRepo(root, toolchain), strings.TrimSpace(versionLine))
		return 1
	}
	fmt.Fprintf(stdout, "==> [%s] canary audit (cargo clippy)\n", rel)
	if canary.RunAudit(ctx, []string{"rust-canary", "--project-dir", project, "--toolchain-file", toolchain}, stdout, stderr) != 0 {
		return 1
	}
	fmtStatus, clippyStatus := "pass", "pass"
	overall := 0
	policy := os.Getenv("LINT_POLICY")
	if policy == "" {
		policy = "fix-existing"
	}
	if code, err := run(ctx, stdout, stderr, project, nil, "cargo", "fmt", "--all", "--", "--check"); err != nil || code != 0 {
		fmtStatus = "fail"
		overall = 1
	}
	if code := runClippy(ctx, stdout, stderr, root, project, policy, []string{"--all-targets", "--all-features"}); code != 0 {
		clippyStatus = "fail"
		overall = 1
	}
	for _, extra := range targets {
		if code := runClippy(ctx, stdout, stderr, root, project, policy, []string{"--lib", "--target", extra, "--all-features"}); code != 0 {
			clippyStatus = "fail"
			overall = 1
		}
	}
	fmt.Fprintf(stdout, "rust-lint summary (%s):\n", rel)
	fmt.Fprintf(stdout, "  fmt: %s\n", fmtStatus)
	fmt.Fprintf(stdout, "  clippy: %s\n", clippyStatus)
	return overall
}

func invalidRustRel(rel string) bool {
	switch {
	case rel == "", strings.HasPrefix(rel, "/"), rel == "..", strings.HasPrefix(rel, "../"):
		return true
	case strings.HasSuffix(rel, "/..") || strings.Contains(rel, "/../"):
		return true
	default:
		return false
	}
}

func findToolchain(project, root string) (string, bool) {
	dir := project
	for {
		candidate := filepath.Join(dir, "rust-toolchain.toml")
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			return candidate, true
		}
		if dir == root || dir == "/" || dir == "." {
			return "", false
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

func rustTargets(text string) []string {
	raw := firstSubmatch(`(?m)^\s*targets\s*=\s*\[(.*)\]`, text)
	if raw == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if len(part) >= 2 && part[0] == '"' && part[len(part)-1] == '"' {
			out = append(out, part[1:len(part)-1])
		}
	}
	return out
}

func runClippy(ctx context.Context, stdout, stderr io.Writer, root, project, policy string, clippyArgs []string) int {
	args := append([]string{"cargo", "clippy"}, clippyArgs...)
	if policy != "block-new" {
		args = append(args, "--", "-D", "warnings")
		code, err := run(ctx, stdout, stderr, project, nil, args...)
		if err != nil {
			fmt.Fprintln(stderr, "ERROR: Required command not found in PATH: cargo")
			return 1
		}
		return code
	}
	tmp, err := os.CreateTemp("", "alt-clippy-*.json")
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err.Error())
		return 1
	}
	path := tmp.Name()
	defer os.Remove(path)
	args = append(args, "--message-format=json", "--", "-D", "warnings")
	code, runErr := run(ctx, tmp, stderr, project, nil, args...)
	tmp.Close()
	if runErr != nil {
		fmt.Fprintln(stderr, "ERROR: Required command not found in PATH: cargo")
		return 1
	}
	if code == 0 {
		return 0
	}
	return ratchet.Run(ctx, []string{"check-fingerprints",
		"--repo-root", root, "--project-dir", project, "--clippy-json", path, "--require-findings"}, stdout, stderr)
}

func firstSubmatch(pattern, text string) string {
	re := regexp.MustCompile(pattern)
	m := re.FindStringSubmatch(text)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

func trimRepo(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}
