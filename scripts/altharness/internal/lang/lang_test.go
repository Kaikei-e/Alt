package lang

import (
	"alt/scripts/altharness/internal/fsperm"
	"alt/scripts/altharness/internal/lint"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestRunGoUsage(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"no arg", nil},
		{"empty", []string{""}},
		{"absolute", []string{"/tmp"}},
		{"two args", []string{"a", "b"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out, errBuf bytes.Buffer
			code := RunGo(context.Background(), tc.args, &out, &errBuf)
			if code != 2 {
				t.Fatalf("exit %d, want 2\n%s", code, errBuf.String())
			}
			if !strings.Contains(errBuf.String(), "Usage:") || !strings.Contains(errBuf.String(), "project-dir") {
				t.Fatalf("usage text:\n%s", errBuf.String())
			}
		})
	}
}

func TestProtoBreakingRef(t *testing.T) {
	cases := []struct {
		name string
		ref  string
	}{
		{"constant", ProtoBreakingAgainst},
		{"argv", strings.Join(bufBreakingArgs(findRepo(t), "proto"), " ")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(tc.ref, "origin/main") {
				t.Fatalf("%s missing origin/main: %s", tc.name, tc.ref)
			}
			if strings.Contains(tc.ref, "github.com") {
				t.Fatalf("%s still points at github.com: %s", tc.name, tc.ref)
			}
			if !strings.Contains(tc.ref, "subdir=proto") {
				t.Fatalf("%s missing subdir=proto: %s", tc.name, tc.ref)
			}
		})
	}
	// Non-git directory must fall back locally, never to github.com.
	nonGit := t.TempDir()
	offline := strings.Join(bufBreakingArgs(nonGit, "proto"), " ")
	if strings.Contains(offline, "github.com") || strings.Contains(offline, "https://") {
		t.Fatalf("non-git fallback hit remote URL: %s", offline)
	}
	// Explicit ALT_PROTO_BREAKING_AGAINST override takes precedence.
	t.Setenv("ALT_PROTO_BREAKING_AGAINST", "/custom/proto")
	override := bufBreakingArgs(nonGit, "proto")
	if len(override) != 4 || override[3] != "/custom/proto" {
		t.Fatalf("override=%v, want --against /custom/proto", override)
	}
}

func TestHostFinalRejectsBackground(t *testing.T) {
	repo := findRepo(t)
	cases := []struct {
		name string
		cmd  string
		tok  string
	}{
		{"ampersand", "echo hi &", "foreground"},
		{"nohup", "nohup echo hi", "nohup"},
		{"autofix", "golangci-lint run --fix", "GUARD-01"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out, errBuf bytes.Buffer
			code := RunHostFinal(context.Background(), []string{"--repo-root", repo, "--target", "scripts", "--test-cmd", tc.cmd}, &out, &errBuf)
			if code != 2 {
				t.Fatalf("exit %d\nstdout:\n%s\nstderr:\n%s", code, out.String(), errBuf.String())
			}
			if !strings.Contains(errBuf.String(), tc.tok) {
				t.Fatalf("missing %q\n%s", tc.tok, errBuf.String())
			}
			if strings.Contains(out.String(), "worktree hygiene") {
				t.Fatal("background command reached later steps")
			}
		})
	}
}

func TestBackgroundPredicate(t *testing.T) {
	cases := []struct {
		cmd string
		bad bool
	}{
		{"echo hi &", true},
		{"nohup sleep 1", true},
		{"echo hi && echo there", false},
		{"echo hi > out.txt", false},
		{"echo hi 2>&1", false},
		{"echo 'a & b'", false},
	}
	for _, tc := range cases {
		t.Run(tc.cmd, func(t *testing.T) {
			if got := hasBackgroundAmpersand(tc.cmd); got != tc.bad {
				t.Fatalf("hasBackgroundAmpersand(%q)=%v", tc.cmd, got)
			}
		})
	}
}

