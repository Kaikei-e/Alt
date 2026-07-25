package cmd

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alt-project/altctl/internal/compose"
	"github.com/alt-project/altctl/internal/config"
)

// repoRootForTest locates the Alt monorepo root (the directory containing
// both compose/ and altctl/) by walking up from the test's working
// directory. Several cmd package tests exercise the CLI end-to-end in
// --dry-run mode, including stack dependency resolution -- since the stack
// registry now derives stacks from compose/*.yaml + .altctl.yaml on disk
// (internal/stack.NewRegistry) rather than a hardcoded list, those tests
// need the real project tree, not an empty t.TempDir().
func repoRootForTest(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getting working directory: %v", err)
	}
	for {
		if info, statErr := os.Stat(filepath.Join(dir, "compose")); statErr == nil && info.IsDir() {
			if _, altErr := os.Stat(filepath.Join(dir, "altctl")); altErr == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Skip("Alt project root (compose/ + altctl/) not found; skipping")
		}
		dir = parent
	}
}

// testConfig returns a *config.Config wired against the real repo's
// compose/*.yaml and .altctl.yaml, for cmd tests that exercise stack
// resolution via --dry-run.
func testConfig(t *testing.T, defaultStacks []string) *config.Config {
	t.Helper()
	root := repoRootForTest(t)
	return &config.Config{
		Output:         config.OutputConfig{Colors: false},
		Logging:        config.LoggingConfig{Level: "info", Format: "text"},
		Defaults:       config.DefaultsConfig{Stacks: defaultStacks},
		Project:        config.ProjectConfig{Root: root},
		Compose:        config.ComposeConfig{Dir: "compose"},
		ConfigFilePath: filepath.Join(root, ".altctl.yaml"),
	}
}

// fakeComposeExecutor implements compose.Executor by recording every
// invocation's argv instead of shelling out. Used with
// compose.NewClientWithExecutor (via installFakeComposeClient below) so cmd
// tests can assert on the EXACT composed argv -- file list, --profile,
// service names, --no-deps/--force-recreate -- instead of only being able
// to check that a command didn't error out. This is the M2 test-honesty
// fix: assertions of this shape are what would have caught C3 (per-stack -f
// subsets rejected by real docker compose), C4 (FindByService
// nondeterminism), and H2 (exec/logs never passing -f at all) instead of
// the previous tests, which only ever checked "did rootCmd.Execute() return
// an error" under --dry-run/--dry-run's log-text path.
type fakeComposeExecutor struct {
	calls [][]string
}

func (f *fakeComposeExecutor) Run(ctx context.Context, cmd string, args []string) error {
	f.calls = append(f.calls, append([]string{cmd}, args...))
	return nil
}

func (f *fakeComposeExecutor) RunWithOutput(ctx context.Context, cmd string, args []string) ([]byte, error) {
	f.calls = append(f.calls, append([]string{cmd}, args...))
	return nil, nil
}

func (f *fakeComposeExecutor) RunWithPipes(ctx context.Context, cmd string, args []string, stdout, stderr io.Writer) error {
	f.calls = append(f.calls, append([]string{cmd}, args...))
	return nil
}

// argvs returns every recorded call's argv joined into one space-separated
// string per call, in call order -- convenient for substring assertions
// against a specific invocation (e.g. the "up" call vs. a preceding "build"
// call).
func (f *fakeComposeExecutor) argvs() []string {
	out := make([]string, len(f.calls))
	for i, c := range f.calls {
		out[i] = strings.Join(c, " ")
	}
	return out
}

// findArgv returns the first recorded call containing every want substring,
// or "" (and false) if none matches -- used to pick out e.g. "the up call"
// among a build+up sequence without depending on call order/count.
func (f *fakeComposeExecutor) findArgv(want ...string) (string, bool) {
	for _, argv := range f.argvs() {
		ok := true
		for _, w := range want {
			if !strings.Contains(argv, w) {
				ok = false
				break
			}
		}
		if ok {
			return argv, true
		}
	}
	return "", false
}

// installFakeComposeClient points newComposeClient at a Client backed by a
// fakeComposeExecutor instead of a real (or dry-run) executor, for the
// duration of the calling test, restoring the default afterward via
// t.Cleanup. Unlike --dry-run (which never calls compose.NewClientWithExecutor
// at all and only logs a human-readable string), this captures the literal
// argv every internal/compose.Client method builds.
func installFakeComposeClient(t *testing.T) *fakeComposeExecutor {
	t.Helper()
	fake := &fakeComposeExecutor{}
	newComposeClient = func() *compose.Client {
		return compose.NewClientWithExecutor(fake, getProjectRoot(), getComposeDir(), logger)
	}
	t.Cleanup(func() {
		newComposeClient = defaultComposeClient
	})
	return fake
}
