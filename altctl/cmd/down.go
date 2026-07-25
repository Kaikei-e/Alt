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

If no stacks are specified, stops all running stacks (the full,
aggregate-file teardown -- dev/frontend-dev/load-test containers, if
running, are NOT covered by this: they sit outside compose/compose.yaml's
include: graph by design, so stop them explicitly, e.g. 'altctl down dev').

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
	downCmd.Flags().Bool("remove-orphans", false, "remove orphan containers (full teardown only -- no effect when stopping specific stacks)")
	downCmd.Flags().Bool("with-deps", false, "also stop stacks that depend on the specified stacks")
	downCmd.Flags().Duration("timeout", 30*time.Second, "timeout for container shutdown")
}

func runDown(cmd *cobra.Command, args []string) error {
	printer := newPrinter()
	registry, err := loadRegistry()
	if err != nil {
		return err
	}

	volumes, _ := cmd.Flags().GetBool("volumes")
	removeOrphans, _ := cmd.Flags().GetBool("remove-orphans")
	timeout, _ := cmd.Flags().GetDuration("timeout")
	withDeps, _ := cmd.Flags().GetBool("with-deps")

	client := newComposeClient()

	if len(args) == 0 {
		return runFullDown(cmd, printer, client, volumes, removeOrphans, timeout)
	}
	return runScopedDown(cmd, printer, registry, client, args, withDeps, volumes, removeOrphans, timeout)
}

// runFullDown handles bare `altctl down` (no stack args): the full,
// unscoped teardown. This uses compose/compose.yaml (the aggregate) alone
// as -f, matching what a human running `docker compose -f
// compose/compose.yaml down` directly would do -- see
// cmd/compose_target.go's buildStackInvocation doc comment for why the old
// behavior (every stack's own ComposeFile combined together) was itself
// broken: e.g. core.yaml and dev.yaml both redeclare alt-frontend-sv with
// conflicting resource limits, so combining them fails compose project
// validation regardless of this fix. dev/frontend-dev/load-test are
// intentionally left out here (they sit outside the aggregate's include:
// graph); tear them down explicitly via `altctl down dev` etc.
func runFullDown(cmd *cobra.Command, printer *output.Printer, client *compose.Client, volumes, removeOrphans bool, timeout time.Duration) error {
	printer.Header("Stopping All Stacks")
	printer.Info("  • %s", printer.Bold("compose/compose.yaml (every aggregate-covered stack)"))
	printer.Warning("dev/frontend-dev/load-test are not covered by a bare 'altctl down' -- stop them explicitly, e.g. 'altctl down dev'")
	fmt.Println()

	ctx, cancel := context.WithTimeout(cmd.Context(), timeout+30*time.Second)
	defer cancel()

	err := client.Down(ctx, compose.DownOptions{
		Files:         []string{stack.AggregateComposeFile},
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
	printer.PrintHints("down")
	return nil
}

// runScopedDown handles `altctl down <stack...>`: stop + remove just the
// named services instead of a plain `docker compose down [SERVICES]`.
// `down [SERVICES]` scopes containers/networks to the named services, but
// NOT volumes -- `-v` still targets every named volume declared anywhere
// across the -f files, not just the ones the named services actually own,
// so a stack-scoped `down --volumes` could silently delete unrelated
// stacks' shared volumes. `stop` + `rm -f -v` scope both containers and
// anonymous volumes cleanly to just the named services; --volumes here
// therefore only removes those services' own anonymous volumes, never
// project-wide named/shared volumes (documented in altctl/CLAUDE.md -- use
// a full, unscoped `altctl down --volumes` for that).
func runScopedDown(cmd *cobra.Command, printer *output.Printer, registry *stack.Registry, client *compose.Client, args []string, withDeps, volumes, removeOrphans bool, timeout time.Duration) error {
	resolver := stack.NewDependencyResolver(registry)

	var stacks []*stack.Stack
	var err error
	if withDeps {
		// Reverse dependency order: dependents first, then dependencies.
		stacks, err = resolver.ResolveWithDependents(args)
		if err != nil {
			return &output.CLIError{
				Summary:    "failed resolving dependencies",
				Detail:     err.Error(),
				Suggestion: "Check stack definitions with 'altctl list --deps'",
				ExitCode:   output.ExitUsageError,
			}
		}
	} else {
		for _, name := range args {
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
	}

	inv := buildStackInvocation(stacks)
	if len(inv.Services) == 0 {
		printer.Warning("No services to stop")
		return nil
	}

	printer.Header("Stopping Stacks")
	for _, s := range stacks {
		printer.Info("  • %s", printer.Bold(s.Name))
	}
	if removeOrphans {
		printer.Warning("--remove-orphans has no effect when stopping specific stacks (it only applies to a full 'altctl down' with no arguments)")
	}
	fmt.Println()

	ctx, cancel := context.WithTimeout(cmd.Context(), timeout+30*time.Second)
	defer cancel()

	if err := client.Stop(ctx, compose.StopOptions{
		Files:    inv.Files,
		Services: inv.Services,
		Profiles: inv.Profiles,
		Timeout:  timeout,
	}); err != nil {
		printer.Error("Failed to stop stacks: %v", err)
		return err
	}

	if err := client.Remove(ctx, compose.RemoveOptions{
		Files:    inv.Files,
		Services: inv.Services,
		Profiles: inv.Profiles,
		Volumes:  volumes,
	}); err != nil {
		printer.Error("Failed to remove stopped containers: %v", err)
		return err
	}

	if volumes {
		printer.Success("Services stopped and their anonymous volumes removed (named/shared volumes untouched — run 'altctl down' with no args for a full teardown)")
	} else {
		printer.Success("Services stopped successfully")
	}
	printer.PrintHints("down")
	return nil
}
