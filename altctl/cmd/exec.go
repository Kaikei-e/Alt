package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/alt-project/altctl/internal/compose"
	"github.com/alt-project/altctl/internal/output"
	"github.com/alt-project/altctl/internal/stack"
)

var execCmd = &cobra.Command{
	Use:   "exec <service> -- <command...>",
	Short: "Execute a command in a running service container",
	Long: `Run a command inside a running service container.

Uses 'docker compose exec' under the hood.

Examples:
  altctl exec alt-backend -- sh
  altctl exec db -- psql -U postgres
  altctl exec alt-backend -- go version`,
	Args:              cobra.MinimumNArgs(1),
	ValidArgsFunction: completeServiceNames,
	RunE:              runExec,
}

func init() {
	rootCmd.AddCommand(execCmd)
}

func runExec(cmd *cobra.Command, args []string) error {
	service := args[0]

	// Validate service exists
	registry := stack.NewRegistry()
	s := registry.FindByService(service)
	if s == nil {
		return &output.CLIError{
			Summary:    fmt.Sprintf("unknown service: %s", service),
			Suggestion: "Run 'altctl list --services' to see available services",
			ExitCode:   output.ExitUsageError,
		}
	}

	// Get command to execute (everything after --)
	var execArgs []string
	if cmd.ArgsLenAtDash() > 0 {
		execArgs = args[cmd.ArgsLenAtDash():]
	} else if len(args) > 1 {
		execArgs = args[1:]
	} else {
		return &output.CLIError{
			Summary:    "no command specified",
			Suggestion: "Usage: altctl exec <service> -- <command>",
			ExitCode:   output.ExitUsageError,
		}
	}

	// Create compose client
	client := compose.NewClient(
		getProjectRoot(),
		getComposeDir(),
		logger,
		dryRun,
	)

	ctx := context.Background()
	return client.Exec(ctx, service, execArgs, os.Stdout, os.Stderr)
}
