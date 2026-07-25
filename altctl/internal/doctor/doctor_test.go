package doctor

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alt-project/altctl/internal/stack"
)

// fakeExecutor implements compose.Executor against a caller-supplied
// handler, so tests can canned-response specific `docker ...` invocations
// (ps/config/logs/info) without a real Docker daemon. It records every call
// for assertions.
type fakeExecutor struct {
	handler func(cmd string, args []string) ([]byte, error)
	calls   [][]string
}

func (f *fakeExecutor) Run(ctx context.Context, cmd string, args []string) error {
	_, err := f.RunWithOutput(ctx, cmd, args)
	return err
}

func (f *fakeExecutor) RunWithOutput(ctx context.Context, cmd string, args []string) ([]byte, error) {
	f.calls = append(f.calls, append([]string{cmd}, args...))
	return f.handler(cmd, args)
}

func (f *fakeExecutor) RunWithPipes(ctx context.Context, cmd string, args []string, stdout, stderr io.Writer) error {
	_, err := f.RunWithOutput(ctx, cmd, args)
	return err
}

func containsAll(args []string, want ...string) bool {
	joined := strings.Join(args, " ")
	for _, w := range want {
		if !strings.Contains(joined, w) {
			return false
		}
	}
	return true
}

func psLine(t *testing.T, e psEntry) string {
	t.Helper()
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal psEntry: %v", err)
	}
	return string(b)
}

func configJSON(t *testing.T, services map[string]composeServiceConfig) []byte {
	t.Helper()
	b, err := json.Marshal(composeConfigDoc{Services: services})
	if err != nil {
		t.Fatalf("marshal composeConfigDoc: %v", err)
	}
	return b
}

