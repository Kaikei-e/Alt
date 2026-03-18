package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/alt-project/altctl/internal/config"
)

func setupHomeTest(t *testing.T) {
	t.Helper()
	cfg = &config.Config{
		Output:   config.OutputConfig{Colors: false},
		Logging:  config.LoggingConfig{Level: "info", Format: "text"},
		Defaults: config.DefaultsConfig{Stacks: []string{"db", "auth", "core", "workers"}},
		Project:  config.ProjectConfig{Root: t.TempDir()},
		Compose:  config.ComposeConfig{Dir: "compose"},
	}
	dryRun = false
	quiet = false
}

func TestHomeCmd_Exists(t *testing.T) {
	setupHomeTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"home", "--help"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("home --help failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Knowledge Home") {
		t.Errorf("expected help to mention 'Knowledge Home', got:\n%s", out)
	}
}

func TestHomeReprojectCmd_Exists(t *testing.T) {
	setupHomeTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"home", "reproject", "--help"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("home reproject --help failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "reproject") {
		t.Errorf("expected help to mention 'reproject', got:\n%s", out)
	}
}

func TestHomeReprojectStart_HasSubcommands(t *testing.T) {
	setupHomeTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"home", "reproject", "--help"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("home reproject --help failed: %v", err)
	}

	out := buf.String()
	for _, sub := range []string{"start", "status", "compare", "swap", "rollback"} {
		if !strings.Contains(out, sub) {
			t.Errorf("expected reproject help to list %q subcommand, got:\n%s", sub, out)
		}
	}
}

func TestHomeReprojectStart_RequiresFlags(t *testing.T) {
	setupHomeTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"home", "reproject", "start"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error when --mode is missing, got nil")
	}
	if !strings.Contains(err.Error(), "mode") {
		t.Errorf("expected error to mention 'mode', got: %v", err)
	}
}

func TestHomeReprojectStart_InvalidMode(t *testing.T) {
	setupHomeTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{
		"home", "reproject", "start",
		"--mode=invalid",
		"--from=1", "--to=2",
	})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid mode, got nil")
	}
	if !strings.Contains(err.Error(), "mode") {
		t.Errorf("expected error to mention 'mode', got: %v", err)
	}
}

func TestHomeReprojectStart_ValidModes(t *testing.T) {
	for _, mode := range []string{"dry_run", "shadow", "live"} {
		t.Run(mode, func(t *testing.T) {
			// Just validate the mode is accepted (will fail on connection which is expected)
			err := validateReprojectMode(mode)
			if err != nil {
				t.Errorf("expected mode %q to be valid, got error: %v", mode, err)
			}
		})
	}
}

func TestHomeReprojectStatus_RequiresRunID(t *testing.T) {
	setupHomeTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"home", "reproject", "status"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error when --run-id is missing, got nil")
	}
	if !strings.Contains(err.Error(), "run-id") {
		t.Errorf("expected error to mention 'run-id', got: %v", err)
	}
}

func TestHomeReprojectCompare_RequiresRunID(t *testing.T) {
	setupHomeTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"home", "reproject", "compare"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error when --run-id is missing, got nil")
	}
}

func TestHomeReprojectSwap_RequiresRunID(t *testing.T) {
	setupHomeTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"home", "reproject", "swap"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error when --run-id is missing, got nil")
	}
}

func TestHomeReprojectRollback_RequiresRunID(t *testing.T) {
	setupHomeTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"home", "reproject", "rollback"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error when --run-id is missing, got nil")
	}
}

func TestValidateUUID(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"550e8400-e29b-41d4-a716-446655440000", true},
		{"not-a-uuid", false},
		{"", false},
		{"550e8400e29b41d4a716446655440000", false}, // no hyphens
		{"550e8400-e29b-41d4-a716-44665544000", false}, // too short
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			err := validateUUID(tt.input)
			if tt.valid && err != nil {
				t.Errorf("expected %q to be valid, got error: %v", tt.input, err)
			}
			if !tt.valid && err == nil {
				t.Errorf("expected %q to be invalid, got nil", tt.input)
			}
		})
	}
}
