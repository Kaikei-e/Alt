package setup

import (
	"testing"
)

func TestDefaultMigrationDirs_HasAllDirs(t *testing.T) {
	dirs := DefaultMigrationDirs()

	if len(dirs) != 3 {
		t.Fatalf("expected 3 migration dirs, got %d", len(dirs))
	}

	names := make(map[string]bool)
	for _, d := range dirs {
		names[d.Name] = true
		if d.Path == "" {
			t.Errorf("migration dir %s has empty path", d.Name)
		}
	}

	expected := []string{"main DB", "recap DB", "RAG DB"}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("missing migration dir: %s", name)
		}
	}
}