func TestLayersViolation(t *testing.T) {
	dir := t.TempDir()
	handler := filepath.Join(dir, "handler")
	if err := os.MkdirAll(handler, fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(handler, "h.go"), []byte("package handler\n\nimport \"example.com/svc/driver\"\n"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	var out, errBuf bytes.Buffer
	code := RunLayers(context.Background(), []string{dir}, &out, &errBuf)
	if code != 1 {
		t.Fatalf("exit %d\n%s\n%s", code, out.String(), errBuf.String())
	}
	if !strings.Contains(out.String(), "VIOLATION") || !strings.Contains(out.String(), "rest-imports-driver-gateway-or-pgx") {
		t.Fatalf("missing violation\n%s", out.String())
	}
	if !strings.Contains(out.String(), "FIX ") {
		t.Fatalf("missing FIX\n%s", out.String())
	}
}

func TestLayersHookClaudeAndAgy(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := filepath.Join(dir, "svc")
	handler := filepath.Join(svc, "handler")
	cleanDomain := filepath.Join(svc, "domain")
	if err := os.MkdirAll(handler, fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(cleanDomain, fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(svc, "go.mod"), []byte("module example.com/svc\n\ngo 1.24\n"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	badFile := filepath.Join(handler, "h.go")
	if err := os.WriteFile(badFile, []byte("package handler\n\nimport \"example.com/svc/driver\"\n"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	goodFile := filepath.Join(cleanDomain, "item.go")
	if err := os.WriteFile(goodFile, []byte("package domain\n\ntype Item struct{}\n"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	mustJSON := func(v any) string {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}

	cases := []struct {
		name     string
		payload  string
		wantCode int
		wantErr  string
	}{
		{
			name:     "claude tool_input violation",
			payload:  mustJSON(map[string]any{"tool_input": map[string]any{"file_path": badFile}}),
			wantCode: 2,
			wantErr:  "VIOLATION rest-imports-driver-gateway-or-pgx",
		},
		{
			name:     "agy toolCall Path violation",
			payload:  mustJSON(map[string]any{"toolCall": map[string]any{"args": map[string]any{"Path": badFile}}}),
			wantCode: 2,
			wantErr:  "VIOLATION rest-imports-driver-gateway-or-pgx",
		},
		{
			name:     "agy toolCall TargetFile violation",
			payload:  mustJSON(map[string]any{"toolCall": map[string]any{"args": map[string]any{"TargetFile": badFile}}}),
			wantCode: 2,
			wantErr:  "VIOLATION rest-imports-driver-gateway-or-pgx",
		},
		{
			name:     "agy toolCall clean file",
			payload:  mustJSON(map[string]any{"toolCall": map[string]any{"args": map[string]any{"Path": goodFile}}}),
			wantCode: 0,
		},
		{
			name:     "invalid json silent allow",
			payload:  "not-json{{{",
			wantCode: 0,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var out, errBuf bytes.Buffer
			code := RunLayersWithStdin(context.Background(), []string{"--hook"}, strings.NewReader(tc.payload), &out, &errBuf)
			if code != tc.wantCode {
				t.Fatalf("code=%d want=%d out=%q err=%q", code, tc.wantCode, out.String(), errBuf.String())
			}
			if tc.wantErr != "" {
				if !strings.Contains(errBuf.String(), tc.wantErr) {
					t.Fatalf("stderr missing %q:\n%s", tc.wantErr, errBuf.String())
				}
				if !strings.Contains(errBuf.String(), "FIX rest-imports-driver-gateway-or-pgx:") {
					t.Fatalf("stderr missing FIX:\n%s", errBuf.String())
				}
				if !strings.Contains(errBuf.String(), "See .claude/skills/clean-architecture/SKILL.md section 9.") {
					t.Fatalf("stderr missing SKILL.md pointer:\n%s", errBuf.String())
				}
			}
		})
	}
}

func TestLayersKnownSuppressesClean(t *testing.T) {
	dir := t.TempDir()
	svc := filepath.Join(dir, "svc")
	drv := filepath.Join(svc, "driver")
	if err := os.MkdirAll(drv, fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(svc, "go.mod"), []byte("module example.com/svc\n\ngo 1.24\n"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(drv, "repo.go"), []byte("package driver\n\nimport \"example.com/svc/domain\"\n"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	known := filepath.Join(dir, "known.tsv")
	if err := os.WriteFile(known, []byte("driver-imports-domain\tsvc/driver/\t*\tlegacy driver\n"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CHECK_LAYERS_ROOT", dir)
	t.Setenv("CHECK_LAYERS_KNOWN", known)

	var out, errBuf bytes.Buffer
	code := RunLayers(context.Background(), []string{svc}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), errBuf.String())
	}
	if !strings.Contains(out.String(), "KNOWN driver-imports-domain svc/driver/ (1 lines): legacy driver") {
		t.Fatalf("missing KNOWN line:\n%s", out.String())
	}
	if strings.Contains(out.String(), "[clean] Go driver: clean") {
		t.Fatalf("expected [clean] suppressed when KNOWN matched:\n%s", out.String())
	}
}

func TestLayersChangedFromNonRepoCwd(t *testing.T) {
	repo := findRepo(t)
	checkLayers := filepath.Join(repo, ".claude", "skills", "clean-architecture", "scripts", "check_layers.sh")
	nonGit := t.TempDir()
	cmd := exec.Command("bash", checkLayers, "--changed")
	cmd.Dir = nonGit
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Exit 1 is allowed only if the repo actually has a clean-arch VIOLATION; exit 2 ("not a git repository") is the bug.
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 2 {
			t.Fatalf("check_layers.sh --changed from non-git CWD failed with exit 2:\n%s", string(out))
		}
	}
	if strings.Contains(string(out), "not a git repository") {
		t.Fatalf("expected repoRoot fallback from non-git CWD, got:\n%s", string(out))
	}
}

func TestAdrRunsWithoutPython3(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "scripts", "lint"), fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "docs", "ADR"), fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	if err := fsperm.WriteExecutable(filepath.Join(repo, "scripts", "lint", "run.sh"), []byte("#!/usr/bin/env bash\n")); err != nil {
		t.Fatal(err)
	}
	gitExec := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	gitExec("init", "-b", "main", "-q")
	gitExec("config", "user.email", "t@example.com")
	gitExec("config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(repo, "docs", "ADR", "000001.md"), []byte("---\ntitle: init\n---\n# 1\n"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	gitExec("add", ".")
	gitExec("commit", "-q", "-m", "c1")
	c1 := gitExec("rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(repo, "docs", "ADR", "000002.md"), []byte("---\ntitle: two\n---\n# 2\n"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	gitExec("add", ".")
	gitExec("commit", "-q", "-m", "c2")
	c2 := gitExec("rev-parse", "HEAD")
	gitExec("update-ref", "refs/remotes/origin/main", c2)

	stubBin := t.TempDir()
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(gitPath, filepath.Join(stubBin, "git")); err != nil {
		t.Fatal(err)
	}
	if err := fsperm.WriteExecutable(filepath.Join(stubBin, "docdag"), []byte("#!/bin/sh\necho stub-docdag \"$@\"\n")); err != nil {
		t.Fatal(err)
	}
	if err := fsperm.WriteExecutable(filepath.Join(stubBin, "go"), []byte("#!/bin/sh\nprintf '%s: go1.27.4\\n\\tpath\\tgithub.com/Kaikei-e/DocDag/cmd/docdag\\n\\tmod\\tgithub.com/Kaikei-e/DocDag\\tv0.6.0\\th1:stub\\n' \"$3\"\n")); err != nil {
		t.Fatal(err)
	}
	// Restricted PATH has git, docdag, go — deliberately NO python3!
	t.Setenv("PATH", stubBin)
	t.Setenv("LINT_BASE", "")
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(repo); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origWD) }()

	var out, errBuf bytes.Buffer
	code := RunAdr(context.Background(), nil, &out, &errBuf)
	if code != 0 {
		t.Fatalf("RunAdr without python3 failed: code=%d out=%q err=%q", code, out.String(), errBuf.String())
	}
	if !strings.Contains(out.String(), "base="+c1) {
		t.Fatalf("expected base=%s in output:\n%s", c1, out.String())
	}
}

func TestNoPythonHarnessCallsInLangRunners(t *testing.T) {
	t.Parallel()
	files := []string{"gosh.go", "python.go", "rust.go", "frontend.go", "deno.go", "proto.go", "adr.go", "hostfinal.go"}
	forbidden := []string{"canary_audit.py", "check_ratchet.py", "check_python_versions.py", "check_worktree_hygiene.py", "task_packet.py", "adr_supersede_gate.py"}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		text := string(data)
		for _, tok := range forbidden {
			if strings.Contains(text, tok) {
				t.Errorf("%s still references retired Python script %q", f, tok)
			}
		}
	}
}

func TestPolicyFlagPropagatesInProcessThroughRunKind(t *testing.T) {
	t.Parallel()
	if got := effectivePolicy(lint.WithPolicy(context.Background(), "block-new")); got != "block-new" {
		t.Fatalf("effectivePolicy(ctx=block-new)=%q, want block-new", got)
	}
	if got := effectivePolicy(lint.WithPolicy(context.Background(), "fix-existing")); got != "fix-existing" {
		t.Fatalf("effectivePolicy(ctx=fix-existing)=%q, want fix-existing", got)
	}
}

func TestLintRunPolicyFlagReachesInProcessRustRunner(t *testing.T) {
	repo := t.TempDir()
	crateDir := filepath.Join(repo, "svc-rust")
	if err := os.MkdirAll(filepath.Join(repo, "scripts", "lint"), fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(crateDir, fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	if err := fsperm.WriteExecutable(filepath.Join(repo, "scripts", "lint", "run.sh"), []byte("#!/usr/bin/env bash\n")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(crateDir, "Cargo.toml"), []byte("[package]\nname = \"svc-rust\"\nversion = \"0.1.0\"\n"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(crateDir, "rust-toolchain.toml"), []byte("[toolchain]\nchannel = \"1.99.1\"\n"), fsperm.File); err != nil {
		t.Fatal(err)
	}

	stubBin := t.TempDir()
	cargoLog := filepath.Join(t.TempDir(), "cargo.log")
	cargoScript := "#!/usr/bin/env bash\n" +
		"if [ \"${1:-}\" = \"--version\" ]; then echo \"cargo 1.99.1 (stub)\"; exit 0; fi\n" +
		"if [ \"${1:-}\" = \"clippy\" ] && [[ \"$*\" == *\"--offline\"* ]]; then echo \"error: clippy::eq_op\" >&2; exit 1; fi\n" +
		"echo \"$*\" >> " + cargoLog + "\n" +
		"exit 0\n"
	if err := fsperm.WriteExecutable(filepath.Join(stubBin, "cargo"), []byte(cargoScript)); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", stubBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("LINT_POLICY", "")
	if err := os.Unsetenv("LINT_POLICY"); err != nil {
		t.Fatal(err)
	}
	t.Chdir(repo)

	var out, errBuf bytes.Buffer
	code := lint.Run(context.Background(), repo, []string{"--policy", "block-new", "svc-rust"}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("lint.Run(--policy block-new svc-rust) failed: code=%d stdout=%q stderr=%q", code, out.String(), errBuf.String())
	}
	logged, err := os.ReadFile(cargoLog)
	if err != nil {
		t.Fatalf("read cargo log: %v", err)
	}
	if !strings.Contains(string(logged), "--message-format=json") {
		t.Fatalf("expected block-new mode (--message-format=json) via in-process runKind, got cargo args:\n%s", string(logged))
	}
	if v, ok := os.LookupEnv("LINT_POLICY"); ok {
		t.Fatalf("lint.Run mutated process env LINT_POLICY=%q", v)
	}
}

func TestBunCachePath(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	got, err := pinnedBunCacheFile("1.3.12")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(os.Getenv("XDG_CACHE_HOME"), "alt-lint", "bun", "1.3.12", "bun")
	if got != want {
		t.Fatalf("cache path %s, want %s", got, want)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM oven/bun:1.3.12-alpine AS base\nFROM oven/bun:1.3.12-alpine AS runner\n"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	ver, err := bunPinFromDockerfile(filepath.Join(dir, "Dockerfile"))
	if err != nil || ver != "1.3.12" {
		t.Fatalf("pin %q err %v", ver, err)
	}
}

func findRepo(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "scripts", "lint", "run.sh")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repo root not found")
		}
		dir = parent
	}
}

func initHostFinalFixtureRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "scripts", "lint"), fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	if err := fsperm.WriteExecutable(filepath.Join(repo, "scripts", "lint", "run.sh"), []byte("#!/usr/bin/env bash\nexit 0\n")); err != nil {
		t.Fatal(err)
	}
	for _, sub := range []string{"gosvc1", "gosvc2", "pysvc"} {
		if err := os.MkdirAll(filepath.Join(repo, sub), fsperm.Dir); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(repo, sub, "README.txt"), []byte("ok\n"), fsperm.File); err != nil {
			t.Fatal(err)
		}
	}
	gitExec := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, string(out))
		}
	}
	gitExec("init", "-b", "main", "-q")
	gitExec("config", "user.email", "t@example.com")
	gitExec("config", "user.name", "t")
	gitExec("add", ".")
	gitExec("commit", "-q", "-m", "init")
	return repo
}

func TestHostFinalRandomizedTestsAndValidation(t *testing.T) {
	repo := initHostFinalFixtureRepo(t)
	outside := t.TempDir()
	escLink := filepath.Join(repo, "escape_link")
	if err := os.Symlink(outside, escLink); err != nil {
		t.Fatal(err)
	}

	// 1. Invalid flags, bad seeds, and path escapes must fail closed with exit 2 before running hygiene.
	invalidCases := []struct {
		name string
		args []string
	}{
		{"go-test-dir parent escape", []string{"--repo-root", repo, "--target", "scripts", "--go-test-dir", "../outside"}},
		{"go-test-dir abs outside", []string{"--repo-root", repo, "--target", "scripts", "--go-test-dir", outside}},
		{"go-test-dir symlink escape", []string{"--repo-root", repo, "--target", "scripts", "--go-test-dir", "escape_link"}},
		{"go-test-dir missing dir", []string{"--repo-root", repo, "--target", "scripts", "--go-test-dir", "does-not-exist"}},
		{"go-test-dir missing value", []string{"--repo-root", repo, "--target", "scripts", "--go-test-dir"}},
		{"py-test-dir parent escape", []string{"--repo-root", repo, "--target", "scripts", "--py-test-dir", "../outside"}},
		{"py-test-dir abs outside", []string{"--repo-root", repo, "--target", "scripts", "--py-test-dir", outside}},
		{"py-test-dir symlink escape", []string{"--repo-root", repo, "--target", "scripts", "--py-test-dir", "escape_link"}},
		{"py-test-dir missing value", []string{"--repo-root", repo, "--target", "scripts", "--py-test-dir"}},
		{"random-seed non-int", []string{"--repo-root", repo, "--target", "scripts", "--random-seed", "abc"}},
		{"random-seed zero", []string{"--repo-root", repo, "--target", "scripts", "--random-seed", "0"}},
		{"random-seed negative", []string{"--repo-root", repo, "--target", "scripts", "--random-seed", "-42"}},
		{"random-seed missing value", []string{"--repo-root", repo, "--target", "scripts", "--random-seed"}},
	}
	for _, tc := range invalidCases {
		t.Run("invalid/"+tc.name, func(t *testing.T) {
			var out, errBuf bytes.Buffer
			code := RunHostFinal(context.Background(), tc.args, &out, &errBuf)
			if code != 2 {
				t.Fatalf("code=%d want 2\nstdout:\n%s\nstderr:\n%s", code, out.String(), errBuf.String())
			}
			if strings.Contains(out.String(), "worktree hygiene") {
				t.Fatalf("invalid flag reached hygiene step:\n%s", out.String())
			}
		})
	}

	// Remove symlink so fixture repo worktree stays clean for git status --porcelain check.
	if err := os.Remove(escLink); err != nil {
		t.Fatal(err)
	}

	// Stub `go` and `uv` in PATH to capture exact randomized invocations without external deps.
	stubBin := t.TempDir()
	goLog := filepath.Join(t.TempDir(), "go.log")
	uvLog := filepath.Join(t.TempDir(), "uv.log")
	goScript := "#!/usr/bin/env bash\nprintf 'cwd=%s args=%s\\n' \"$PWD\" \"$*\" >> \"" + goLog + "\"\nexit 0\n"
	uvScript := "#!/usr/bin/env bash\nprintf 'cwd=%s seed=%s args=%s\\n' \"$PWD\" \"${ALT_PYTEST_RANDOM_SEED:-}\" \"$*\" >> \"" + uvLog + "\"\nexit 0\n"
	if err := fsperm.WriteExecutable(filepath.Join(stubBin, "go"), []byte(goScript)); err != nil {
		t.Fatal(err)
	}
	if err := fsperm.WriteExecutable(filepath.Join(stubBin, "uv"), []byte(uvScript)); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", stubBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	// 2. Explicit --random-seed runs both repeatable --go-test-dir and --py-test-dir plus --test-cmd.
	t.Run("explicit seed with go and py dirs and test-cmd", func(t *testing.T) {
		_ = os.Remove(goLog)
		_ = os.Remove(uvLog)
		var out, errBuf bytes.Buffer
		code := RunHostFinal(context.Background(), []string{
			"--repo-root", repo,
			"--target", "scripts",
			"--skip-repo-lint",
			"--test-cmd", "echo legacy-test-cmd-ok",
			"--go-test-dir", "gosvc1",
			"--go-test-dir", filepath.Join(repo, "gosvc2"),
			"--py-test-dir", "pysvc",
			"--random-seed", "424242",
		}, &out, &errBuf)
		if code != 0 {
			t.Fatalf("code=%d\nstdout:\n%s\nstderr:\n%s", code, out.String(), errBuf.String())
		}
		if !strings.Contains(out.String(), "legacy-test-cmd-ok") {
			t.Fatalf("missing --test-cmd output:\n%s", out.String())
		}
		if !strings.Contains(out.String(), "random_seed=424242") {
			t.Fatalf("missing random_seed=424242 in stdout:\n%s", out.String())
		}
		if !strings.Contains(out.String(), "PYTEST_RANDOM_SEED=424242") {
			t.Fatalf("missing PYTEST_RANDOM_SEED=424242 in stdout:\n%s", out.String())
		}
		goData, err := os.ReadFile(goLog)
		if err != nil {
			t.Fatalf("read go log: %v", err)
		}
		goLines := strings.Split(strings.TrimSpace(string(goData)), "\n")
		if len(goLines) != 2 {
			t.Fatalf("expected 2 go test invocations, got %d:\n%s", len(goLines), string(goData))
		}
		for _, wantDir := range []string{"gosvc1", "gosvc2"} {
			if !strings.Contains(string(goData), wantDir) {
				t.Fatalf("go log missing %s:\n%s", wantDir, string(goData))
			}
		}
		if !strings.Contains(string(goData), "args=test -shuffle=424242 -race -count=1 ./...") {
			t.Fatalf("go log missing shuffle/race/count args:\n%s", string(goData))
		}
		uvData, err := os.ReadFile(uvLog)
		if err != nil {
			t.Fatalf("read uv log: %v", err)
		}
		if !strings.Contains(string(uvData), "seed=424242") || !strings.Contains(string(uvData), "_alt_pytest_random_order_plugin") {
			t.Fatalf("uv log missing seed/plugin:\n%s", string(uvData))
		}
	})

	// 3. Auto-generated positive seed when --random-seed is omitted is logged and shared across dirs.
	t.Run("auto-generated positive seed logged and shared", func(t *testing.T) {
		_ = os.Remove(goLog)
		_ = os.Remove(uvLog)
		var out, errBuf bytes.Buffer
		code := RunHostFinal(context.Background(), []string{
			"--repo-root", repo,
			"--target", "scripts",
			"--skip-repo-lint",
			"--go-test-dir", "gosvc1",
			"--py-test-dir", "pysvc",
		}, &out, &errBuf)
		if code != 0 {
			t.Fatalf("code=%d\nstdout:\n%s\nstderr:\n%s", code, out.String(), errBuf.String())
		}
		re := regexp.MustCompile(`random_seed=([0-9]+)`)
		m := re.FindStringSubmatch(out.String())
		if len(m) < 2 {
			t.Fatalf("missing auto-generated random_seed=<n> in stdout:\n%s", out.String())
		}
		seed, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil || seed <= 0 {
			t.Fatalf("invalid auto-generated seed %q: %v", m[1], err)
		}
		goData, err := os.ReadFile(goLog)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(goData), "-shuffle="+m[1]) {
			t.Fatalf("go test did not use logged seed %s:\n%s", m[1], string(goData))
		}
		uvData, err := os.ReadFile(uvLog)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(uvData), "seed="+m[1]) {
			t.Fatalf("pytest-rand did not use logged seed %s:\n%s", m[1], string(uvData))
		}
	})
}
