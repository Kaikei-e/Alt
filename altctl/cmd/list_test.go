package cmd

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/alt-project/altctl/internal/config"
	"github.com/alt-project/altctl/internal/stack"
)

func setupListTest(t *testing.T) {
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
	listCmd.Flags().Set("services", "false")
	listCmd.Flags().Set("deps", "false")
	listCmd.Flags().Set("json", "false")
}

func TestList_Default(t *testing.T) {
	setupListTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"list"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("list command failed: %v", err)
	}
}

func TestList_Services(t *testing.T) {
	setupListTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"list", "--services"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("list --services failed: %v", err)
	}
}

func TestList_Deps(t *testing.T) {
	setupListTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"list", "--deps"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("list --deps failed: %v", err)
	}
}

func TestList_JSON(t *testing.T) {
	setupListTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"list", "--json"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("list --json failed: %v", err)
	}
}

func TestList_RegistryContainsAllStacks(t *testing.T) {
	registry := stack.NewRegistry()
	stacks := registry.All()

	data, err := json.Marshal(stacks)
	if err != nil {
		t.Fatalf("failed to marshal stacks: %v", err)
	}

	var result []map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	expectedStacks := map[string]bool{
		"base": false, "db": false, "auth": false, "core": false,
		"ai": false, "workers": false, "recap": false, "logging": false,
		"rag": false, "observability": false, "mq": false, "bff": false,
		"perf": false, "dev": false, "frontend-dev": false, "backup": false,
	}

	for _, item := range result {
		name := item["name"].(string)
		expectedStacks[name] = true
	}

	for name, found := range expectedStacks {
		if !found {
			t.Errorf("expected stack %q to be in registry", name)
		}
	}
}
