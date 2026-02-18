package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/alt-project/altctl/internal/compose"
	"github.com/alt-project/altctl/internal/output"
	"github.com/alt-project/altctl/internal/stack"
)

var upCmd = &cobra.Command{
	Use:   "up [stacks...]",
	Short: "Start specified stacks",
	Long: `Start one or more stacks with automatic dependency resolution.

If no stacks are specified, starts the default stacks (db, auth, core, workers).
Dependencies are automatically started in the correct order.

Examples:
  altctl up                    # Start default stacks
  altctl up core db            # Start core and db stacks
  altctl up --all              # Start all stacks including optional ones
  altctl up ai --build         # Start AI stack with image rebuild
  altctl up core --no-deps     # Start core without dependencies`,
	Args:              cobra.ArbitraryArgs,
	ValidArgsFunction: completeStackNames,
	RunE:              runUp,
}

func init() {
	rootCmd.AddCommand(upCmd)

	upCmd.Flags().BoolP("build", "b", false, "rebuild images before starting")
	upCmd.Flags().BoolP("detach", "d", true, "run in detached mode")
	upCmd.Flags().Bool("no-deps", false, "don't start dependent stacks")
	upCmd.Flags().Bool("all", false, "start all stacks including optional ones")
	upCmd.Flags().Duration("timeout", 5*time.Minute, "timeout for container startup")
	upCmd.Flags().Bool("remove-orphans", false, "remove orphan containers")
	upCmd.Flags().String("progress", "auto", "set type of progress output (auto, tty, plain, quiet) (implies --build)")
}

func runUp(cmd *cobra.Command, args []string) error {
	printer := newPrinter()
	registry := stack.NewRegistry()
	resolver := stack.NewDependencyResolver(registry)

	// Determine which stacks to start
	var stackNames []string
	all, _ := cmd.Flags().GetBool("all")

	if all {
		stackNames = registry.Names()
	} else if len(args) > 0 {
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

	// Check for GPU requirement
	for _, s := range stacks {
		if s.RequiresGPU {
			printer.Warning("Stack '%s' requires GPU. Ensure NVIDIA drivers are installed.", s.Name)
		}
	}

	// Check for missing feature dependencies
	featureResolver := stack.NewFeatureResolver(registry)
	resolvedStackNames := make([]string, len(stacks))
	for i, s := range stacks {
		resolvedStackNames[i] = s.Name
	}

	warnings := featureResolver.CheckMissingFeatures(resolvedStackNames)
	if len(warnings) > 0 {
		printer.Header("Feature Warnings")
		for _, w := range warnings {
			printer.Warning("Stack '%s' requires feature '%s' which is not available.", w.Stack, w.MissingFeature)
			if len(w.ProvidedBy) > 0 {
				printer.Info("  Suggestion: Also start: %s", w.ProvidedBy[0])
			}
		}

		// Show command suggestion
		suggested := featureResolver.SuggestAdditionalStacks(resolvedStackNames)
		if len(suggested) > 0 {
			fmt.Println()
			printer.Info("To include suggested stacks, run:")
			suggestedArgs := append(stackNames, suggested...)
			printer.Info("  altctl up %s", strings.Join(suggestedArgs, " "))
		}
		fmt.Println()
	}

	// Collect compose files
	var files []string
	for _, s := range stacks {
		if s.ComposeFile != "" {
			files = append(files, s.ComposeFile)
		}
	}

	if len(files) == 0 {
		printer.Warning("No compose files to start")
		return nil
	}

	// Print what we're going to do
	printer.Header("Starting Stacks")
	for _, s := range stacks {
		printer.Info("  • %s: %s", printer.Bold(s.Name), s.Description)
	}
	fmt.Println()

	// Get flags
	build, _ := cmd.Flags().GetBool("build")
	detach, _ := cmd.Flags().GetBool("detach")
	timeout, _ := cmd.Flags().GetDuration("timeout")
	removeOrphans, _ := cmd.Flags().GetBool("remove-orphans")
	progress, _ := cmd.Flags().GetString("progress")

	// Disable remove-orphans when --no-deps is used to prevent removing other stacks
	if noDeps && removeOrphans && !cmd.Flags().Changed("remove-orphans") {
		removeOrphans = false
		printer.Warning("Auto-disabled --remove-orphans (use --remove-orphans=true to override)")
	}

	// Create compose client
	client := compose.NewClient(
		getProjectRoot(),
		getComposeDir(),
		logger,
		dryRun,
	)

	// If progress is specified or build is requested with progress, run build first
	// We do this because 'docker compose up --build' doesn't support --progress flag directly in all versions/wrappers
	// and it gives us better control.
	if progress != "auto" || (build && cmd.Flags().Changed("progress")) {
		printer.Header("Building Stacks")

		buildCtx, buildCancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer buildCancel()

		err = client.Build(buildCtx, compose.BuildOptions{
			Files:    files,
			Progress: progress,
		})
		if err != nil {
			printer.Error("Failed to build stacks: %v", err)
			return err
		}

		// We just built, so we don't need to build again in Up
		build = false
	}

	// Start services
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	err = client.Up(ctx, compose.UpOptions{
		Files:         files,
		Detach:        detach,
		Build:         build,
		NoDeps:        false, // We've already resolved deps
		Timeout:       timeout,
		RemoveOrphans: removeOrphans,
	})

	if err != nil {
		printer.Error("Failed to start stacks: %v", err)

		// Diagnose partial startup
		psCtx, psCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer psCancel()
		statuses, psErr := client.PS(psCtx, files)
		if psErr == nil {
			diag := classifyServices(stacks, statuses)
			if cliErr := buildPartialStartupError(diag, err); cliErr != nil {
				fmt.Println()
				printDiagnostic(printer, diag)
				return cliErr
			}
		}
		return err
	}

	printer.Success("Stacks started successfully")
	printer.PrintHints("up")
	return nil
}

// completeStackNames provides shell completion for stack names
func completeStackNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	registry := stack.NewRegistry()
	names := registry.Names()

	// Filter out already specified stacks
	seen := make(map[string]bool)
	for _, arg := range args {
		seen[arg] = true
	}

	var completions []string
	for _, name := range names {
		if !seen[name] {
			completions = append(completions, name)
		}
	}

	return completions, cobra.ShellCompDirectiveNoFileComp
}

