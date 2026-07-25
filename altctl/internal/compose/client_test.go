package compose

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeExecutor implements Executor by recording every invocation's argv
// instead of shelling out, so cmd/internal/compose tests can assert on the
// exact composed argv (file list, --profile, service names,
// --no-deps/--force-recreate, ...) instead of parsing dry-run log text.
type fakeExecutor struct {
	calls [][]string
}

func (f *fakeExecutor) Run(ctx context.Context, cmd string, args []string) error {
	f.calls = append(f.calls, append([]string{cmd}, args...))
	return nil
}

func (f *fakeExecutor) RunWithOutput(ctx context.Context, cmd string, args []string) ([]byte, error) {
	f.calls = append(f.calls, append([]string{cmd}, args...))
	return nil, nil
}

func (f *fakeExecutor) RunWithPipes(ctx context.Context, cmd string, args []string, stdout, stderr io.Writer) error {
	f.calls = append(f.calls, append([]string{cmd}, args...))
	return nil
}

func (f *fakeExecutor) lastCall() []string {
	if len(f.calls) == 0 {
		return nil
	}
	return f.calls[len(f.calls)-1]
}

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

	err := client.Logs(context.Background(), []string{"core.yaml"}, []string{"svc-a", "svc-b", "svc-c"}, LogsOptions{Tail: 50})
	if err != nil {
		t.Fatalf("Logs failed: %v", err)
	}

	out := buf.String()
	if strings.Count(out, "[dry-run] docker compose") != 1 {
		t.Fatalf("expected exactly one 'docker compose ... logs' invocation (all services in one call), got log output:\n%s", out)
	}
	if !strings.Contains(out, " logs ") && !strings.HasSuffix(strings.TrimRight(out, "\n"), " logs") {
		t.Fatalf("expected the invocation to include the 'logs' subcommand, got:\n%s", out)
	}
	for _, svc := range []string{"svc-a", "svc-b", "svc-c"} {
		if !strings.Contains(out, svc) {
			t.Errorf("expected the single invocation to include service %q, got:\n%s", svc, out)
		}
	}
	// H2 fix: Logs used to be called with no -f at all, which dies with
	// "no configuration file provided" the moment dry-run is off.
	if !strings.Contains(out, "-f") || !strings.Contains(out, "core.yaml") {
		t.Errorf("expected the invocation to include '-f .../core.yaml', got:\n%s", out)
	}
}

