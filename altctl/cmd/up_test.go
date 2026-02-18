package cmd

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/alt-project/altctl/internal/compose"
	"github.com/alt-project/altctl/internal/config"
	"github.com/alt-project/altctl/internal/output"
	"github.com/alt-project/altctl/internal/stack"
)

func setupUpTest(t *testing.T) {
	t.Helper()
	cfg = &config.Config{
		Output:   config.OutputConfig{Colors: false},
		Logging:  config.LoggingConfig{Level: "info", Format: "text"},
		Defaults: config.DefaultsConfig{Stacks: []string{"db", "auth", "core", "workers"}},
		Project:  config.ProjectConfig{Root: t.TempDir()},
		Compose:  config.ComposeConfig{Dir: "compose"},
	}
	dryRun = true
	quiet = false
	// Reset flags that persist between test runs
	upCmd.Flags().Set("all", "false")
	upCmd.Flags().Set("no-deps", "false")
	upCmd.Flags().Set("build", "false")
	upCmd.Flags().Set("remove-orphans", "false")
	upCmd.Flags().Set("progress", "auto")
}

func TestUp_DefaultStacks(t *testing.T) {
	setupUpTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"up", "--dry-run"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("up command failed: %v", err)
	}
}

func TestUp_SpecificStack(t *testing.T) {
	setupUpTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"up", "recap", "--dry-run"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("up recap failed: %v", err)
	}
}

func TestUp_UnknownStack(t *testing.T) {
	setupUpTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"up", "nonexistent", "--dry-run"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for unknown stack, got nil")
	}
}

func TestUp_NoDeps(t *testing.T) {
	setupUpTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"up", "core", "--no-deps", "--dry-run"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("up --no-deps failed: %v", err)
	}
}

func TestUp_All(t *testing.T) {
	setupUpTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"up", "--all", "--dry-run"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("up --all failed: %v", err)
	}
}

func TestUp_UnknownStack_NoDeps(t *testing.T) {
	setupUpTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"up", "nonexistent", "--no-deps", "--dry-run"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for unknown stack with --no-deps, got nil")
	}
}

func TestUp_DryRunDoesNotFail(t *testing.T) {
	setupUpTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"up", "ai", "--dry-run"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("up ai --dry-run failed: %v", err)
	}
}

// --- classifyServices / buildPartialStartupError tests ---

func workersStack() *stack.Stack {
	return &stack.Stack{
		Name:     "workers",
		Services: []string{"auth-token-manager", "search-indexer", "tag-generator"},
	}
}

func TestDiagnosePartialStartup_SomeServicesMissing(t *testing.T) {
	stacks := []*stack.Stack{workersStack()}
	statuses := []compose.ServiceStatus{
		{Name: "tag-generator", State: "running"},
		{Name: "auth-token-manager", State: "running"},
	}

	diag := classifyServices(stacks, statuses)

	if !slices.Equal(diag.running, []string{"auth-token-manager", "tag-generator"}) {
		t.Errorf("running: got %v, want [auth-token-manager tag-generator]", diag.running)
	}
	if !slices.Equal(diag.missing, []string{"search-indexer"}) {
		t.Errorf("missing: got %v, want [search-indexer]", diag.missing)
	}
	if len(diag.unhealthy) != 0 {
		t.Errorf("unhealthy: got %v, want []", diag.unhealthy)
	}

	cliErr := buildPartialStartupError(diag, fmt.Errorf("exit status 1"))
	if !strings.Contains(cliErr.Summary, "2 of 3") {
		t.Errorf("summary %q should contain '2 of 3'", cliErr.Summary)
	}
	if cliErr.ExitCode != output.ExitComposeError {
		t.Errorf("exit code: got %d, want %d", cliErr.ExitCode, output.ExitComposeError)
	}
	if !strings.Contains(cliErr.Suggestion, "--build") {
		t.Errorf("suggestion %q should contain '--build'", cliErr.Suggestion)
	}
	if !strings.Contains(cliErr.Suggestion, "workers") {
		t.Errorf("suggestion %q should contain 'workers'", cliErr.Suggestion)
	}
}

func TestDiagnosePartialStartup_AllServicesRunning(t *testing.T) {
	stacks := []*stack.Stack{workersStack()}
	statuses := []compose.ServiceStatus{
		{Name: "auth-token-manager", State: "running"},
		{Name: "search-indexer", State: "running"},
		{Name: "tag-generator", State: "running"},
	}

	diag := classifyServices(stacks, statuses)

	if len(diag.running) != 3 {
		t.Errorf("running: got %d, want 3", len(diag.running))
	}
	if len(diag.missing) != 0 {
		t.Errorf("missing: got %v, want []", diag.missing)
	}

	cliErr := buildPartialStartupError(diag, fmt.Errorf("exit status 1"))
	if !strings.Contains(cliErr.Summary, "3 of 3") {
		t.Errorf("summary %q should contain '3 of 3'", cliErr.Summary)
	}
	// No --build suggestion when nothing is missing
	if strings.Contains(cliErr.Suggestion, "--build") {
		t.Errorf("suggestion %q should not contain '--build' when all running", cliErr.Suggestion)
	}
}

func TestDiagnosePartialStartup_NoServicesRunning(t *testing.T) {
	stacks := []*stack.Stack{workersStack()}
	var statuses []compose.ServiceStatus

	diag := classifyServices(stacks, statuses)

	if len(diag.running) != 0 {
		t.Errorf("running: got %d, want 0", len(diag.running))
	}
	if len(diag.missing) != 3 {
		t.Errorf("missing: got %d, want 3", len(diag.missing))
	}

	cliErr := buildPartialStartupError(diag, fmt.Errorf("exit status 1"))
	if !strings.Contains(cliErr.Summary, "0 of 3") {
		t.Errorf("summary %q should contain '0 of 3'", cliErr.Summary)
	}
}

func TestDiagnosePartialStartup_UnhealthyService(t *testing.T) {
	stacks := []*stack.Stack{workersStack()}
	statuses := []compose.ServiceStatus{
		{Name: "auth-token-manager", State: "running"},
		{Name: "search-indexer", State: "running", Health: "unhealthy"},
		{Name: "tag-generator", State: "running"},
	}

	diag := classifyServices(stacks, statuses)

	if !slices.Equal(diag.unhealthy, []string{"search-indexer"}) {
		t.Errorf("unhealthy: got %v, want [search-indexer]", diag.unhealthy)
	}
	if !slices.Equal(diag.running, []string{"auth-token-manager", "tag-generator"}) {
		t.Errorf("running: got %v, want [auth-token-manager tag-generator]", diag.running)
	}
	if len(diag.missing) != 0 {
		t.Errorf("missing: got %v, want []", diag.missing)
	}
}

func TestDiagnosePartialStartup_EmptyStacks(t *testing.T) {
	stacks := []*stack.Stack{
		{Name: "base", Services: []string{}},
	}
	var statuses []compose.ServiceStatus

	diag := classifyServices(stacks, statuses)

	if len(diag.expected) != 0 {
		t.Errorf("expected: got %d, want 0", len(diag.expected))
	}
	if len(diag.running) != 0 {
		t.Errorf("running: got %d, want 0", len(diag.running))
	}
	if len(diag.missing) != 0 {
		t.Errorf("missing: got %d, want 0", len(diag.missing))
	}

	cliErr := buildPartialStartupError(diag, fmt.Errorf("exit status 1"))
	if cliErr != nil {
		t.Errorf("expected nil CLIError for empty stacks, got %v", cliErr)
	}
}