// serviceDiag holds the classification of services after a partial startup failure.
type serviceDiag struct {
	running   []string
	unhealthy []string
	missing   []string
	expected  map[string]string // service name → stack name
}

// classifyServices compares expected services from stacks against actual statuses
// and classifies each as running, unhealthy, or missing.
func classifyServices(stacks []*stack.Stack, statuses []compose.ServiceStatus) serviceDiag {
	expected := make(map[string]string)
	for _, s := range stacks {
		for _, svc := range s.Services {
			expected[svc] = s.Name
		}
	}

	actual := make(map[string]compose.ServiceStatus)
	for _, s := range statuses {
		actual[s.Name] = s
	}

	var diag serviceDiag
	diag.expected = expected
	for svc := range expected {
		if status, ok := actual[svc]; ok {
			if status.Health == "unhealthy" {
				diag.unhealthy = append(diag.unhealthy, svc)
			} else {
				diag.running = append(diag.running, svc)
			}
		} else {
			diag.missing = append(diag.missing, svc)
		}
	}
	sort.Strings(diag.running)
	sort.Strings(diag.unhealthy)
	sort.Strings(diag.missing)
	return diag
}

// printDiagnostic renders the service status table after a startup failure.
func printDiagnostic(printer *output.Printer, diag serviceDiag) {
	printer.Header("Service Status After Failure")
	table := output.NewTable([]string{"SERVICE", "STACK", "STATUS"})
	for _, svc := range diag.running {
		table.AddRow([]string{svc, diag.expected[svc], printer.StatusBadge("running") + " running"})
	}
	for _, svc := range diag.unhealthy {
		table.AddRow([]string{svc, diag.expected[svc], printer.StatusBadge("restarting") + " unhealthy"})
	}
	for _, svc := range diag.missing {
		table.AddRow([]string{svc, diag.expected[svc], printer.StatusBadge("exited") + " not started"})
	}
	table.Render()
	printer.Info("Running: %d | Unhealthy: %d | Not started: %d",
		len(diag.running), len(diag.unhealthy), len(diag.missing))
}

// buildPartialStartupError constructs a CLIError from the diagnostic result.
// Returns nil when there are no expected services (nothing to diagnose).
func buildPartialStartupError(diag serviceDiag, cause error) *output.CLIError {
	if len(diag.expected) == 0 {
		return nil
	}

	total := len(diag.expected)
	started := len(diag.running)
	detail := fmt.Sprintf("%d of %d services started", started, total)
	suggestion := "Run 'altctl status' to see current state"

	if len(diag.missing) > 0 {
		stackSet := make(map[string]bool)
		for _, svc := range diag.missing {
			stackSet[diag.expected[svc]] = true
		}
		var stackNames []string
		for name := range stackSet {
			stackNames = append(stackNames, name)
		}
		sort.Strings(stackNames)
		suggestion += fmt.Sprintf(
			", or 'altctl up %s --build' to rebuild",
			strings.Join(stackNames, " "))
	}

	return &output.CLIError{
		Summary:    fmt.Sprintf("partial startup: %s", detail),
		Detail:     cause.Error(),
		Suggestion: suggestion,
		ExitCode:   output.ExitComposeError,
	}
}
