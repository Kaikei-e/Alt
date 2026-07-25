package cmd

import (
	"bytes"
	"testing"
)

func setupRestartTest(t *testing.T) {
	t.Helper()
	cfg = testConfig(t, []string{"db", "auth", "core", "workers"})
	dryRun = true
	quiet = false
	restartCmd.Flags().Set("build", "false")
}

func TestRestart_DefaultStacks(t *testing.T) {
	setupRestartTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"restart", "--dry-run"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("restart command failed: %v", err)
	}
}

func TestRestart_SpecificStack(t *testing.T) {
	setupRestartTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"restart", "recap", "--dry-run"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("restart recap failed: %v", err)
	}
}

func TestRestart_UnknownStack(t *testing.T) {
	setupRestartTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"restart", "nonexistent", "--dry-run"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for unknown stack, got nil")
	}
}

// TestRestart_DryRun_SkipsReadyWait guards against restart hanging in
// --dry-run: since dry-run never actually starts containers, waiting for
// `docker compose ps` to report them Ready would either report everything
// as permanently "missing" or (worse) block for the full Ready-wait
// timeout. restart must short-circuit exactly like `up` does.
func TestRestart_DryRun_SkipsReadyWait(t *testing.T) {
	setupRestartTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"restart", "core", "--dry-run"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("restart core --dry-run failed: %v", err)
	}
}
