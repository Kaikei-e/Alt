package compose

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestClient_Logs_MultipleServicesSingleInvocation guards against the
// per-service Logs loop regression: `altctl logs <stack> --follow` used to
// call client.Logs once per service, and since `docker compose logs -f`
// never exits, the loop stuck on the first service forever -- the rest of
// the stack's logs were silently never tailed. Logs must accept the whole
// service list and issue exactly one `docker compose logs` invocation
// (compose accepts multiple service args natively).
func TestClient_Logs_MultipleServicesSingleInvocation(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	projectDir := t.TempDir()
	composeDir := filepath.Join(projectDir, "compose")
	if err := os.MkdirAll(composeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	client := NewClient(projectDir, composeDir, logger, true) // dry-run: no real docker invocation

	err := client.Logs(context.Background(), []string{"svc-a", "svc-b", "svc-c"}, LogsOptions{Tail: 50})
	if err != nil {
		t.Fatalf("Logs failed: %v", err)
	}

	out := buf.String()
	if strings.Count(out, "compose logs") != 1 {
		t.Fatalf("expected exactly one 'docker compose logs' invocation (all services in one call), got log output:\n%s", out)
	}
	for _, svc := range []string{"svc-a", "svc-b", "svc-c"} {
		if !strings.Contains(out, svc) {
			t.Errorf("expected the single invocation to include service %q, got:\n%s", svc, out)
		}
	}
}

func TestBuildFileArgs_WithEnvFile(t *testing.T) {
	projectDir := t.TempDir()
	composeDir := filepath.Join(projectDir, "compose")
	if err := os.MkdirAll(composeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	envFile := filepath.Join(projectDir, ".env")
	if err := os.WriteFile(envFile, []byte("DB_HOST=localhost\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	client := NewClient(projectDir, composeDir, slog.Default(), true)

	files := []string{"base.yaml", "core.yaml"}
	args := client.buildFileArgs(files)

	// --env-file must appear before -f flags
	if len(args) < 2 {
		t.Fatalf("expected at least 2 args for --env-file, got %d: %v", len(args), args)
	}
	if args[0] != "--env-file" {
		t.Errorf("expected first arg to be --env-file, got %q", args[0])
	}
	if args[1] != envFile {
		t.Errorf("expected second arg to be %q, got %q", envFile, args[1])
	}

	// -f flags for each compose file should follow
	expectedFileArgs := []string{
		"-f", filepath.Join(composeDir, "base.yaml"),
		"-f", filepath.Join(composeDir, "core.yaml"),
	}
	remaining := args[2:]
	if len(remaining) != len(expectedFileArgs) {
		t.Fatalf("expected %d file args, got %d: %v", len(expectedFileArgs), len(remaining), remaining)
	}
	for i, want := range expectedFileArgs {
		if remaining[i] != want {
			t.Errorf("arg[%d]: expected %q, got %q", i+2, want, remaining[i])
		}
	}
}

func TestBuildFileArgs_WithoutEnvFile(t *testing.T) {
	projectDir := t.TempDir()
	composeDir := filepath.Join(projectDir, "compose")
	if err := os.MkdirAll(composeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// No .env file created — should fall back to default behavior
	client := NewClient(projectDir, composeDir, slog.Default(), true)

	files := []string{"base.yaml"}
	args := client.buildFileArgs(files)

	// Should only have -f flags, no --env-file
	expected := []string{"-f", filepath.Join(composeDir, "base.yaml")}
	if len(args) != len(expected) {
		t.Fatalf("expected %d args, got %d: %v", len(expected), len(args), args)
	}
	for i, want := range expected {
		if args[i] != want {
			t.Errorf("arg[%d]: expected %q, got %q", i, want, args[i])
		}
	}
}

func TestBuildFileArgs_EmptyFiles(t *testing.T) {
	projectDir := t.TempDir()
	composeDir := filepath.Join(projectDir, "compose")

	envFile := filepath.Join(projectDir, ".env")
	if err := os.WriteFile(envFile, []byte("X=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	client := NewClient(projectDir, composeDir, slog.Default(), true)

	args := client.buildFileArgs(nil)

	// Should still prepend --env-file even with no compose files
	if len(args) != 2 {
		t.Fatalf("expected 2 args (--env-file pair), got %d: %v", len(args), args)
	}
	if args[0] != "--env-file" || args[1] != envFile {
		t.Errorf("expected [--env-file %s], got %v", envFile, args)
	}
}
