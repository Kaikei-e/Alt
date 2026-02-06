package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/alt-project/altctl/internal/config"
)

func setupVersionTest(t *testing.T) {
	t.Helper()
	SetBuildInfo("abc1234", "2026-02-06T07:16:38Z")
	// Initialize minimal config so PersistentPreRunE doesn't fail on version
	cfg = &config.Config{
		Output: config.OutputConfig{Colors: false},
		Logging: config.LoggingConfig{
			Level:  "info",
			Format: "text",
		},
	}
	// Reset flags between test runs to avoid state leaking
	versionCmd.Flags().Set("short", "false")
	versionCmd.Flags().Set("json", "false")
}

func TestVersionOutput_ContainsFields(t *testing.T) {
	setupVersionTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"version"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("version command failed: %v", err)
	}

	out := buf.String()
	for _, field := range []string{"commit:", "built:", "go version:", "platform:"} {
		if !strings.Contains(out, field) {
			t.Errorf("version output missing %q field. Got:\n%s", field, out)
		}
	}
}

func TestVersionShort(t *testing.T) {
	setupVersionTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"version", "--short"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("version --short failed: %v", err)
	}

	out := buf.String()
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 1 {
		t.Errorf("expected 1 line, got %d: %q", len(lines), out)
	}
}

func TestVersionJSON(t *testing.T) {
	setupVersionTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"version", "--json"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("version --json failed: %v", err)
	}

	var result map[string]string
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\nGot: %s", err, buf.String())
	}

	for _, key := range []string{"version", "commit", "built", "goVersion", "platform"} {
		if _, ok := result[key]; !ok {
			t.Errorf("JSON output missing key %q. Got: %v", key, result)
		}
	}
}
