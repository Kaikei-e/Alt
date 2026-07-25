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
	registry, err := loadRegistry()
	if err != nil {
		return err
	}
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

	// Compute the -f file list / --profile list / service names, same
	// aggregate-file-first strategy `up` uses (C3 fix) -- see
	// cmd/compose_target.go's buildStackInvocation doc comment.
	inv := buildStackInvocation(stacks)
	files := inv.Files

	if len(inv.Services) == 0 {
		printer.Warning("No compose files to restart")
		return nil
	}

	// Create compose client
	client := newComposeClient()

	timeout, _ := cmd.Flags().GetDuration("timeout")
	build, _ := cmd.Flags().GetBool("build")

	// Phase 1: Down -- stop + remove scoped to just the resolved stacks'
	// own services, never a plain `docker compose down`, which (now that
	// Files may be just the aggregate compose.yaml) would tear down the
	// *entire* project instead of only what was asked to restart. See
	// cmd/down.go's runScopedDown doc comment for why stop+rm replaces a
	// scoped down (down [SERVICES] doesn't cleanly scope volumes).
	printer.Header("Stopping Stacks")
	for _, s := range stacks {
		printer.Info("  • %s", printer.Bold(s.Name))
	}
	fmt.Println()

	downCtx, downCancel := context.WithTimeout(cmd.Context(), 60*time.Second)
	defer downCancel()

	if err := client.Stop(downCtx, compose.StopOptions{
		Files:    files,
		Services: inv.Services,
		Profiles: inv.Profiles,
		Timeout:  30 * time.Second,
	}); err != nil {
		printer.Error("Failed to stop stacks: %v", err)
		return err
	}
	if err := client.Remove(downCtx, compose.RemoveOptions{
		Files:    files,
		Services: inv.Services,
		Profiles: inv.Profiles,
	}); err != nil {
		printer.Error("Failed to remove stopped containers: %v", err)
		return err
	}

	// Phase 2: Up
	printer.Header("Starting Stacks")
	for _, s := range stacks {
		printer.Info("  • %s: %s", printer.Bold(s.Name), s.Description)
	}
	fmt.Println()

	upCtx, upCancel := context.WithTimeout(cmd.Context(), timeout)
	defer upCancel()

	err = client.Up(upCtx, compose.UpOptions{
		Files:    files,
		Services: inv.Services,
		Profiles: inv.Profiles,
		Detach:   true,
		Build:    build,
		Timeout:  timeout,
	})
	if err != nil {
		printer.Error("Failed to start stacks: %v", err)

		// Diagnose partial startup -- restart previously returned the raw
		// compose error here with no diagnostics; reuse the same
		// classifyServices/buildPartialStartupError path `up` uses so a
		// restart failure is just as actionable.
		psCtx, psCancel := context.WithTimeout(cmd.Context(), 15*time.Second)
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

	if dryRun {
		printer.Success("Stacks restarted successfully (dry-run: skipping Ready-wait)")
		printer.PrintHints("restart")
		return nil
	}

	// Trustworthy success: same Ready-wait `up` uses (internal/health),
	// so `restart` doesn't report success until every target service is
	// actually usable either.
	printer.Header("Waiting for Services to Become Ready")
	waitTimeout := maxStartupTimeout(stacks)
	result, waitErr := waitForReady(cmd.Context(), printer, client, files, stacks, waitTimeout)
	if waitErr != nil {
		return waitErr
	}
	if cliErr := renderReadyFailure(cmd.Context(), printer, files, stacks, result); cliErr != nil {
		return cliErr
	}

	printer.Success("Stacks restarted successfully — all %d services Ready", len(result.States))
	printer.PrintHints("restart")
	return nil
}
