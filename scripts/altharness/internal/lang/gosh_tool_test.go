package lang

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"alt/scripts/altharness/internal/fsperm"
)

func TestEnsureGoTool_ReusesMatchingPathVersionOffline(t *testing.T) {
	binDir := t.TempDir()
	cacheDir := t.TempDir()
	stubPath := filepath.Join(binDir, "golangci-lint")
	stub := "#!/bin/sh\necho 'golangci-lint has version 2.14.0 built with go1.27.3 from (unknown)'\n"
	if err := fsperm.WriteExecutable(stubPath, []byte(stub)); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GOPROXY", "off")

	var stderr bytes.Buffer
	got, rc := ensureGoTool(context.Background(), &stderr, t.TempDir(), cacheDir, "golangci-lint", golangciModule, golangciLintVersion)
	if rc != 0 {
		t.Fatalf("expected pinned PATH binary reuse offline, got rc=%d stderr=%q", rc, stderr.String())
	}
	if got != stubPath {
		t.Fatalf("got bin=%q, want %q", got, stubPath)
	}
}

func TestEnsureGoTool_RejectsMismatchedPathVersionOffline(t *testing.T) {
	binDir := t.TempDir()
	cacheDir := t.TempDir()
	stubPath := filepath.Join(binDir, "golangci-lint")
	stub := "#!/bin/sh\necho 'golangci-lint has version 1.64.0 built with go1.24.0'\n"
	if err := fsperm.WriteExecutable(stubPath, []byte(stub)); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GOPROXY", "off")
	t.Setenv("GOMODCACHE", t.TempDir())

	var stderr bytes.Buffer
	got, rc := ensureGoTool(context.Background(), &stderr, t.TempDir(), cacheDir, "golangci-lint", golangciModule, golangciLintVersion)
	if rc == 0 || got == stubPath {
		t.Fatalf("mismatched version must NOT be silently reused: got=%q rc=%d", got, rc)
	}
}
