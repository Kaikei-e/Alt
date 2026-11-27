package verifylive

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"alt/scripts/altharness/internal/fsperm"
)

// The fakes stand in for Python interpreters: each decides from its argv
// whether the readiness import "succeeds", so the tests pin which import the
// resolver asks for without needing a real environment.
const (
	fakePyHandlerReady = "#!/bin/sh\ncase \"$*\" in *news_creator.handler.sse_stream*) exit 0 ;; esac\nexit 1\n"
	fakePyWebOnly      = "#!/bin/sh\ncase \"$*\" in *news_creator*) exit 1 ;; esac\nexit 0\n"
	fakePyBare         = "#!/bin/sh\nexit 1\n"
)

func writeFakePython(t *testing.T, path, script string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), fsperm.Dir); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := fsperm.WriteExecutable(path, []byte(script)); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func fakeRepoWithNewsCreator(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "news-creator", "app"), fsperm.Dir); err != nil {
		t.Fatalf("mkdir news-creator/app: %v", err)
	}
	return root
}

func TestResolvePythonForSSE_AcceptsInterpreterThatImportsTheHandler(t *testing.T) {
	root := fakeRepoWithNewsCreator(t)
	venvPy := filepath.Join(root, "news-creator", "app", ".venv", "bin", "python")
	writeFakePython(t, venvPy, fakePyHandlerReady)
	t.Setenv("PATH", t.TempDir())

	cmd, _, err := resolvePythonForSSE(context.Background(), root)
	if err != nil {
		t.Fatalf("resolvePythonForSSE: %v", err)
	}
	if len(cmd) != 1 || cmd[0] != venvPy {
		t.Fatalf("resolved %v, want the news-creator .venv interpreter %s", cmd, venvPy)
	}
}

func TestResolvePythonForSSE_RejectsInterpreterWithOnlyFastAPIAndUvicorn(t *testing.T) {
	root := fakeRepoWithNewsCreator(t)
	bin := t.TempDir()
	writeFakePython(t, filepath.Join(bin, "python3"), fakePyWebOnly)
	t.Setenv("PATH", bin)

	cmd, detail, err := resolvePythonForSSE(context.Background(), root)
	if err == nil {
		t.Fatalf("resolved %v (%s); an interpreter that cannot import the SSE handler must not be accepted", cmd, detail)
	}
}

func TestResolvePythonForSSE_ErrorsWhenNoInterpreterCanImportTheHandler(t *testing.T) {
	root := fakeRepoWithNewsCreator(t)
	bin := t.TempDir()
	writeFakePython(t, filepath.Join(bin, "python3"), fakePyBare)
	t.Setenv("PATH", bin)

	cmd, detail, err := resolvePythonForSSE(context.Background(), root)
	if err == nil {
		t.Fatalf("resolved %v (%s); a bare python3 is guaranteed to fail the probe and must be an error", cmd, detail)
	}
	for _, want := range []string{"news_creator.handler.sse_stream", "uv sync"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err.Error(), want)
		}
	}
}
