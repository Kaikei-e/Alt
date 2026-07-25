package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/alt-project/altctl/internal/compose"
	"github.com/alt-project/altctl/internal/doctor"
	"github.com/alt-project/altctl/internal/output"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor [stack...]",
	Short: "Diagnose stack problems: state, evidence, root cause, and next steps",
	Long: `doctor is a READ-ONLY diagnosis: it never restarts, recreates, or otherwise
changes anything. It classifies every problem service (missing, unhealthy,
restarting/crash-looping, exited non-zero, or still starting), captures the
last log lines as evidence, walks depends_on to point at the actual root
cause when a failure cascades, and suggests a concrete next command.

With no arguments the scope is every non-optional stack plus any optional
stack that currently has containers. Naming stacks narrows the scope to
exactly those.

Examples:
  altctl doctor                # Diagnose the whole running stack
  altctl doctor core sovereign # Diagnose just these stacks
  altctl doctor --json         # Machine-readable findings

Exit codes: 0 nothing wrong, 1 problems found, 3 docker itself is unreachable.`,
	Args:              cobra.ArbitraryArgs,
	ValidArgsFunction: completeStackNames,
	RunE:              runDoctor,
}

func init() {
	rootCmd.AddCommand(doctorCmd)

	doctorCmd.Flags().Bool("json", false, "output findings as JSON")
	doctorCmd.Flags().Int("tail", 30, "number of log lines to capture as evidence per problem service")
}

func runDoctor(cmd *cobra.Command, args []string) error {
	printer := newPrinter()
	registry, err := loadRegistry()
	if err != nil {
		return err
	}

	jsonOutput, _ := cmd.Flags().GetBool("json")
	tail, _ := cmd.Flags().GetInt("tail")

	exec := newDoctorExecutor()

	report, err := doctor.Diagnose(cmd.Context(), doctor.Options{
		Registry:     registry,
		Executor:     exec,
		ProjectDir:   getProjectRoot(),
		ComposeDir:   getComposeDir(),
		Stacks:       args,
		LogTailLines: tail,
	})
	if err != nil {
		return &output.CLIError{
			Summary:    err.Error(),
			Suggestion: "Run 'altctl list' to see available stacks",
			ExitCode:   output.ExitUsageError,
		}
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			return err
		}
	} else {
		printer.Print("%s", report.RenderText())
	}

	return doctorExitError(report)
}

// newDoctorExecutor builds the compose.Executor doctor uses for its
// read-only introspection (docker info / compose ps / compose config /
// compose logs). It deliberately always executes for real, ignoring the
// global --dry-run flag: doctor has no side effects to preview in the first
// place, and a "dry run" that fakes empty ps/config output would make every
// service look "missing" -- exactly the misleading failure mode doctor
// exists to prevent.
//
// It also works around an unrelated landmine: compose/logging.yaml requires
// DOCKER_GROUP_ID to even parse (`${DOCKER_GROUP_ID:?...}`), which would
// otherwise make `docker compose -f compose/compose.yaml config/ps` -- the
// aggregate probe doctor uses for every stack, see internal/doctor/probe.go
// -- hard-fail for users who aren't touching the logging stack at all. A
// harmless placeholder is injected for doctor's own read-only calls only
// (it's never used to actually start a container); the real unset condition
// is still separately flagged as a preflight Finding whenever the logging
// stack ends up in scope.
func newDoctorExecutor() compose.Executor {
	exec := compose.NewExecutor(getProjectRoot(), logger, false)
	if os.Getenv("DOCKER_GROUP_ID") == "" {
		exec.SetEnv("DOCKER_GROUP_ID", "0")
	}
	return exec
}

// doctorExitError maps a completed Report to the RunE return value that
// gives main.go the right exit code (see doctorCmd.Long): a distinct,
// honest error when docker itself couldn't be reached (ExitComposeError),
// a summary error when problems were found (ExitGeneral), or nil when
// everything's clean. The detailed report has already been printed by
// runDoctor by the time this runs.
func doctorExitError(report *doctor.Report) error {
	if !report.DockerReachable {
		return &output.CLIError{
			Summary:    "docker daemon unreachable",
			Suggestion: "start Docker and retry (see the finding above for detail)",
			ExitCode:   output.ExitComposeError,
		}
	}
	if report.HasProblems() {
		return &output.CLIError{
			Summary:    fmt.Sprintf("%d problem(s) found", len(report.Problems)),
			Suggestion: "see the findings above for evidence and next steps",
			ExitCode:   output.ExitGeneral,
		}
	}
	return nil
}
