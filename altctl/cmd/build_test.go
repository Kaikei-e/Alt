package cmd

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func setupBuildTest(t *testing.T) {
	t.Helper()
	cfg = testConfig(t, []string{"db", "auth", "core", "workers"})
	dryRun = true
	quiet = false
	require.NoError(t, buildCmd.Flags().Set("no-deps", "false"))
	require.NoError(t, buildCmd.Flags().Set("no-cache", "false"))
	require.NoError(t, buildCmd.Flags().Set("pull", "false"))
	require.NoError(t, buildCmd.Flags().Set("progress", "auto"))
}

func TestBuild_DefaultStacks(t *testing.T) {
	setupBuildTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"build", "--dry-run"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("build command failed: %v", err)
	}
}

func TestBuild_SpecificStack(t *testing.T) {
	setupBuildTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"build", "core", "--dry-run"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("build core failed: %v", err)
	}
}

func TestBuild_UnknownStack(t *testing.T) {
	setupBuildTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"build", "nonexistent", "--dry-run"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for unknown stack, got nil")
	}
}

func TestBuild_NoDeps(t *testing.T) {
	setupBuildTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"build", "core", "--no-deps", "--dry-run"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("build --no-deps failed: %v", err)
	}
}
