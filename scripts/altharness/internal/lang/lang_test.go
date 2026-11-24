package lang

import (
	"alt/scripts/altharness/internal/fsperm"
	"bytes"
	"context"
	"os"
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
