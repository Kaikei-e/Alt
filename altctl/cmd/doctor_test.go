package cmd

import (
	"bytes"
	"testing"

	"github.com/alt-project/altctl/internal/doctor"
	"github.com/alt-project/altctl/internal/output"
)

func setupDoctorTest(t *testing.T) {
	t.Helper()
	cfg = testConfig(t, []string{"db", "auth", "core", "workers"})
	dryRun = false
	quiet = false
	doctorCmd.Flags().Set("json", "false")
	doctorCmd.Flags().Set("tail", "30")
}

// An unknown stack name is rejected before doctor ever shells out to
// docker (internal/doctor.Diagnose validates names first), so this is safe
// to run for real via rootCmd.Execute() without a live daemon.
func TestDoctor_UnknownStack(t *testing.T) {
	setupDoctorTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"doctor", "nonexistent-stack"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for unknown stack, got nil")
	}
}

func TestDoctor_FlagsRegistered(t *testing.T) {
	jsonFlag := doctorCmd.Flags().Lookup("json")
	if jsonFlag == nil {
		t.Fatal("expected --json flag to be registered")
	}
	if jsonFlag.DefValue != "false" {
		t.Errorf("--json default = %q, want %q", jsonFlag.DefValue, "false")
	}

	tailFlag := doctorCmd.Flags().Lookup("tail")
	if tailFlag == nil {
		t.Fatal("expected --tail flag to be registered")
	}
	if tailFlag.DefValue != "30" {
		t.Errorf("--tail default = %q, want %q", tailFlag.DefValue, "30")
	}
}

func TestDoctor_RegisteredOnRootCmd(t *testing.T) {
	found := false
	for _, c := range rootCmd.Commands() {
		if c.Name() == "doctor" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected 'doctor' to be registered as a subcommand of rootCmd")
	}
}

// --- doctorExitError: pure mapping from Report to RunE's return value ---

func TestDoctorExitError_DockerUnreachable(t *testing.T) {
	report := &doctor.Report{DockerReachable: false}

	err := doctorExitError(report)
	if err == nil {
		t.Fatal("expected an error when docker is unreachable")
	}
	cliErr, ok := err.(*output.CLIError)
	if !ok {
		t.Fatalf("expected *output.CLIError, got %T", err)
	}
	if cliErr.ExitCode != output.ExitComposeError {
		t.Errorf("ExitCode = %d, want output.ExitComposeError (%d)", cliErr.ExitCode, output.ExitComposeError)
	}
}

func TestDoctorExitError_ProblemsFound(t *testing.T) {
	report := &doctor.Report{
		DockerReachable: true,
		Problems: []doctor.Finding{
			{Severity: doctor.SeverityError, Message: "alt-backend is unhealthy"},
		},
	}

	err := doctorExitError(report)
	if err == nil {
		t.Fatal("expected an error when problems were found")
	}
	cliErr, ok := err.(*output.CLIError)
	if !ok {
		t.Fatalf("expected *output.CLIError, got %T", err)
	}
	if cliErr.ExitCode != output.ExitGeneral {
		t.Errorf("ExitCode = %d, want output.ExitGeneral (%d)", cliErr.ExitCode, output.ExitGeneral)
	}
}

func TestDoctorExitError_NoProblems(t *testing.T) {
	report := &doctor.Report{DockerReachable: true}

	if err := doctorExitError(report); err != nil {
		t.Errorf("expected nil error for a clean report, got %v", err)
	}
}

// --- newDoctorExecutor: DOCKER_GROUP_ID workaround ---

func TestNewDoctorExecutor_InjectsPlaceholderDockerGroupIDWhenUnset(t *testing.T) {
	setupDoctorTest(t)
	t.Setenv("DOCKER_GROUP_ID", "")

	// newDoctorExecutor must not panic and must return a usable Executor
	// even though the real environment has no DOCKER_GROUP_ID set -- this
	// exercises the same construction path runDoctor uses, without
	// actually invoking docker (RunWithOutput isn't called here).
	exec := newDoctorExecutor()
	if exec == nil {
		t.Fatal("expected a non-nil Executor")
	}
}

// Sanity: prescriptions/help text reference `altctl doctor` consistently
// with main.go's interrupted-stack hint (see main.go), so the command name
// itself must not silently drift.
func TestDoctor_CommandName(t *testing.T) {
	if doctorCmd.Use != "doctor [stack...]" {
		t.Errorf("doctorCmd.Use = %q, unexpected drift from documented usage", doctorCmd.Use)
	}
}
