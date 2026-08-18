package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/alt-project/altctl/internal/compose"
	"github.com/alt-project/altctl/internal/health"
	"github.com/alt-project/altctl/internal/output"
	"github.com/alt-project/altctl/internal/stack"
)

func setupUpTest(t *testing.T) {
	t.Helper()
	cfg = testConfig(t, []string{"db", "auth", "core", "workers"})
	dryRun = true
	quiet = false
	// Reset flags that persist between test runs
	upCmd.Flags().Set("all", "false")
	upCmd.Flags().Set("no-deps", "false")
	upCmd.Flags().Set("build", "false")
	upCmd.Flags().Set("remove-orphans", "false")
	upCmd.Flags().Set("progress", "auto")
	upCmd.Flags().Set("detach", "false")
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
	// Realistic `docker compose ps` shape: Name is the container name,
	// Service is the compose service name classifyServices must key on.
	statuses := []compose.ServiceStatus{
		{Name: "alt-tag-generator-1", Service: "tag-generator", State: "running"},
		{Name: "alt-auth-token-manager-1", Service: "auth-token-manager", State: "running"},
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
		{Name: "alt-auth-token-manager-1", Service: "auth-token-manager", State: "running"},
		{Name: "alt-search-indexer-1", Service: "search-indexer", State: "running"},
		{Name: "alt-tag-generator-1", Service: "tag-generator", State: "running"},
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
		{Name: "alt-auth-token-manager-1", Service: "auth-token-manager", State: "running"},
		{Name: "alt-search-indexer-1", Service: "search-indexer", State: "running", Health: "unhealthy"},
		{Name: "alt-tag-generator-1", Service: "tag-generator", State: "running"},
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

// --- Ready-wait wiring: --detach default, timeout selection, failure diagnostics ---

func TestUp_DetachFlag_DefaultsToFalse(t *testing.T) {
	setupUpTest(t)

	f := upCmd.Flags().Lookup("detach")
	if f == nil {
		t.Fatal("expected --detach flag to exist")
	}
	if f.DefValue != "false" {
		t.Errorf("--detach default = %q, want %q (up must wait for Ready by default; --detach opts back into fire-and-forget)", f.DefValue, "false")
	}
}

func TestUp_DetachFlag_SkipsReadyWaitInDryRun(t *testing.T) {
	setupUpTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"up", "core", "--detach", "--dry-run"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("up --detach failed: %v", err)
	}
}

func TestMaxStartupTimeout_UsesLargestStackTimeout(t *testing.T) {
	stacks := []*stack.Stack{
		{Name: "core", Timeout: 30 * time.Second},
		{Name: "ai", Timeout: 1200 * time.Second},
		{Name: "recap", Timeout: 1200 * time.Second},
	}
	got := maxStartupTimeout(stacks)
	if got != 1200*time.Second {
		t.Errorf("maxStartupTimeout = %v, want 1200s", got)
	}
}

func TestMaxStartupTimeout_FloorsAtFiveMinutesWhenUnset(t *testing.T) {
	stacks := []*stack.Stack{
		{Name: "core"}, // Timeout unset -> GetTimeout() default is 5m
	}
	got := maxStartupTimeout(stacks)
	if got != 5*time.Minute {
		t.Errorf("maxStartupTimeout = %v, want 5m floor", got)
	}
}

func TestMaxStartupTimeout_EmptyStacks(t *testing.T) {
	got := maxStartupTimeout(nil)
	if got != 5*time.Minute {
		t.Errorf("maxStartupTimeout(nil) = %v, want 5m floor", got)
	}
}

func readyState(service, stackName string) health.State {
	return health.State{Service: service, Stack: stackName, Ready: true, Reason: "running"}
}

func notReadyState(service, stackName, reason string) health.State {
	return health.State{Service: service, Stack: stackName, Ready: false, Reason: reason}
}

func TestDiagnosticFromStates_ClassifiesByReadyAndReason(t *testing.T) {
	stacks := []*stack.Stack{
		{Name: "workers", Services: []string{"auth-token-manager", "search-indexer", "tag-generator"}},
	}
	states := []health.State{
		readyState("auth-token-manager", "workers"),
		notReadyState("search-indexer", "workers", "missing"),
		notReadyState("tag-generator", "workers", "health: starting"),
	}

	diag := diagnosticFromStates(stacks, states)

	if !slices.Equal(diag.running, []string{"auth-token-manager"}) {
		t.Errorf("running: got %v", diag.running)
	}
	if !slices.Equal(diag.missing, []string{"search-indexer"}) {
		t.Errorf("missing: got %v", diag.missing)
	}
	if !slices.Equal(diag.unhealthy, []string{"tag-generator"}) {
		t.Errorf("unhealthy: got %v", diag.unhealthy)
	}
}

func TestRenderReadyFailure_AllReady_ReturnsNil(t *testing.T) {
	printer := newTestPrinter()
	result := &health.Result{Ready: true, States: []health.State{readyState("a", "core")}}

	if cliErr := renderReadyFailure(t.Context(), printer, nil, nil, result); cliErr != nil {
		t.Errorf("expected nil CLIError when Ready, got %+v", cliErr)
	}
}

func TestRenderReadyFailure_TimedOut_ReturnsExitTimeout(t *testing.T) {
	dryRun = true
	printer := newTestPrinter()
	stacks := []*stack.Stack{{Name: "ai", Services: []string{"rerank-local"}}}
	result := &health.Result{
		Ready:    false,
		TimedOut: true,
		States:   []health.State{notReadyState("rerank-local", "ai", "health: starting")},
	}

	cliErr := renderReadyFailure(t.Context(), printer, nil, stacks, result)
	if cliErr == nil {
		t.Fatal("expected a CLIError for a timed-out result")
	}
	if cliErr.ExitCode != output.ExitTimeout {
		t.Errorf("ExitCode = %d, want output.ExitTimeout (%d)", cliErr.ExitCode, output.ExitTimeout)
	}
}

func TestRenderReadyFailure_NotReadyWithoutTimeout_ReturnsExitComposeError(t *testing.T) {
	dryRun = true
	printer := newTestPrinter()
	stacks := []*stack.Stack{{Name: "db", Services: []string{"migrator"}}}
	result := &health.Result{
		Ready:  false,
		States: []health.State{notReadyState("migrator", "db", "exited(1)")},
	}

	cliErr := renderReadyFailure(t.Context(), printer, nil, stacks, result)
	if cliErr == nil {
		t.Fatal("expected a CLIError for a not-ready, non-timed-out result")
	}
	if cliErr.ExitCode != output.ExitComposeError {
		t.Errorf("ExitCode = %d, want output.ExitComposeError (%d)", cliErr.ExitCode, output.ExitComposeError)
	}
}

func newTestPrinter() *output.Printer {
	return output.NewPrinterWithOptions(output.PrinterOptions{ColorMode: output.ColorNever, Quiet: true})
}

// --- M2: exact argv assertions (the kind that would have caught C3/C4/H2) ---

// TestUp_Core_ComposesAggregateFileAndCoreServices is the C3 regression
// guard directly on `altctl up core`'s real argv: before the fix, this
// built `-f base.yaml -f db.yaml -f pgbouncer.yaml -f auth.yaml -f
// sovereign.yaml -f core.yaml`, which real `docker compose ... config`
// rejects ("service 'alt-backend' depends on undefined service
// 'search-indexer'" / pki-agent sidecar variants -- see
// cmd/compose_target.go). It must now build a single `-f
// <composeDir>/compose.yaml` and name every resolved stack's own services
// explicitly (base has none; db/pgbouncer/auth/sovereign/core do).
func TestUp_Core_ComposesAggregateFileAndCoreServices(t *testing.T) {
	setupUpTest(t)
	fake := installFakeComposeClient(t)

	rootCmd.SetArgs([]string{"up", "core", "--dry-run"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("up core failed: %v", err)
	}

	argv, ok := fake.findArgv(" up ", "-d")
	if !ok {
		t.Fatalf("expected an 'up' invocation, got calls: %v", fake.argvs())
	}

	wantAggregateFile := "-f " + filepath.Join(getComposeDir(), "compose.yaml")
	if !strings.Contains(argv, wantAggregateFile) {
		t.Errorf("up core argv %q missing aggregate file arg %q", argv, wantAggregateFile)
	}
	// No per-stack narrow -f subset must appear -- that's exactly the
	// broken shape C3 fixed.
	for _, narrow := range []string{"base.yaml", "db.yaml", "pgbouncer.yaml", "auth.yaml", "sovereign.yaml", "core.yaml"} {
		if strings.Contains(argv, "-f "+filepath.Join(getComposeDir(), narrow)) {
			t.Errorf("up core argv %q must not contain a narrow per-stack -f %s (C3 regression)", argv, narrow)
		}
	}
	for _, svc := range []string{"plecto-proxy", "alt-frontend-sv", "alt-backend", "migrate"} {
		if !strings.Contains(argv, svc) {
			t.Errorf("up core argv %q missing core service %q", argv, svc)
		}
	}
}

// TestUp_Perf_IncludesProfileFlag guards the --profile requirement for
// compose-profile-gated stacks (perf declares profile: "perf" in
// .altctl.yaml).
func TestUp_Perf_IncludesProfileFlag(t *testing.T) {
	setupUpTest(t)
	fake := installFakeComposeClient(t)

	rootCmd.SetArgs([]string{"up", "perf", "--dry-run"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("up perf failed: %v", err)
	}

	argv, ok := fake.findArgv(" up ", "-d")
	if !ok {
		t.Fatalf("expected an 'up' invocation, got calls: %v", fake.argvs())
	}
	if !strings.Contains(argv, "--profile perf") {
		t.Errorf("up perf argv %q missing '--profile perf'", argv)
	}
}

// TestUp_Dev_UsesIsolatedFileSet guards the isolated-stack branch of
// buildStackInvocation: dev.yaml sits outside compose/compose.yaml's
// include: graph (and combining it with the aggregate breaks -- core.yaml
// and dev.yaml both redeclare alt-frontend-sv with conflicting resource
// limits), so `up dev` must use dev's own file, never the aggregate.
func TestUp_Dev_UsesIsolatedFileSet(t *testing.T) {
	setupUpTest(t)
	fake := installFakeComposeClient(t)

	rootCmd.SetArgs([]string{"up", "dev", "--dry-run"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("up dev failed: %v", err)
	}

	argv, ok := fake.findArgv(" up ", "-d")
	if !ok {
		t.Fatalf("expected an 'up' invocation, got calls: %v", fake.argvs())
	}

	wantDevFile := "-f " + filepath.Join(getComposeDir(), "dev.yaml")
	if !strings.Contains(argv, wantDevFile) {
		t.Errorf("up dev argv %q missing %q", argv, wantDevFile)
	}
	wantAggregateFile := "-f " + filepath.Join(getComposeDir(), "compose.yaml")
	if strings.Contains(argv, wantAggregateFile) {
		t.Errorf("up dev argv %q must not contain the aggregate file (dev/core.yaml service redeclarations conflict)", argv)
	}
}

// TestUp_LoadTest_CombinesAggregateWithOwnFile guards the more subtle
// isolated-stack case: load-test.yaml is outside the aggregate's include:
// graph (like dev/frontend-dev), but unlike them it is NOT self-sufficient
// alone -- perf.yaml's k6 service depends_on alt-backend, which only
// exists in core.yaml, unreachable from load-test's own dependency closure
// (base + perf + load-test). Empirically verified against real `docker
// compose ... config`: the pure isolated closure (base.yaml + perf.yaml +
// load-test.yaml) is REJECTED ("k6 depends on undefined service
// alt-backend"), while aggregate + load-test.yaml on top succeeds cleanly
// -- exactly what compose/load-test.yaml's own header comment documents as
// the supported direct-usage recipe. buildStackInvocation must pick the
// aggregate+isolated-file-on-top branch here, not the pure-isolated one.
func TestUp_LoadTest_CombinesAggregateWithOwnFile(t *testing.T) {
	setupUpTest(t)
	fake := installFakeComposeClient(t)

	rootCmd.SetArgs([]string{"up", "load-test", "--dry-run"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("up load-test failed: %v", err)
	}

	argv, ok := fake.findArgv(" up ", "-d")
	if !ok {
		t.Fatalf("expected an 'up' invocation, got calls: %v", fake.argvs())
	}

	wantAggregateFile := "-f " + filepath.Join(getComposeDir(), "compose.yaml")
	wantLoadTestFile := "-f " + filepath.Join(getComposeDir(), "load-test.yaml")
	if !strings.Contains(argv, wantAggregateFile) {
		t.Errorf("up load-test argv %q missing %q (load-test alone is not self-sufficient: perf's k6 depends on alt-backend, defined in core.yaml)", argv, wantAggregateFile)
	}
	if !strings.Contains(argv, wantLoadTestFile) {
		t.Errorf("up load-test argv %q missing %q", argv, wantLoadTestFile)
	}
	if !strings.Contains(argv, "--profile perf") || !strings.Contains(argv, "--profile load-test") {
		t.Errorf("up load-test argv %q missing --profile perf / --profile load-test", argv)
	}
	if !strings.Contains(argv, "mock-rss-server") {
		t.Errorf("up load-test argv %q missing mock-rss-server", argv)
	}
}

// cannedPSExecutor is a compose.Executor whose RunWithOutput always returns
// the canned `docker compose ps --format json` payload, for driving
// waitForReady against realistic ps output.
type cannedPSExecutor struct {
	ps string
}

func (c *cannedPSExecutor) Run(ctx context.Context, cmd string, args []string) error { return nil }

func (c *cannedPSExecutor) RunWithOutput(ctx context.Context, cmd string, args []string) ([]byte, error) {
	return []byte(c.ps), nil
}

func (c *cannedPSExecutor) RunWithPipes(ctx context.Context, cmd string, args []string, stdout, stderr io.Writer) error {
	return nil
}

// TestWaitForReady_MatchesComposeServiceName is the regression test for the
// "3/29 Ready — everything (missing)" failure: `docker compose ps` reports
// Name as the CONTAINER name ("alt-alt-butterfly-facade-1") and the compose
// service name in Service. The Ready-wait targets are service names
// (stack.Stack.Services), so the poller must hand the waiter Service, not
// Name — keying on Name left every service whose container_name differs
// from its service name permanently "missing", and `altctl up` sat at the
// Ready-wait until timeout with the whole stack actually healthy.
func TestWaitForReady_MatchesComposeServiceName(t *testing.T) {
	exec := &cannedPSExecutor{
		ps: `{"Name":"alt-alt-butterfly-facade-1","Service":"alt-butterfly-facade","State":"running","Health":"healthy","ExitCode":0}`,
	}
	client := compose.NewClientWithExecutor(exec, t.TempDir(), t.TempDir(), logger)
	printer := output.NewPrinter(false)
	stacks := []*stack.Stack{{Name: "bff", Services: []string{"alt-butterfly-facade"}}}

	result, err := waitForReady(context.Background(), printer, client, []string{"compose.yaml"}, stacks, 3*time.Second)
	if err != nil {
		t.Fatalf("waitForReady failed: %v", err)
	}
	if !result.Ready {
		t.Fatalf("expected Ready for a healthy running container of the target service, got states %+v", result.States)
	}
}
