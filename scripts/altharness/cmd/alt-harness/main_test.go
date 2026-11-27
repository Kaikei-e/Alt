package main

import (
	"alt/scripts/altharness/internal/guards"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunUsageAndUnknown(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run(context.Background(), nil, strings.NewReader(""), &out, &errb); code != 2 {
		t.Fatalf("empty args code=%d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage:") {
		t.Fatalf("missing usage: %s", errb.String())
	}

	out.Reset()
	errb.Reset()
	if code := run(context.Background(), []string{"does-not-exist"}, strings.NewReader(""), &out, &errb); code != 2 {
		t.Fatalf("unknown cmd code=%d, want 2", code)
	}
	if !strings.Contains(errb.String(), "unknown command") {
		t.Fatalf("missing unknown command msg: %s", errb.String())
	}
}

func TestHelpListsAllSubcommands(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run(context.Background(), []string{"--help"}, strings.NewReader(""), &out, &errb); code != 0 {
		t.Fatalf("--help code=%d err=%s", code, errb.String())
	}
	want := []string{
		"lint", "lang", "layers", "host-final", "link-naming",
		"sync-go", "pki", "pyver", "workflow",
		"hygiene", "ratchet", "suppressions", "suppression-diff",
		"generated", "screener", "packet", "corpus", "selfcheck",
		"canary", "entrypoint-canary", "postmortem", "migration-gate", "adr-gate",
		"guard-exec", "guard-scope", "guard-ops", "guard-protected", "guard-event-time", "guard-adr-read",
		"harness-run", "agy-hook", "preflight", "pytest-rand",
		"verify-claims", "verify-diff", "verify-golden", "verify-live", "flaky-hunt",
		"loop-review", "loop-eval", "loop-correct", "gate",
	}
	help := out.String()
	for _, cmd := range want {
		if !strings.Contains(help, cmd) {
			t.Errorf("--help missing subcommand %q in %q", cmd, help)
		}
	}
	if guards.PreflightRunner == nil {
		t.Error("expected guards.PreflightRunner to be wired in-process by main init")
	}
}

func TestWiredSubcommandsHelpAndErrors(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantCode int
		wantOut  string
		wantErr  string
	}{
		{"pki help", []string{"pki", "--help"}, 0, "sync_python_pki.py", ""},
		{"pyver help", []string{"pyver", "--help"}, 0, "check_python_versions.py", ""},
		{"sync-go help", []string{"sync-go", "--help"}, 0, "sync-go", ""},
		{"hygiene help", []string{"hygiene", "--help"}, 0, "hygiene", ""},
		{"entrypoint-canary help", []string{"entrypoint-canary", "--help"}, 0, "entrypoint canary", ""},
		{"pytest-rand help", []string{"pytest-rand", "--help"}, 0, "pytest", ""},
		{"verify-claims help", []string{"verify-claims", "--help"}, 0, "verify-06", ""},
		{"verify-diff help", []string{"verify-diff", "--help"}, 0, "verify-diff", ""},
		{"verify-golden help", []string{"verify-golden", "--help"}, 0, "verify-golden", ""},
		{"verify-live help", []string{"verify-live", "--help"}, 0, "verify-live", ""},
		{"flaky-hunt help", []string{"flaky-hunt", "--help"}, 0, "flaky-hunt", ""},
		{"loop-review help", []string{"loop-review", "--help"}, 0, "loop-04", ""},
		{"loop-eval help", []string{"loop-eval", "--help"}, 0, "loop-05", ""},
		{"loop-correct help", []string{"loop-correct", "--help"}, 0, "loop-07", ""},
		{"lang missing", []string{"lang"}, 2, "", "usage: alt-harness lang"},
		{"lang unknown", []string{"lang", "bogus"}, 2, "", "unknown language runner"},
		{"gate bad arity", []string{"gate"}, 2, "", "usage: alt-harness gate"},
		{"gate unknown", []string{"gate", "bogus"}, 2, "", "unknown gate"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out, errb bytes.Buffer
			code := run(context.Background(), tc.args, strings.NewReader(""), &out, &errb)
			if code != tc.wantCode {
				t.Fatalf("code=%d want=%d stdout=%q stderr=%q", code, tc.wantCode, out.String(), errb.String())
			}
			if tc.wantOut != "" && !strings.Contains(strings.ToLower(out.String()), strings.ToLower(tc.wantOut)) {
				t.Errorf("stdout=%q missing %q", out.String(), tc.wantOut)
			}
			if tc.wantErr != "" && !strings.Contains(errb.String(), tc.wantErr) {
				t.Errorf("stderr=%q missing %q", errb.String(), tc.wantErr)
			}
		})
	}
}

func TestResolveRepoRootFromSubdir(t *testing.T) {
	// Walk up from package dir to repo root.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	wantRoot := filepath.Clean(filepath.Join(wd, "..", "..", "..", ".."))
	subdir := filepath.Join(wantRoot, "alt-backend", "app")
	if _, err := os.Stat(subdir); err != nil {
		t.Skipf("subdir %s absent", subdir)
	}
	t.Chdir(subdir)
	got, err := resolveRepoRoot()
	if err != nil {
		t.Fatalf("resolveRepoRoot failed: %v", err)
	}
	if filepath.Clean(got) != wantRoot {
		t.Fatalf("resolveRepoRoot()=%q, want %q", got, wantRoot)
	}
}
