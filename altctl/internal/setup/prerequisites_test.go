package setup

import (
	"testing"
)

func TestCheckPrerequisites_ReturnsResults(t *testing.T) {
	results := CheckPrerequisites()

	if len(results) < 3 {
		t.Fatalf("expected at least 3 check results, got %d", len(results))
	}

	names := make(map[string]bool)
	for _, r := range results {
		names[r.Name] = true
		if r.Name == "" {
			t.Error("check result should have a name")
		}
	}

	expected := []string{"Docker CLI", "Docker Compose", "Docker Daemon"}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("missing prerequisite check: %s", name)
		}
	}
}

func TestCheckResult_Fields(t *testing.T) {
	r := CheckResult{
		Name:    "test",
		OK:      true,
		Version: "1.0",
		Detail:  "all good",
	}

	if r.Name != "test" || !r.OK || r.Version != "1.0" {
		t.Error("CheckResult fields not set correctly")
	}
}
