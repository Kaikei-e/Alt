package doctor

import (
	"os"
	"testing"
)

// TestEnsureDockerGroupIDEnv_InjectsWhenUnset is the H1 regression test at
// the shared-helper level: with DOCKER_GROUP_ID unset, the helper must set
// a placeholder so a read-only aggregate compose call (status/doctor)
// doesn't hard-fail at config-interpolation time on compose/logging.yaml's
// `${DOCKER_GROUP_ID:?...}`.
func TestEnsureDockerGroupIDEnv_InjectsWhenUnset(t *testing.T) {
	t.Setenv("DOCKER_GROUP_ID", "")

	restore := EnsureDockerGroupIDEnv()
	defer restore()

	if os.Getenv("DOCKER_GROUP_ID") == "" {
		t.Fatal("expected EnsureDockerGroupIDEnv to inject a non-empty placeholder")
	}
}

// TestEnsureDockerGroupIDEnv_RestoreUnsetsPlaceholder ensures the injected
// placeholder doesn't leak past the caller's scope -- a real docker.sock
// group ID must never be silently masked by a stale "0" left behind from an
// earlier read-only call.
func TestEnsureDockerGroupIDEnv_RestoreUnsetsPlaceholder(t *testing.T) {
	t.Setenv("DOCKER_GROUP_ID", "")

	restore := EnsureDockerGroupIDEnv()
	if os.Getenv("DOCKER_GROUP_ID") == "" {
		t.Fatal("expected placeholder to be set before restore")
	}
	restore()

	if os.Getenv("DOCKER_GROUP_ID") != "" {
		t.Errorf("expected DOCKER_GROUP_ID to be unset again after restore, got %q", os.Getenv("DOCKER_GROUP_ID"))
	}
}

// TestEnsureDockerGroupIDEnv_LeavesRealValueUntouched guards against the
// helper clobbering a genuinely configured DOCKER_GROUP_ID -- it must only
// ever fill a gap, never override an operator's real setting, and restore
// must be a no-op in that case.
func TestEnsureDockerGroupIDEnv_LeavesRealValueUntouched(t *testing.T) {
	t.Setenv("DOCKER_GROUP_ID", "999")

	restore := EnsureDockerGroupIDEnv()
	if got := os.Getenv("DOCKER_GROUP_ID"); got != "999" {
		t.Fatalf("expected the real DOCKER_GROUP_ID to be left untouched, got %q", got)
	}
	restore()

	if got := os.Getenv("DOCKER_GROUP_ID"); got != "999" {
		t.Errorf("expected DOCKER_GROUP_ID to still be %q after restore, got %q", "999", got)
	}
}
