package cmd

import (
	"os"
	"path/filepath"
	"testing"

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