// TestClient_Logs_NoFilesStillOmitsDashF guards that an empty files list
// (the legacy, broken H2 call shape) doesn't silently succeed in a way that
// would mask a caller forgetting to resolve files -- Logs must not inject a
// default itself; callers (cmd/logs.go) are responsible for resolving a
// non-empty file list via buildStackInvocation.
func TestClient_Logs_NoFilesStillOmitsDashF(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	projectDir := t.TempDir()
	composeDir := filepath.Join(projectDir, "compose")
	if err := os.MkdirAll(composeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	client := NewClient(projectDir, composeDir, logger, true)

	if err := client.Logs(context.Background(), nil, []string{"svc-a"}, LogsOptions{}); err != nil {
		t.Fatalf("Logs failed: %v", err)
	}

	out := buf.String()
	if strings.Contains(out, "-f") {
		t.Errorf("expected no -f flag when files is nil, got:\n%s", out)
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

// --- NewClientWithExecutor / argv capture --------------------------------

func TestClient_Up_ComposesProfileAndServiceArgs(t *testing.T) {
	fake := &fakeExecutor{}
	client := NewClientWithExecutor(fake, "/proj", "/proj/compose", nil)

	err := client.Up(context.Background(), UpOptions{
		Files:    []string{"compose.yaml"},
		Services: []string{"alt-perf", "k6"},
		Profiles: []string{"perf"},
		Detach:   true,
	})
	if err != nil {
		t.Fatalf("Up failed: %v", err)
	}

	got := fake.lastCall()
	joined := strings.Join(got, " ")
	for _, want := range []string{"-f " + filepath.Join("/proj/compose", "compose.yaml"), "--profile perf", "up", "-d", "alt-perf", "k6"} {
		if !strings.Contains(joined, want) {
			t.Errorf("Up argv %q missing %q", joined, want)
		}
	}
	// --profile / -f must come before the "up" subcommand.
	profileIdx := indexOfArg(got, "--profile")
	upIdx := indexOfArg(got, "up")
	if profileIdx == -1 || upIdx == -1 || profileIdx > upIdx {
		t.Errorf("expected --profile before 'up' subcommand, got argv %v", got)
	}
}

func TestClient_Rebuild_ComposesNoDepsForceRecreate(t *testing.T) {
	fake := &fakeExecutor{}
	client := NewClientWithExecutor(fake, "/proj", "/proj/compose", nil)

	err := client.Up(context.Background(), UpOptions{
		Files:         []string{"core.yaml"},
		Services:      []string{"alt-backend"},
		Detach:        true,
		NoDeps:        true,
		ForceRecreate: true,
	})
	if err != nil {
		t.Fatalf("Up failed: %v", err)
	}

	joined := strings.Join(fake.lastCall(), " ")
	for _, want := range []string{"--no-deps", "--force-recreate", "alt-backend"} {
		if !strings.Contains(joined, want) {
			t.Errorf("rebuild-style Up argv %q missing %q", joined, want)
		}
	}
}

func TestClient_Stop_ScopesToNamedServices(t *testing.T) {
	fake := &fakeExecutor{}
	client := NewClientWithExecutor(fake, "/proj", "/proj/compose", nil)

	if err := client.Stop(context.Background(), StopOptions{
		Files:    []string{"compose.yaml"},
		Services: []string{"alt-backend", "alt-frontend-sv"},
	}); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	got := fake.lastCall()
	joined := strings.Join(got, " ")
	if !strings.Contains(joined, "stop") {
		t.Errorf("expected 'stop' in argv, got %v", got)
	}
	if !strings.Contains(joined, "alt-backend alt-frontend-sv") {
		t.Errorf("expected named services at the tail of argv, got %v", got)
	}
	if strings.Contains(joined, "down") {
		t.Errorf("Stop must never invoke 'down', got %v", got)
	}
}

func TestClient_Remove_PassesForceAndVolumes(t *testing.T) {
	fake := &fakeExecutor{}
	client := NewClientWithExecutor(fake, "/proj", "/proj/compose", nil)

	if err := client.Remove(context.Background(), RemoveOptions{
		Files:    []string{"compose.yaml"},
		Services: []string{"alt-backend"},
		Volumes:  true,
	}); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	joined := strings.Join(fake.lastCall(), " ")
	for _, want := range []string{"rm", "-f", "-v", "alt-backend"} {
		if !strings.Contains(joined, want) {
			t.Errorf("Remove argv %q missing %q", joined, want)
		}
	}
}

// TestClient_Exec_IncludesFileArgs guards the other half of the H2 fix:
// `altctl exec <service> -- <cmd>` used to call client.Exec with no -f at
// all, so every real invocation died with "no configuration file provided".
func TestClient_Exec_IncludesFileArgs(t *testing.T) {
	fake := &fakeExecutor{}
	client := NewClientWithExecutor(fake, "/proj", "/proj/compose", nil)

	var stdout, stderr bytes.Buffer
	err := client.Exec(context.Background(), []string{"compose.yaml"}, "alt-backend", []string{"sh"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Exec failed: %v", err)
	}

	got := fake.lastCall()
	joined := strings.Join(got, " ")
	for _, want := range []string{"-f " + filepath.Join("/proj/compose", "compose.yaml"), "exec alt-backend sh"} {
		if !strings.Contains(joined, want) {
			t.Errorf("Exec argv %q missing %q", joined, want)
		}
	}
}

func indexOfArg(args []string, want string) int {
	for i, a := range args {
		if a == want {
			return i
		}
	}
	return -1
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
