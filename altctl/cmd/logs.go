package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/alt-project/altctl/internal/compose"
	"github.com/alt-project/altctl/internal/output"
	"github.com/alt-project/altctl/internal/stack"
)

var logsCmd = &cobra.Command{
	Use:   "logs <service|stack>",
	Short: "Tail logs for a service or stack",
	Long: `Stream logs from a specific service or all services in a stack.

Examples:
  altctl logs alt-backend      # Tail backend logs
  altctl logs alt-backend -f   # Follow log output
  altctl logs alt-backend -n 100  # Show last 100 lines
  altctl logs db --since 1h    # Show logs from last hour
  altctl logs recap            # Tail all recap stack services`,
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeServiceAndStackNames,
	RunE:              runLogs,
}

func init() {
	rootCmd.AddCommand(logsCmd)

	logsCmd.Flags().BoolP("follow", "f", false, "follow log output")
	logsCmd.Flags().IntP("tail", "n", 100, "number of lines to show")
	logsCmd.Flags().BoolP("timestamps", "t", false, "show timestamps")
	logsCmd.Flags().String("since", "", "show logs since timestamp (e.g., 2h, 30m)")
}

func runLogs(cmd *cobra.Command, args []string) error {
	target := args[0]
	printer := newPrinter()

	// Check if target is a stack name
	registry, err := loadRegistry()
	if err != nil {
		return err
	}
	var services []string
	var files []string
	if s, ok := registry.Get(target); ok {
		services = s.Services
		files = buildStackInvocation([]*stack.Stack{s}).Files
		printer.Info("Showing logs for stack '%s' (%d services)", target, len(services))
	} else if s, ferr := registry.FindByService(target); ferr != nil {
		return ambiguousLogsTargetError(target, ferr)
	} else if s != nil {
		services = []string{target}
		files = buildStackInvocation([]*stack.Stack{s}).Files
	} else {
		printer.Warning("'%s' not found as a service or stack name", target)
		services = []string{target} // Pass through to docker compose
		// H2 fix: `docker compose logs` with no -f at all dies with "no
		// configuration file provided" -- fall back to the aggregate file
		// (compose/compose.yaml) since the target's owning stack is
		// unknown here.
		files = []string{stack.AggregateComposeFile}
	}

	// Get flags
	follow, _ := cmd.Flags().GetBool("follow")
	tail, _ := cmd.Flags().GetInt("tail")
	timestamps, _ := cmd.Flags().GetBool("timestamps")
	since, _ := cmd.Flags().GetString("since")

	// Create compose client
	client := newComposeClient()

	// Create context (no timeout for follow mode)
	var ctx context.Context
	var cancel context.CancelFunc
	if follow {
		ctx, cancel = context.WithCancel(cmd.Context())
	} else {
		ctx, cancel = context.WithTimeout(cmd.Context(), 30*time.Second)
	}
	defer cancel()

	// Stream logs for all resolved services in a single invocation. Compose
	// accepts multiple service args natively; calling this once per service
	// would stick forever on the first service when --follow is set (see
	// internal/compose.Client.Logs).
	if err := client.Logs(ctx, files, services, compose.LogsOptions{
		Follow:     follow,
		Tail:       tail,
		Timestamps: timestamps,
		Since:      since,
	}); err != nil {
		return err
	}

	printer.PrintHints("logs")
	return nil
}

// ambiguousLogsTargetError wraps a stack.Registry.FindByService
// disambiguation error into a *output.CLIError for logs' usage-error path.
func ambiguousLogsTargetError(target string, cause error) *output.CLIError {
	return &output.CLIError{
		Summary:    fmt.Sprintf("cannot resolve %q to a single stack", target),
		Detail:     cause.Error(),
		Suggestion: "Pass the owning stack name explicitly instead of the bare service name",
		ExitCode:   output.ExitUsageError,
	}
}

// completeServiceNames provides shell completion for service names
func completeServiceNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	registry, err := loadRegistry()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	var services []string
	for _, s := range registry.All() {
		services = append(services, s.Services...)
	}

	return services, cobra.ShellCompDirectiveNoFileComp
}

// completeServiceAndStackNames provides shell completion for both service and stack names
func completeServiceAndStackNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	registry, err := loadRegistry()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	var completions []string
	completions = append(completions, registry.Names()...)
	for _, s := range registry.All() {
		completions = append(completions, s.Services...)
	}

	return completions, cobra.ShellCompDirectiveNoFileComp
}
