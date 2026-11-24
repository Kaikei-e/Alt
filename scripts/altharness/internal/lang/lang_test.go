package lang

import (
	"alt/scripts/altharness/internal/fsperm"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
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

func TestAdrRunsWithoutPython3(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "scripts", "lint"), fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "docs", "ADR"), fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "scripts", "lint", "run.sh"), []byte("#!/usr/bin/env bash\n"), fsperm.Dir); err != nil {
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
	if err := os.WriteFile(filepath.Join(stubBin, "docdag"), []byte("#!/bin/sh\necho stub-docdag \"$@\"\n"), fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stubBin, "go"), []byte("#!/bin/sh\nprintf '%s: go1.27.4\\n\\tpath\\tgithub.com/Kaikei-e/DocDag/cmd/docdag\\n\\tmod\\tgithub.com/Kaikei-e/DocDag\\tv0.6.0\\th1:stub\\n' \"$3\"\n"), fsperm.Dir); err != nil {
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
