package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/alt-project/altctl/internal/mtlsedge"
	"github.com/alt-project/altctl/internal/output"
)

var (
	newMTLSExecutor = mtlsedge.DefaultExecutorFactory
	mtlsEnviron     = os.Environ
)

var mtlsCmd = &cobra.Command{
	Use:   "mtls",
	Short: "Read-only mTLS edge checks for pre-processor -> news-creator",
	Long: `mtls provides read-only pre-mutation and live-container mTLS edge verification
for the fixed edge pre-processor -> news-creator (https://news-creator:9443/queue/status).

Neither subcommand modifies the live .env, compose files, or running containers.

Exit codes: 0 pass; 1 check failed or not observed, also unreadable --env-file / missing compose/compose.yaml / flag errors; 2 usage error; 3 docker unreachable; 4 config error (--project-dir missing or .altctl.yaml invalid).`,
}

var mtlsValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate a candidate .env for the pre-processor -> news-creator mTLS edge in an isolated staging dir",
	Long: `validate is a READ-ONLY pre-mutation check (environment freeze + isolated
candidate render and semantic mTLS edge validation) for the edge
pre-processor -> news-creator.

It freezes the child environment to DOCKER_HOST, DOCKER_CONTEXT, DOCKER_CONFIG,
PATH, HOME, and COMPOSE_PROJECT_NAME=alt, failing closed if any variable
interpolated by compose files reachable through the include: graph from
compose/compose.yaml is present in the caller's environment. It then renders
the compose configuration against a temporary 0700 staging copy with the
candidate .env and verifies the effective mTLS edge configuration without
modifying the live .env or compose files or printing any secret values.

Examples:
  altctl mtls validate --env-file /path/to/candidate.env
  altctl mtls validate --env-file /path/to/candidate.env --json

Exit codes: 0 pass; 1 check failed or not observed, also unreadable --env-file / missing compose/compose.yaml / flag errors; 2 usage error; 3 docker unreachable; 4 config error (--project-dir missing or .altctl.yaml invalid).`,
	Args: cobra.NoArgs,
	RunE: runMTLSValidate,
}

var mtlsObserveCmd = &cobra.Command{
	Use:   "observe",
	Short: "Observe live containers and mTLS probe for the pre-processor -> news-creator edge",
	Long: `observe is a READ-ONLY post-rollout check (environment freeze + live
observation of running containers and mTLS probe) for the edge
pre-processor -> news-creator.

It applies the environment freeze across compose files reachable through the
include: graph from compose/compose.yaml, verifies that exactly one container is
running for both services, resolves image RepoDigests via docker image inspect
(and compares against expected image digests when supplied), checks for
effective environment drift between rendered compose config and running
container environments, validates the semantic mTLS edge configuration, and
executes a caller-originated mTLS probe against the non-exempt /queue/status
endpoint.

Examples:
  altctl mtls observe
  altctl mtls observe --expect-caller-image repo/pre-processor@sha256:... --expect-receiver-image repo/news-creator@sha256:...
  altctl mtls observe --json

Exit codes: 0 pass; 1 check failed or not observed, also unreadable --env-file / missing compose/compose.yaml / flag errors; 2 usage error; 3 docker unreachable; 4 config error (--project-dir missing or .altctl.yaml invalid).`,
	Args: cobra.NoArgs,
	RunE: runMTLSObserve,
}

func init() {
	rootCmd.AddCommand(mtlsCmd)
	mtlsCmd.AddCommand(mtlsValidateCmd)
	mtlsCmd.AddCommand(mtlsObserveCmd)

	mtlsValidateCmd.Flags().String("env-file", "", "path to candidate .env file to validate (required)")
	mtlsValidateCmd.Flags().Bool("json", false, "output evidence as JSON")

	mtlsObserveCmd.Flags().String("expect-caller-image", "", "expected caller image RepoDigest (<repo>@sha256:<64 lowercase hex>)")
	mtlsObserveCmd.Flags().String("expect-receiver-image", "", "expected receiver image RepoDigest (<repo>@sha256:<64 lowercase hex>)")
	mtlsObserveCmd.Flags().Bool("json", false, "output evidence as JSON")
}

