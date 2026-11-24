package lang

import (
	"alt/scripts/altharness/internal/canary"
	"alt/scripts/altharness/internal/fsperm"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const bufVersion = "1.48.0"

// ProtoBreakingAgainst is the local buf breaking input. It names origin/main
// inside this checkout and does not call GitHub.
const ProtoBreakingAgainst = ".git#branch=origin/main,subdir=proto"

// RunProto is scripts/lint/proto.sh. The buf canary runs before buf lint,
// buf breaking, and the allowlist drift check.
func RunProto(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	ctx, stdout, stderr, code := prepare(ctx, stdout, stderr)
	if code != 0 {
		return code
	}
	root, rc := mustRepo(stderr)
	if rc != 0 {
		return rc
	}
	target := "proto"
	if len(args) > 0 && args[0] != "" {
		target = args[0]
	}
	if _, err := exec.LookPath("buf"); err != nil {
		fmt.Fprintln(stderr, "error: required command not found in PATH: buf")
		return 1
	}
	verOut, verErr, verCode, verRunErr := capture(ctx, root, nil, "buf", "--version")
	if verRunErr != nil || verCode != 0 {
		fmt.Fprintf(stderr, "error: failed to run 'buf --version': %s\n", strings.TrimSpace(verOut+verErr))
		return 1
	}
	actual := strings.TrimPrefix(strings.Join(strings.Fields(verOut), ""), "v")
	if actual != bufVersion {
		fmt.Fprintf(stderr, "error: buf version mismatch: expected %s, got '%s'\n", bufVersion, actual)
		return 1
	}
	fmt.Fprintf(stdout, "==> [%s] canary audit (buf lint)\n", target)
	if canary.RunAudit(ctx, []string{"proto-canary"}, stdout, stderr) != 0 {
		return 1
	}
	tmp, err := os.MkdirTemp("", "alt-proto-")
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err.Error())
		return 1
	}
	defer os.RemoveAll(tmp)
	labels := []string{"buf-lint", "buf-breaking", "allowlist-drift"}
	codes := []int{
		stepCode(ctx, stdout, stderr, filepath.Join(root, target), nil, "buf", "lint"),
		stepCode(ctx, stdout, stderr, filepath.Join(root, target), nil, bufBreakingArgs(root, target)...),
		allowlistDrift(ctx, root, target, tmp, stdout, stderr),
	}
	fail := 0
	for i, label := range labels {
		if codes[i] == 0 {
			fmt.Fprintf(stdout, "PASS: %s\n", label)
			continue
		}
		fmt.Fprintf(stdout, "FAIL: %s\n", label)
		fail = 1
	}
	return fail
}

func bufBreakingArgs(root, target string) []string {
	if explicit := strings.TrimSpace(os.Getenv("ALT_PROTO_BREAKING_AGAINST")); explicit != "" {
		return []string{"buf", "breaking", "--against", explicit}
	}
	if target == "" {
		target = "proto"
	}
	against := filepath.Join(root, target)
	if root != "" {
		gitDir := resolveGitCommonDir(root)
		if gitDir != "" {
			if exec.Command("git", "-C", root, "rev-parse", "--verify", "--quiet", "origin/main^{commit}").Run() == nil {
				against = gitDir + "#branch=origin/main,subdir=" + target
			} else if exec.Command("git", "-C", root, "rev-parse", "--verify", "--quiet", "HEAD^{commit}").Run() == nil {
				against = gitDir + "#ref=HEAD,subdir=" + target
			}
		}
	}
	return []string{"buf", "breaking", "--against", against}
}

func resolveGitCommonDir(root string) string {
	out, err := exec.Command("git", "-C", root, "rev-parse", "--git-common-dir").Output()
	if err != nil {
		return ""
	}
	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return ""
	}
	if filepath.IsAbs(raw) {
		return raw
	}
	return filepath.Clean(filepath.Join(root, raw))
}

func stepCode(ctx context.Context, stdout, stderr io.Writer, dir string, env []string, args ...string) int {
	code, err := run(ctx, stdout, stderr, dir, env, args...)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: Required command not found in PATH: %s\n", args[0])
		return 1
	}
	return code
}

func allowlistDrift(ctx context.Context, root, target, tmp string, stdout, stderr io.Writer) int {
	outDir := filepath.Join(tmp, "protovis-out")
	if err := os.MkdirAll(outDir, fsperm.Dir); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err.Error())
		return 1
	}
	desc := filepath.Join(tmp, "descriptor.binpb")
	if code, err := run(ctx, io.Discard, stderr, filepath.Join(root, target), nil, "buf", "build", "--exclude-source-info", "-o", desc); err != nil || code != 0 {
		fmt.Fprintln(stderr, "buf build failed.")
		return 1
	}
	if code, err := run(ctx, io.Discard, stderr, filepath.Join(root, target, "tools", "protovis"), nil, "go", "run", ".", "-desc", desc, "-root", outDir); err != nil || code != 0 {
		fmt.Fprintln(stderr, "protovis generator failed.")
		return 1
	}
	stale := false
	for _, rel := range []string{
		"alt-butterfly-facade/internal/server/allowlist_gen.go",
		"alt-frontend-sv/src/lib/gen/allowlist.ts",
	} {
		repoFile := filepath.Join(root, rel)
		genFile := filepath.Join(outDir, rel)
		if _, err := os.Stat(repoFile); err != nil {
			fmt.Fprintf(stderr, "Generated allowlist missing: %s\n", rel)
			stale = true
			continue
		}
		if code, err := run(ctx, stdout, stderr, root, nil, "diff", "-u", repoFile, genFile); err != nil || code != 0 {
			stale = true
		}
	}
	if stale {
		fmt.Fprintln(stderr, "Generated allowlists are stale. Re-run protovis and commit the result.")
		return 1
	}
	return 0
}
