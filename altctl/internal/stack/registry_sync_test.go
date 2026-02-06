package stack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// composeDir returns the path to the compose/ directory relative to the project root.
// It identifies the project root by looking for a directory that has both "compose/" and "altctl/".
func composeDir(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getting working directory: %v", err)
	}
	for {
		candidate := filepath.Join(dir, "compose")
		// The project root's compose/ dir contains YAML files (not Go files)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			// Verify this is the project compose dir (has .yaml files and altctl/ sibling)
			if _, err := os.Stat(filepath.Join(dir, "altctl")); err == nil {
				return candidate
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Skip("compose/ directory not found; skipping sync test")
		}
		dir = parent
	}
}

// parseComposeServices extracts service names from a compose YAML file.
// It looks for top-level "services:" key and collects indented service names.
func parseComposeServices(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}

	var services []string
	inServices := false
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Top-level key detection (no leading whitespace)
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			if strings.HasPrefix(trimmed, "services:") {
				inServices = true
				continue
			}
			// Any other top-level key exits the services block
			if inServices {
				inServices = false
			}
			continue
		}

		if !inServices {
			continue
		}

		// Two-space or one-tab indented key is a service name
		if (strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ")) ||
			(strings.HasPrefix(line, "\t") && !strings.HasPrefix(line, "\t\t")) {
			name := strings.TrimSpace(strings.TrimSuffix(trimmed, ":"))
			if strings.HasSuffix(trimmed, ":") {
				services = append(services, name)
			}
		}
	}
	return services
}

func TestEveryStackComposeFileExists(t *testing.T) {
	dir := composeDir(t)
	registry := NewRegistry()

	for _, s := range registry.All() {
		if s.ComposeFile == "" {
			continue
		}
		path := filepath.Join(dir, s.ComposeFile)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("stack %q references compose file %q which does not exist at %s", s.Name, s.ComposeFile, path)
		}
	}
}

func TestRegistryServicesMatchCompose(t *testing.T) {
	dir := composeDir(t)
	registry := NewRegistry()

	for _, s := range registry.All() {
		if s.ComposeFile == "" || len(s.Services) == 0 {
			continue
		}
		path := filepath.Join(dir, s.ComposeFile)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue // Covered by TestEveryStackComposeFileExists
		}

		composeServices := parseComposeServices(t, path)
		composeSet := make(map[string]bool, len(composeServices))
		for _, svc := range composeServices {
			composeSet[svc] = true
		}

		for _, svc := range s.Services {
			if !composeSet[svc] {
				t.Errorf("stack %q lists service %q which is not defined in %s (found: %v)",
					s.Name, svc, s.ComposeFile, composeServices)
			}
		}
	}
}

func TestNoOrphanComposeFiles(t *testing.T) {
	dir := composeDir(t)
	registry := NewRegistry()

	// Build set of all referenced compose files
	referenced := make(map[string]bool)
	for _, s := range registry.All() {
		if s.ComposeFile != "" {
			referenced[s.ComposeFile] = true
		}
	}

	// compose.yaml is the aggregate file, not a stack-specific file
	referenced["compose.yaml"] = true

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading compose dir: %v", err)
	}

	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		if !referenced[name] {
			t.Errorf("compose file %q has no corresponding stack in registry", name)
		}
	}
}
