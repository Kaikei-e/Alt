package cmd

import (
	"context"
	"time"

	"github.com/spf13/cobra"

	"github.com/alt-project/altctl/internal/compose"
	"github.com/alt-project/altctl/internal/output"
	"github.com/alt-project/altctl/internal/stack"
)

var logsCmd = &cobra.Command{
	Use:   "logs <service>",
	Short: "Tail logs for a service",
	Long: `Stream logs from a specific service.

Examples:
  altctl logs alt-backend      # Tail backend logs
  altctl logs alt-backend -f   # Follow log output
  altctl logs alt-backend -n 100  # Show last 100 lines
  altctl logs db --since 1h    # Show logs from last hour`,
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeServiceNames,
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
	service := args[0]
	printer := output.NewPrinter(cfg.Output.Colors)

	// Validate service exists
	registry := stack.NewRegistry()
	s := registry.FindByService(service)
	if s == nil {
		printer.Warning("Service '%s' not found in any stack", service)
	}

	// Get flags
	follow, _ := cmd.Flags().GetBool("follow")
	tail, _ := cmd.Flags().GetInt("tail")
	timestamps, _ := cmd.Flags().GetBool("timestamps")
	since, _ := cmd.Flags().GetString("since")

	// Create compose client
	client := compose.NewClient(
		getProjectRoot(),
		getProjectRoot(), // Use project root for logs, not compose dir
		logger,
		dryRun,
	)

	// Create context (no timeout for follow mode)
	var ctx context.Context
	var cancel context.CancelFunc
	if follow {
		ctx, cancel = context.WithCancel(context.Background())
	} else {
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	}
	defer cancel()

	// Stream logs
	return client.Logs(ctx, service, compose.LogsOptions{
		Follow:     follow,
		Tail:       tail,
		Timestamps: timestamps,
		Since:      since,
	})
}

// completeServiceNames provides shell completion for service names
func completeServiceNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	registry := stack.NewRegistry()
	var services []string
	for _, s := range registry.All() {
		services = append(services, s.Services...)
	}

	return services, cobra.ShellCompDirectiveNoFileComp
}
