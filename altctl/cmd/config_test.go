package cmd

import (
	"bytes"
	"testing"
)

func setupConfigTest(t *testing.T) {
	t.Helper()
	cfg = testConfig(t, []string{"db", "auth", "core", "workers"})
	dryRun = false
	quiet = false
}

func TestConfig_Default(t *testing.T) {
	setupConfigTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"config"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("config command failed: %v", err)
	}
}

func TestConfig_JSON(t *testing.T) {
	setupConfigTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"config", "--json"})

	// config --json writes to os.Stdout directly; verify no error
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("config --json failed: %v", err)
	}
}

func TestConfig_Path(t *testing.T) {
	setupConfigTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"config", "--path"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("config --path failed: %v", err)
	}
}
