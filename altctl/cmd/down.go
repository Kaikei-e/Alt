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

var downCmd = &cobra.Command{
	Use:   "down [stacks...]",
	Short: "Stop specified stacks",
	Long: `Stop one or more stacks. By default, only stops the specified stacks.

If no stacks are specified, stops all running stacks.

Examples:
  altctl down                  # Stop all running stacks
  altctl down recap            # Stop only recap stack
  altctl down --volumes        # Stop and remove volumes
  altctl down db --with-deps   # Stop db and stacks that depend on it`,
	Args:              cobra.ArbitraryArgs,
	ValidArgsFunction: completeStackNames,
	RunE:              runDown,
}

func init() {
	rootCmd.AddCommand(downCmd)

	downCmd.Flags().Bool("volumes", false, "remove named volumes")
	downCmd.Flags().Bool("remove-orphans", false, "remove orphan containers")
	downCmd.Flags().Bool("with-deps", false, "also stop stacks that depend on the specified stacks")
	downCmd.Flags().Duration("timeout", 30*time.Second, "timeout for container shutdown")
}

func runDown(cmd *cobra.Command, args []string) error {
	printer := output.NewPrinter(cfg.Output.Colors)
	registry := stack.NewRegistry()
	resolver := stack.NewDependencyResolver(registry)

	// Determine which stacks to stop
	var stackNames []string
	if len(args) > 0 {
		stackNames = args
	} else {
		// Stop all stacks
		stackNames = registry.Names()
	}

	// Only resolve dependents if --with-deps is set
	withDeps, _ := cmd.Flags().GetBool("with-deps")
	var stacks []*stack.Stack
	var err error

	if withDeps {
		// Get stacks in reverse order (dependents first)
		stacks, err = resolver.ResolveWithDependents(stackNames)
		if err != nil {
			return fmt.Errorf("resolving dependencies: %w", err)
		}
	} else {
		// Default: only stop the specified stacks
		for _, name := range stackNames {
			s, ok := registry.Get(name)
			if !ok {
				return fmt.Errorf("unknown stack: %s", name)
			}
			stacks = append(stacks, s)
		}
	}

	// Collect compose files (reverse order for shutdown)
	var files []string
	for _, s := range stacks {
		if s.ComposeFile != "" {
			files = append(files, s.ComposeFile)
		}
	}

	if len(files) == 0 {
		printer.Warning("No compose files to stop")
		return nil
	}

	// Print what we're going to do
	printer.Header("Stopping Stacks")
	for _, s := range stacks {
		printer.Info("  • %s", printer.Bold(s.Name))
	}
	fmt.Println()

	// Get flags
	volumes, _ := cmd.Flags().GetBool("volumes")
	removeOrphans, _ := cmd.Flags().GetBool("remove-orphans")
	timeout, _ := cmd.Flags().GetDuration("timeout")

	// Create compose client
	client := compose.NewClient(
		getProjectRoot(),
		getComposeDir(),
		logger,
		dryRun,
	)

	// Stop services
	ctx, cancel := context.WithTimeout(context.Background(), timeout+30*time.Second)
	defer cancel()

	err = client.Down(ctx, compose.DownOptions{
		Files:         files,
		Volumes:       volumes,
		RemoveOrphans: removeOrphans,
		Timeout:       timeout,
	})

	if err != nil {
		printer.Error("Failed to stop stacks: %v", err)
		return err
	}

	if volumes {
		printer.Success("Stacks stopped and volumes removed")
	} else {
		printer.Success("Stacks stopped successfully")
	}
	return nil
}