// buildTestRegistry constructs a minimal registry with a tiny compose/
// fixture directory (base + web + logging stacks), independent of the real
// Alt monorepo, so classification/root-cause tests are self-contained and
// fast.
func buildTestRegistry(t *testing.T) (*stack.Registry, string) {
	t.Helper()
	dir := t.TempDir()
	composeDir := filepath.Join(dir, "compose")
	if err := os.MkdirAll(composeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	files := map[string]string{
		"compose.yaml": "name: alt\ninclude:\n  - base.yaml\n  - db.yaml\n  - web.yaml\n  - logging.yaml\n",
		"base.yaml":    "name: alt\nsecrets:\n  db_password:\n    file: ../secrets/db_password.txt\n",
		"db.yaml":      "name: alt\ninclude:\n  - base.yaml\nservices:\n  db:\n    image: postgres:17\n",
		"web.yaml":     "name: alt\ninclude:\n  - base.yaml\nservices:\n  web:\n    image: web:latest\n",
		"logging.yaml": "name: alt\ninclude:\n  - base.yaml\nservices:\n  log-forwarder:\n    image: forwarder:latest\n",
		"dev.yaml":     "name: alt\ninclude:\n  - base.yaml\nservices:\n  dev-tool:\n    image: dev:latest\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(composeDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	semantics := &stack.SemanticsConfig{
		Stacks: map[string]stack.StackSemantics{
			"base":    {Optional: false},
			"db":      {Optional: false, DependsOn: []string{"base"}},
			"web":     {Optional: false, DependsOn: []string{"base", "db"}},
			"logging": {Optional: true, DependsOn: []string{"base"}},
			"dev":     {Optional: true, DependsOn: []string{"base"}},
		},
	}

	reg, err := stack.NewRegistryFromSemantics(composeDir, semantics)
	if err != nil {
		t.Fatalf("NewRegistryFromSemantics: %v", err)
	}
	return reg, dir
}

func TestDiagnose_DockerUnreachable(t *testing.T) {
	reg, dir := buildTestRegistry(t)
	exec := &fakeExecutor{handler: func(cmd string, args []string) ([]byte, error) {
		if containsAll(args, "info") {
			return nil, errors.New("cannot connect to the Docker daemon")
		}
		t.Fatalf("unexpected call after daemon check: %v", args)
		return nil, nil
	}}

	report, err := Diagnose(context.Background(), Options{
		Registry:   reg,
		Executor:   exec,
		ProjectDir: dir,
		ComposeDir: filepath.Join(dir, "compose"),
	})
	if err != nil {
		t.Fatalf("Diagnose returned error: %v", err)
	}
	if report.DockerReachable {
		t.Fatal("expected DockerReachable=false")
	}
	if len(report.Stacks) != 0 {
		t.Fatalf("expected no stacks diagnosed when daemon unreachable, got %d", len(report.Stacks))
	}
	found := false
	for _, f := range report.Preflight {
		if strings.Contains(f.Message, "docker daemon unreachable") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a 'docker daemon unreachable' finding, got %+v", report.Preflight)
	}
	if !report.HasProblems() {
		t.Fatal("expected HasProblems() true when daemon unreachable")
	}
}

func TestDiagnose_UnknownStackIsError(t *testing.T) {
	reg, dir := buildTestRegistry(t)
	exec := &fakeExecutor{handler: func(cmd string, args []string) ([]byte, error) {
		t.Fatalf("no docker call expected before stack validation, got: %v", args)
		return nil, nil
	}}

	_, err := Diagnose(context.Background(), Options{
		Registry:   reg,
		Executor:   exec,
		ProjectDir: dir,
		ComposeDir: filepath.Join(dir, "compose"),
		Stacks:     []string{"nonexistent"},
	})
	if err == nil {
		t.Fatal("expected an error for an unknown stack name")
	}
}

func TestDiagnose_HealthyStackNoFindings(t *testing.T) {
	reg, dir := buildTestRegistry(t)
	writeEnvAndSecrets(t, dir)

	exec := &fakeExecutor{handler: func(cmd string, args []string) ([]byte, error) {
		switch {
		case containsAll(args, "info"):
			return []byte("{}"), nil
		case containsAll(args, "ps"):
			return []byte(psLine(t, psEntry{Name: "alt-db-1", Service: "db", State: "running", Health: "healthy"}) + "\n" +
				psLine(t, psEntry{Name: "alt-web-1", Service: "web", State: "running", Health: "healthy"})), nil
		case containsAll(args, "config"):
			return configJSON(t, map[string]composeServiceConfig{
				"db":  {},
				"web": {DependsOn: map[string]composeDependsOn{"db": {Condition: "service_started"}}},
			}), nil
		}
		t.Fatalf("unexpected docker call: %v", args)
		return nil, nil
	}}

	report, err := Diagnose(context.Background(), Options{
		Registry:   reg,
		Executor:   exec,
		ProjectDir: dir,
		ComposeDir: filepath.Join(dir, "compose"),
		Stacks:     []string{"db", "web"},
	})
	if err != nil {
		t.Fatalf("Diagnose error: %v", err)
	}
	if report.HasProblems() {
		t.Fatalf("expected no problems, got: %+v", report.Problems)
	}
	for _, sr := range report.Stacks {
		if !sr.Healthy {
			t.Errorf("stack %s expected healthy, findings: %+v", sr.Name, sr.Findings)
		}
	}
}

func TestDiagnose_RootCauseChaining(t *testing.T) {
	reg, dir := buildTestRegistry(t)
	writeEnvAndSecrets(t, dir)

	exec := &fakeExecutor{handler: func(cmd string, args []string) ([]byte, error) {
		switch {
		case containsAll(args, "info"):
			return []byte("{}"), nil
		case containsAll(args, "logs"):
			return []byte("boom: connection refused\n"), nil
		case containsAll(args, "ps"):
			// db is unhealthy; web is "running" (no problem of its own) but
			// depends on db, so web's finding should point at db.
			return []byte(psLine(t, psEntry{Name: "alt-db-1", Service: "db", State: "running", Health: "unhealthy"}) + "\n" +
				psLine(t, psEntry{Name: "alt-web-1", Service: "web", State: "restarting", Health: ""})), nil
		case containsAll(args, "config"):
			return configJSON(t, map[string]composeServiceConfig{
				"db":  {},
				"web": {DependsOn: map[string]composeDependsOn{"db": {Condition: "service_healthy"}}},
			}), nil
		}
		t.Fatalf("unexpected docker call: %v", args)
		return nil, nil
	}}

	report, err := Diagnose(context.Background(), Options{
		Registry:   reg,
		Executor:   exec,
		ProjectDir: dir,
		ComposeDir: filepath.Join(dir, "compose"),
		Stacks:     []string{"db", "web"},
	})
	if err != nil {
		t.Fatalf("Diagnose error: %v", err)
	}

	var webFinding *Finding
	for _, sr := range report.Stacks {
		if sr.Name != "web" {
			continue
		}
		for i := range sr.Findings {
			if sr.Findings[i].Service == "web" && sr.Findings[i].Category == "service" {
				webFinding = &sr.Findings[i]
			}
		}
	}
	if webFinding == nil {
		t.Fatal("expected a finding for web")
	}
	if webFinding.RootCause != "db" {
		t.Errorf("expected web's root cause to be db, got %q (message: %s)", webFinding.RootCause, webFinding.Message)
	}
	if !strings.Contains(webFinding.Message, "db") {
		t.Errorf("expected web's message to mention db, got: %s", webFinding.Message)
	}
	if len(webFinding.Evidence) == 0 {
		t.Error("expected log evidence to be captured for web")
	}
}

func TestDiagnose_MissingServiceNoLogFetch(t *testing.T) {
	reg, dir := buildTestRegistry(t)
	writeEnvAndSecrets(t, dir)

	exec := &fakeExecutor{handler: func(cmd string, args []string) ([]byte, error) {
		switch {
		case containsAll(args, "info"):
			return []byte("{}"), nil
		case containsAll(args, "logs"):
			t.Fatalf("logs should not be fetched for a missing (never-started) service")
			return nil, nil
		case containsAll(args, "ps"):
			return []byte(""), nil // nothing running at all
		case containsAll(args, "config"):
			return configJSON(t, map[string]composeServiceConfig{"db": {}}), nil
		}
		t.Fatalf("unexpected docker call: %v", args)
		return nil, nil
	}}

	report, err := Diagnose(context.Background(), Options{
		Registry:   reg,
		Executor:   exec,
		ProjectDir: dir,
		ComposeDir: filepath.Join(dir, "compose"),
		Stacks:     []string{"db"},
	})
	if err != nil {
		t.Fatalf("Diagnose error: %v", err)
	}
	if !report.HasProblems() {
		t.Fatal("expected a problem for the missing db service")
	}
	found := false
	for _, sr := range report.Stacks {
		for _, f := range sr.Findings {
			if f.Service == "db" && f.State == StateMissing {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("expected a 'missing' finding for db, got: %+v", report.Stacks)
	}
}

func TestDiagnose_HealthyDependsOnWithoutHealthcheck(t *testing.T) {
	reg, dir := buildTestRegistry(t)
	writeEnvAndSecrets(t, dir)

	exec := &fakeExecutor{handler: func(cmd string, args []string) ([]byte, error) {
		switch {
		case containsAll(args, "info"):
			return []byte("{}"), nil
		case containsAll(args, "logs"):
			return []byte(""), nil
		case containsAll(args, "ps"):
			return []byte(psLine(t, psEntry{Name: "alt-db-1", Service: "db", State: "running", Health: "healthy"}) + "\n" +
				psLine(t, psEntry{Name: "alt-web-1", Service: "web", State: "running", Health: "healthy"})), nil
		case containsAll(args, "config"):
			// web depends_on db with service_healthy, but db has no
			// healthcheck: block -- this must be flagged even though both
			// services currently look "running/healthy" in ps.
			return configJSON(t, map[string]composeServiceConfig{
				"db":  {},
				"web": {DependsOn: map[string]composeDependsOn{"db": {Condition: "service_healthy"}}},
			}), nil
		}
		t.Fatalf("unexpected docker call: %v", args)
		return nil, nil
	}}

	report, err := Diagnose(context.Background(), Options{
		Registry:   reg,
		Executor:   exec,
		ProjectDir: dir,
		ComposeDir: filepath.Join(dir, "compose"),
		Stacks:     []string{"db", "web"},
	})
	if err != nil {
		t.Fatalf("Diagnose error: %v", err)
	}
	found := false
	for _, sr := range report.Stacks {
		for _, f := range sr.Findings {
			if f.Category == "config" && strings.Contains(f.Message, "service_healthy") {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("expected a service_healthy-without-healthcheck finding, got: %+v", report.Stacks)
	}
}

func TestDiagnose_DefaultScopeIncludesRunningOptionalStack(t *testing.T) {
	reg, dir := buildTestRegistry(t)
	writeEnvAndSecrets(t, dir)

	exec := &fakeExecutor{handler: func(cmd string, args []string) ([]byte, error) {
		switch {
		case containsAll(args, "info"):
			return []byte("{}"), nil
		case containsAll(args, "logs"):
			return []byte(""), nil
		case containsAll(args, "ps"):
			return []byte(
				psLine(t, psEntry{Name: "alt-db-1", Service: "db", State: "running", Health: "healthy"}) + "\n" +
					psLine(t, psEntry{Name: "alt-web-1", Service: "web", State: "running", Health: "healthy"}) + "\n" +
					psLine(t, psEntry{Name: "alt-log-forwarder-1", Service: "log-forwarder", State: "running", Health: ""}),
			), nil
		case containsAll(args, "config"):
			return configJSON(t, map[string]composeServiceConfig{}), nil
		}
		t.Fatalf("unexpected docker call: %v", args)
		return nil, nil
	}}

	report, err := Diagnose(context.Background(), Options{
		Registry:   reg,
		Executor:   exec,
		ProjectDir: dir,
		ComposeDir: filepath.Join(dir, "compose"),
		// No explicit Stacks -> default scope.
	})
	if err != nil {
		t.Fatalf("Diagnose error: %v", err)
	}

	hasLogging := false
	for _, name := range report.Scope {
		if name == "logging" {
			hasLogging = true
		}
	}
	if !hasLogging {
		t.Fatalf("expected optional 'logging' stack (has a running container) in default scope, got: %v", report.Scope)
	}
}

func TestDiagnose_MissingDotEnvAndSecretsReported(t *testing.T) {
	reg, dir := buildTestRegistry(t)
	// Deliberately do NOT create .env or secrets/.

	exec := &fakeExecutor{handler: func(cmd string, args []string) ([]byte, error) {
		switch {
		case containsAll(args, "info"):
			return []byte("{}"), nil
		case containsAll(args, "ps"), containsAll(args, "config"):
			return nil, errors.New("env file not found")
		}
		t.Fatalf("unexpected docker call: %v", args)
		return nil, nil
	}}

	report, err := Diagnose(context.Background(), Options{
		Registry:   reg,
		Executor:   exec,
		ProjectDir: dir,
		ComposeDir: filepath.Join(dir, "compose"),
		Stacks:     []string{"db"},
	})
	if err != nil {
		t.Fatalf("Diagnose error: %v", err)
	}

	var messages []string
	for _, f := range report.Preflight {
		messages = append(messages, f.Message)
	}
	joined := strings.Join(messages, " | ")
	if !strings.Contains(joined, "missing .env") {
		t.Errorf("expected a missing .env finding, got: %s", joined)
	}
	if !strings.Contains(joined, "secret") {
		t.Errorf("expected a missing secrets finding, got: %s", joined)
	}
	if !report.HasProblems() {
		t.Fatal("expected HasProblems() true")
	}
}

func TestDiagnose_LoggingStackTriggersDockerGroupIDCheck(t *testing.T) {
	reg, dir := buildTestRegistry(t)
	writeEnvAndSecrets(t, dir)
	t.Setenv("DOCKER_GROUP_ID", "")

	exec := &fakeExecutor{handler: func(cmd string, args []string) ([]byte, error) {
		switch {
		case containsAll(args, "info"):
			return []byte("{}"), nil
		case containsAll(args, "logs"):
			return []byte(""), nil
		case containsAll(args, "ps"):
			return []byte(psLine(t, psEntry{Name: "alt-log-forwarder-1", Service: "log-forwarder", State: "running"})), nil
		case containsAll(args, "config"):
			return configJSON(t, map[string]composeServiceConfig{}), nil
		}
		t.Fatalf("unexpected docker call: %v", args)
		return nil, nil
	}}

	report, err := Diagnose(context.Background(), Options{
		Registry:   reg,
		Executor:   exec,
		ProjectDir: dir,
		ComposeDir: filepath.Join(dir, "compose"),
		Stacks:     []string{"logging"},
	})
	if err != nil {
		t.Fatalf("Diagnose error: %v", err)
	}
	found := false
	for _, f := range report.Preflight {
		if strings.Contains(f.Message, "DOCKER_GROUP_ID") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a DOCKER_GROUP_ID finding when logging is in scope, got: %+v", report.Preflight)
	}
}

func TestRenderText_NoProblems(t *testing.T) {
	r := &Report{
		DockerReachable: true,
		Scope:           []string{"db"},
		Stacks: []StackReport{
			{Name: "db", Healthy: true, ServiceCount: 1},
		},
	}
	out := r.RenderText()
	if !strings.Contains(out, "No problems found.") {
		t.Errorf("expected 'No problems found.' in output, got: %s", out)
	}
	if !strings.Contains(out, "[OK] db") {
		t.Errorf("expected healthy stack summary line, got: %s", out)
	}
}

func TestRenderText_DockerUnreachable(t *testing.T) {
	r := &Report{
		DockerReachable: false,
		Preflight: []Finding{
			{Severity: SeverityError, Message: "docker daemon unreachable", Detail: "connection refused"},
		},
	}
	out := r.RenderText()
	if !strings.Contains(out, "DOCKER DAEMON UNREACHABLE") {
		t.Errorf("expected loud daemon-unreachable header, got: %s", out)
	}
	if !strings.Contains(out, "connection refused") {
		t.Errorf("expected detail in output, got: %s", out)
	}
}

func writeEnvAndSecrets(t *testing.T, projectDir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(projectDir, ".env"), []byte("FOO=bar\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	secretsDir := filepath.Join(projectDir, "secrets")
	if err := os.MkdirAll(secretsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(secretsDir, "db_password.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}
