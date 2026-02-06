package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alt-project/altctl/internal/config"
)

func setupDocsTest(t *testing.T) {
	t.Helper()
	cfg = &config.Config{
		Output: config.OutputConfig{Colors: false},
		Logging: config.LoggingConfig{
			Level:  "info",
			Format: "text",
		},
	}
}

func TestDocsMan(t *testing.T) {
	setupDocsTest(t)

	tmpDir := t.TempDir()
	rootCmd.SetArgs([]string{"docs", "--format", "man", "--output", tmpDir})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("docs --format man failed: %v", err)
	}

	// Check that at least one man page was generated
	matches, err := filepath.Glob(filepath.Join(tmpDir, "*.1"))
	if err != nil {
		t.Fatalf("glob failed: %v", err)
	}
	if len(matches) == 0 {
		t.Error("no man pages generated")
	}
}

func TestDocsMarkdown(t *testing.T) {
	setupDocsTest(t)

	tmpDir := t.TempDir()
	rootCmd.SetArgs([]string{"docs", "--format", "markdown", "--output", tmpDir})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("docs --format markdown failed: %v", err)
	}

	// Check that at least one markdown file was generated
	matches, err := filepath.Glob(filepath.Join(tmpDir, "*.md"))
	if err != nil {
		t.Fatalf("glob failed: %v", err)
	}
	if len(matches) == 0 {
		entries, _ := os.ReadDir(tmpDir)
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("no markdown files generated. Files in dir: %v", names)
	}
}