func runMTLSValidate(cmd *cobra.Command, _ []string) error {
	if dryRun {
		return &output.CLIError{
			Summary:    "--dry-run is not supported for read-only mtls checks",
			Suggestion: "Re-run without --dry-run",
			ExitCode:   output.ExitUsageError,
		}
	}

	envFile, _ := cmd.Flags().GetString("env-file")
	jsonOutput, _ := cmd.Flags().GetBool("json")

	if envFile == "" {
		return &output.CLIError{
			Summary:    "--env-file is required",
			Suggestion: "Provide a candidate .env path with --env-file <path>",
			ExitCode:   output.ExitUsageError,
		}
	}

	res, err := mtlsedge.Validate(cmd.Context(), mtlsedge.ValidateOptions{
		ProjectDir:       getProjectRoot(),
		CandidateEnvFile: envFile,
		Environ:          mtlsEnviron(),
		NewExecutor:      newMTLSExecutor,
	})
	if err != nil {
		return &output.CLIError{
			Summary:  err.Error(),
			ExitCode: output.ExitGeneral,
		}
	}

	if err := writeMTLSResult(cmd, res, jsonOutput); err != nil {
		return err
	}
	return mtlsExitError(res)
}

func runMTLSObserve(cmd *cobra.Command, _ []string) error {
	if dryRun {
		return &output.CLIError{
			Summary:    "--dry-run is not supported for read-only mtls checks",
			Suggestion: "Re-run without --dry-run",
			ExitCode:   output.ExitUsageError,
		}
	}

	expectCaller, _ := cmd.Flags().GetString("expect-caller-image")
	expectReceiver, _ := cmd.Flags().GetString("expect-receiver-image")
	jsonOutput, _ := cmd.Flags().GetBool("json")

	if err := mtlsedge.ValidateExpectedImageRef(expectCaller); err != nil {
		return &output.CLIError{
			Summary:    fmt.Sprintf("invalid --expect-caller-image: %s", err.Error()),
			Suggestion: "Use format <repo>@sha256:<64 lowercase hex>",
			ExitCode:   output.ExitUsageError,
		}
	}
	if err := mtlsedge.ValidateExpectedImageRef(expectReceiver); err != nil {
		return &output.CLIError{
			Summary:    fmt.Sprintf("invalid --expect-receiver-image: %s", err.Error()),
			Suggestion: "Use format <repo>@sha256:<64 lowercase hex>",
			ExitCode:   output.ExitUsageError,
		}
	}

	res, err := mtlsedge.Observe(cmd.Context(), mtlsedge.ObserveOptions{
		ProjectDir:          getProjectRoot(),
		ExpectCallerImage:   expectCaller,
		ExpectReceiverImage: expectReceiver,
		Environ:             mtlsEnviron(),
		NewExecutor:         newMTLSExecutor,
	})
	if err != nil {
		return &output.CLIError{
			Summary:  err.Error(),
			ExitCode: output.ExitGeneral,
		}
	}

	if err := writeMTLSResult(cmd, res, jsonOutput); err != nil {
		return err
	}
	return mtlsExitError(res)
}

func writeMTLSResult(cmd *cobra.Command, res *mtlsedge.Result, jsonOutput bool) error {
	if jsonOutput {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		return enc.Encode(res.Evidence)
	}
	newPrinter().Print("%s", res.RenderText())
	return nil
}

func mtlsExitError(res *mtlsedge.Result) error {
	if !res.DockerReachable {
		return &output.CLIError{
			Summary:    "docker daemon unreachable",
			Suggestion: "start Docker and retry",
			ExitCode:   output.ExitComposeError,
		}
	}
	if !res.Passed() {
		return &output.CLIError{
			Summary:    "mTLS edge check failed",
			Suggestion: "inspect the reported reason identifiers above",
			ExitCode:   output.ExitGeneral,
		}
	}
	return nil
}
