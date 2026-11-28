package guards

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"alt/scripts/altharness/internal/fsperm"
)

func TestRunLintChanged_ProvenToFailOnUnusedTestHelper(t *testing.T) {
	if _, err := exec.LookPath("golangci-lint"); err != nil {
		t.Skip("golangci-lint not on PATH")
	}

	// Locate real repo .golangci.yml so this test proves detection against
	// the exact CI lint configuration.
	realRoot, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	cfgBytes, err := os.ReadFile(filepath.Join(realRoot, ".golangci.yml"))
	if err != nil {
		t.Fatalf("read repo .golangci.yml: %v", err)
	}

	repo := initTestGitRepo(t)
	if err := os.WriteFile(filepath.Join(repo, ".golangci.yml"), cfgBytes, fsperm.File); err != nil {
		t.Fatal(err)
	}
	svcDir := filepath.Join(repo, "svc", "app")
	pkgDir := filepath.Join(svcDir, "pkg")
	if err := os.MkdirAll(pkgDir, fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(svcDir, "go.mod"), []byte("module example.com/svc\n\ngo 1.24.0\n"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "pkg.go"), []byte("package pkg\n\nfunc Hello() string { return \"ok\" }\n"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	runGitInTest(t, repo, "add", ".golangci.yml", "svc/app/go.mod", "svc/app/pkg/pkg.go")
	runGitInTest(t, repo, "commit", "-m", "init go module")

	t.Setenv("ALT_HARNESS_REPO_ROOT", repo)
	t.Setenv("ALT_ACTIVE_PACKET", "")

	// 1. Clean repo must pass immediately.
	var stdout, stderr bytes.Buffer
	if rc := RunLintChanged(context.Background(), []string{"--root", repo}, &stdout, &stderr); rc != 0 {
		t.Fatalf("clean repo should pass: rc=%d out=%q err=%q", rc, stdout.String(), stderr.String())
	}

	// 2. Proven-to-Fail: reproduce the exact PR #418 failure (unused fakeReadPort in *_test.go).
	badTest := `package pkg

import "testing"

type fakeReadPort struct {
	items []string
}

func (f *fakeReadPort) List() []string {
	return f.items
}

func TestHello(t *testing.T) {
	if Hello() != "ok" {
		t.Fatal("unexpected")
	}
}
`
	if err := os.WriteFile(filepath.Join(pkgDir, "pkg_test.go"), []byte(badTest), fsperm.File); err != nil {
		t.Fatal(err)
	}

	stdout.Reset()
	stderr.Reset()
	rc := RunLintChanged(context.Background(), []string{"--root", repo}, &stdout, &stderr)
	combined := stdout.String() + stderr.String()
	if rc == 0 || !strings.Contains(combined, "fakeReadPort") || !strings.Contains(combined, "unused") {
		t.Fatalf("expected Proven-to-Fail unused finding, got rc=%d output:\n%s", rc, combined)
	}

	// Verify agy-hook stop blocks completion ("decision":"continue") on the real failure.
	origLayers, origDocdag, origLint := agyRunLayersFn, agyRunDocdagValidateFn, agyRunChangedLintFn
	t.Cleanup(func() {
		agyRunLayersFn = origLayers
		agyRunDocdagValidateFn = origDocdag
		agyRunChangedLintFn = origLint
	})
	agyRunLayersFn = func(_ context.Context, _ string) (int, string) { return 0, "" }
	agyRunDocdagValidateFn = func(_ context.Context, _ string) (int, string) { return 0, "" }
	agyRunChangedLintFn = defaultAgyRunChangedLint

	stopPayload := `{"conversationId":"c-lint","cwd":"` + repo + `","continuationActive":false,"terminationReason":"NO_TOOL_CALL"}`
	stdout.Reset()
	stderr.Reset()
	hookRC := RunAgyHook(context.Background(), []string{"stop"}, strings.NewReader(stopPayload), &stdout, &stderr)
	if hookRC != 0 || !strings.Contains(stdout.String(), `"continue"`) || !strings.Contains(stdout.String(), "fakeReadPort") {
		t.Fatalf("agy-hook stop must block on dirty Go lint violation: rc=%d out=%s", hookRC, stdout.String())
	}

	// 3. Verify Claude/Codex --hook mode returns exit 2 on failure and exit 0 when stop_hook_active=true.
	stdout.Reset()
	stderr.Reset()
	if rc := RunLintChangedWithStdin(context.Background(), []string{"--hook", "--root", repo}, strings.NewReader(`{"stop_hook_active":false}`), &stdout, &stderr); rc != 2 || !strings.Contains(stderr.String(), "fakeReadPort") {
		t.Fatalf("--hook mode should exit 2 with finding: rc=%d err=%q", rc, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if rc := RunLintChangedWithStdin(context.Background(), []string{"--hook", "--root", repo}, strings.NewReader(`{"stop_hook_active":true}`), &stdout, &stderr); rc != 0 {
		t.Fatalf("--hook mode with stop_hook_active=true must allow to avoid infinite loop: rc=%d", rc)
	}

	// 4. Fixed state: exercise fakeReadPort -> both RunLintChanged and agy-hook stop PASS.
	goodTest := `package pkg

import "testing"

type fakeReadPort struct {
	items []string
}

func (f *fakeReadPort) List() []string {
	return f.items
}

func TestHello(t *testing.T) {
	if Hello() != "ok" {
		t.Fatal("unexpected")
	}
	p := &fakeReadPort{items: []string{"a"}}
	if len(p.List()) != 1 {
		t.Fatal("expected 1")
	}
}
`
	if err := os.WriteFile(filepath.Join(pkgDir, "pkg_test.go"), []byte(goodTest), fsperm.File); err != nil {
		t.Fatal(err)
	}

	stdout.Reset()
	stderr.Reset()
	if rc := RunLintChanged(context.Background(), []string{"--root", repo}, &stdout, &stderr); rc != 0 {
		t.Fatalf("fixed Go test should pass lint-changed: rc=%d out=%q err=%q", rc, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	hookRC = RunAgyHook(context.Background(), []string{"stop"}, strings.NewReader(stopPayload), &stdout, &stderr)
	if hookRC != 0 || !strings.Contains(stdout.String(), `"allow"`) {
		t.Fatalf("fixed state must allow stop: rc=%d out=%s", hookRC, stdout.String())
	}
}

func TestDefaultAgyFormatFile_ResolvesNearestProjectRoot(t *testing.T) {
	repo := t.TempDir()
	pyRoot := filepath.Join(repo, "news-creator", "app")
	pyFile := filepath.Join(pyRoot, "tests", "sample.py")
	tsRoot := filepath.Join(repo, "alt-frontend-sv")
	tsFile := filepath.Join(tsRoot, "src", "sample.ts")
	if err := os.MkdirAll(filepath.Dir(pyFile), fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(tsFile), fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tsRoot, "node_modules", ".bin"), fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pyRoot, "pyproject.toml"), []byte("[project]\nname='nc'\n"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pyFile, []byte("x=1\n"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tsRoot, "package.json"), []byte("{}\n"), fsperm.File); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tsFile, []byte("const x=1;\n"), fsperm.File); err != nil {
		t.Fatal(err)
	}

	binDir := t.TempDir()
	uvLog := filepath.Join(binDir, "uv.log")
	biomeLog := filepath.Join(binDir, "biome.log")

	uvStub := "#!/bin/sh\nprintf '%s|%s\\n' \"$(pwd)\" \"$*\" >> \"" + uvLog + "\"\n"
	if err := fsperm.WriteExecutable(filepath.Join(binDir, "uv"), []byte(uvStub)); err != nil {
		t.Fatal(err)
	}
	biomeStub := "#!/bin/sh\nprintf '%s|%s\\n' \"$(pwd)\" \"$*\" >> \"" + biomeLog + "\"\n"
	if err := fsperm.WriteExecutable(filepath.Join(tsRoot, "node_modules", ".bin", "biome"), []byte(biomeStub)); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	defaultAgyFormatFile(context.Background(), repo, pyFile)
	defaultAgyFormatFile(context.Background(), repo, tsFile)

	uvGot, err := os.ReadFile(uvLog)
	if err != nil {
		t.Fatalf("expected uv to run: %v", err)
	}
	if !strings.HasPrefix(string(uvGot), pyRoot+"|") {
		t.Fatalf("uv ran in wrong dir: %q, want prefix %q", string(uvGot), pyRoot+"|")
	}

	biomeGot, err := os.ReadFile(biomeLog)
	if err != nil {
		t.Fatalf("expected local biome to run: %v", err)
	}
	if !strings.HasPrefix(string(biomeGot), tsRoot+"|") {
		t.Fatalf("biome ran in wrong dir: %q, want prefix %q", string(biomeGot), tsRoot+"|")
	}
}
