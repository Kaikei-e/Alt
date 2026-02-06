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

var buildCmd = &cobra.Command{
	Use:   "build [stacks...]",
	Short: "Build images for specified stacks",
	Long: `Build Docker images for one or more stacks.

If no stacks are specified, builds all default stacks.
Dependencies are automatically built unless --no-deps is specified.

Examples:
  altctl build                 # Build all default stacks
  altctl build core            # Build core stack images
  altctl build recap --no-deps # Build only recap images
  altctl build --no-cache      # Build without cache
  altctl build --pull          # Pull base images before building`,
	Args:              cobra.ArbitraryArgs,
	ValidArgsFunction: completeStackNames,
	RunE:              runBuild,
}

func init() {
	rootCmd.AddCommand(buildCmd)

	buildCmd.Flags().Bool("no-cache", false, "build without cache")
	buildCmd.Flags().Bool("pull", false, "pull base images before building")
	buildCmd.Flags().Bool("parallel", true, "build in parallel")
	buildCmd.Flags().Bool("no-deps", false, "don't build dependent stacks")
	buildCmd.Flags().String("progress", "auto", "set type of progress output (auto, tty, plain, quiet)")
}

func runBuild(cmd *cobra.Command, args []string) error {
	printer := newPrinter()
	registry := stack.NewRegistry()
	resolver := stack.NewDependencyResolver(registry)

	// Determine which stacks to build
	var stackNames []string
	if len(args) > 0 {
		stackNames = args
	} else {
		stackNames = cfg.Defaults.Stacks
	}

	// Resolve dependencies unless --no-deps is set
	noDeps, _ := cmd.Flags().GetBool("no-deps")
	var stacks []*stack.Stack
	var err error

	if noDeps {
		for _, name := range stackNames {
			s, ok := registry.Get(name)
			if !ok {
				return &output.CLIError{
					Summary:    fmt.Sprintf("unknown stack: %s", name),
					Suggestion: "Run 'altctl list' to see available stacks",
					ExitCode:   output.ExitUsageError,
				}
			}
			stacks = append(stacks, s)
		}
	} else {
		stacks, err = resolver.Resolve(stackNames)
		if err != nil {
			return &output.CLIError{
				Summary:    "failed resolving dependencies",
				Detail:     err.Error(),
				Suggestion: "Check stack definitions with 'altctl list --deps'",
				ExitCode:   output.ExitUsageError,
			}
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
		printer.Warning("No compose files to build")
		return nil
	}

	// Print what we're going to do
	printer.Header("Building Stacks")
	for _, s := range stacks {
		printer.Info("  • %s", printer.Bold(s.Name))
	}
	fmt.Println()

	// Get flags
	noCache, _ := cmd.Flags().GetBool("no-cache")
	pull, _ := cmd.Flags().GetBool("pull")
	parallel, _ := cmd.Flags().GetBool("parallel")
	progress, _ := cmd.Flags().GetString("progress")

	// Create compose client
	client := compose.NewClient(
		getProjectRoot(),
		getComposeDir(),
		logger,
		dryRun,
	)

	// Build services
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	err = client.Build(ctx, compose.BuildOptions{
		Files:    files,
		NoCache:  noCache,
		Pull:     pull,
		Parallel: parallel,
		Progress: progress,
	})

	if err != nil {
		printer.Error("Failed to build stacks: %v", err)
		return err
	}

	printer.Success("Build completed successfully")
	printer.PrintHints("build")
	return nil
}
