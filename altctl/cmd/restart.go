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

var restartCmd = &cobra.Command{
	Use:   "restart [stacks...]",
	Short: "Restart specified stacks (down then up)",
	Long: `Restart one or more stacks by stopping and then starting them.

If no stacks are specified, restarts the default stacks.
Dependencies are automatically resolved for the up phase.

Examples:
  altctl restart                 # Restart default stacks
  altctl restart recap           # Restart recap stack
  altctl restart core --build    # Restart with image rebuild`,
	Args:              cobra.ArbitraryArgs,
	ValidArgsFunction: completeStackNames,
	RunE:              runRestart,
}

func init() {
	rootCmd.AddCommand(restartCmd)

	restartCmd.Flags().BoolP("build", "b", false, "rebuild images before starting")
	restartCmd.Flags().Duration("timeout", 5*time.Minute, "timeout for container startup")
}

func runRestart(cmd *cobra.Command, args []string) error {
	printer := newPrinter()
	registry := stack.NewRegistry()
	resolver := stack.NewDependencyResolver(registry)

	// Determine which stacks to restart
	var stackNames []string
	if len(args) > 0 {
		stackNames = args
	} else {
		stackNames = cfg.Defaults.Stacks
	}

	// Validate stack names
	for _, name := range stackNames {
		if _, ok := registry.Get(name); !ok {
			return &output.CLIError{
				Summary:    fmt.Sprintf("unknown stack: %s", name),
				Suggestion: "Run 'altctl list' to see available stacks",
				ExitCode:   output.ExitUsageError,
			}
		}
	}

	// Resolve dependencies
	stacks, err := resolver.Resolve(stackNames)
	if err != nil {
		return &output.CLIError{
			Summary:    "failed resolving dependencies",
			Detail:     err.Error(),
			Suggestion: "Check stack definitions with 'altctl list --deps'",
			ExitCode:   output.ExitUsageError,
		}
	}

	// Collect compose files
	var files []string
	for _, s := range stacks {
		if s.ComposeFile != "" {
			files = append(files, s.ComposeFile)
		}
	}

	if len(files) == 0 {
		printer.Warning("No compose files to restart")
		return nil
	}

	// Create compose client
	client := compose.NewClient(
		getProjectRoot(),
		getComposeDir(),
		logger,
		dryRun,
	)

	timeout, _ := cmd.Flags().GetDuration("timeout")
	build, _ := cmd.Flags().GetBool("build")

	// Phase 1: Down
	printer.Header("Stopping Stacks")
	for _, s := range stacks {
		printer.Info("  • %s", printer.Bold(s.Name))
	}
	fmt.Println()

	downCtx, downCancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer downCancel()

	err = client.Down(downCtx, compose.DownOptions{
		Files:   files,
		Timeout: 30 * time.Second,
	})
	if err != nil {
		printer.Error("Failed to stop stacks: %v", err)
		return err
	}

	// Phase 2: Up
	printer.Header("Starting Stacks")
	for _, s := range stacks {
		printer.Info("  • %s: %s", printer.Bold(s.Name), s.Description)
	}
	fmt.Println()

	upCtx, upCancel := context.WithTimeout(context.Background(), timeout)
	defer upCancel()

	err = client.Up(upCtx, compose.UpOptions{
		Files:   files,
		Detach:  true,
		Build:   build,
		Timeout: timeout,
	})
	if err != nil {
		printer.Error("Failed to start stacks: %v", err)
		return err
	}

	printer.Success("Stacks restarted successfully")
	printer.PrintHints("restart")
	return nil
}
