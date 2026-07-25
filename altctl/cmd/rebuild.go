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

var rebuildCmd = &cobra.Command{
	Use:   "rebuild <service|stack> [more...]",
	Short: "Rebuild images and force-recreate specific services or stacks",
	Long: `Rebuild images and force-recreate containers for the targeted services or
stacks only -- this is the fix for the repo's #1 pitfall: "changed Go/Rust/TS
code but forgot --build; the old binary keeps running silently" (root
CLAUDE.md Critical Rule 3).

Unlike 'altctl up --build', which rebuilds and (re)starts an entire stack
plus its dependencies, 'altctl rebuild' touches only the services you name:

  docker compose build <svcs>
  docker compose up -d --no-deps --force-recreate <svcs>

--force-recreate matters: 'docker compose up' alone will not recreate a
container whose image tag hasn't changed, so a freshly rebuilt image with
the same tag would otherwise leave the stale container running untouched
(documented failure pattern ADR-000761 / PM-2026-005). rebuild always forces
recreation so this can't happen silently.

Each argument is resolved as either a stack name (expands to every service
in that stack) or a service name (via the derived stack registry); the two
can be mixed freely and duplicates across arguments are deduped. One-shot
targets (migrators/init jobs) are valid rebuild targets too -- Ready for
them means "exited 0", same rule 'up'/'restart' use.

Examples:
  altctl rebuild alt-backend             # Rebuild + recreate one service
  altctl rebuild core                    # Rebuild + recreate every service in the core stack
  altctl rebuild alt-backend migrate     # Multiple targets (services and/or stacks)
  altctl rebuild core --no-cache         # Rebuild without the Docker build cache
  altctl rebuild core --detach           # Rebuild + recreate, skip the Ready-wait`,
	Args:              cobra.MinimumNArgs(1),
	ValidArgsFunction: completeServiceAndStackNames,
	RunE:              runRebuild,
}

func init() {
	rootCmd.AddCommand(rebuildCmd)

	rebuildCmd.Flags().Bool("no-cache", false, "build without cache")
	rebuildCmd.Flags().BoolP("detach", "d", false, "rebuild and recreate without waiting for services to become Ready (fire-and-forget)")
	rebuildCmd.Flags().Duration("timeout", 5*time.Minute, "timeout for the `docker compose up` invocation itself")
}

// rebuildTarget is the resolved set of services `rebuild` should operate
// on, grouped by owning stack.
type rebuildTarget struct {
	// services is the deduped, order-of-first-mention list of every service
	// name to pass to `docker compose build`/`up` (and to internal/health).
	services []string
	// stacks holds one synthetic *stack.Stack per owning stack, with
	// Services narrowed to only the subset actually targeted (not the
	// whole compose file). This lets rebuild reuse up.go's Stack-shaped
	// helpers (maxStartupTimeout, waitForReady, classifyServices,
	// renderReadyFailure, ...) unmodified: they only ever look at
	// Name/ComposeFile/Services/Timeout, all of which are copied from the
	// real stack here.
	stacks []*stack.Stack
	// files is the -f argument list needed to reach every targeted service,
	// computed via buildStackInvocation against the real (unnarrowed)
	// owning stacks -- see its doc comment for why this can't simply be
	// "each owning stack's own ComposeFile": even a single stack's own
	// file can fail compose project validation alone (e.g. `-f core.yaml`
	// alone: "migrate depends on undefined service db", since migrate's
	// depends_on reaches outside core.yaml's own include: closure).
	files []string
	// profiles is the deduped `--profile` list needed for any targeted
	// stack that declares one (e.g. rebuilding a perf-stack service needs
	// --profile perf).
	profiles []string
}

// resolveRebuildTargets resolves each arg to either a stack (expands to all
// its services) or a single service (via registry.FindByService), dedupes
// services across args, and groups the result by owning stack. Returns an
// *output.CLIError naming close matches (see closestMatches) for any arg
// that is neither a known stack nor a known service, and for a service name
// that resolves ambiguously (see stack.Registry.FindByService).
func resolveRebuildTargets(registry *stack.Registry, args []string) (*rebuildTarget, error) {
	seen := make(map[string]bool)
	byStack := make(map[string]*stack.Stack)    // synthetic, narrowed-to-target stacks (unchanged shape for waitForReady/etc.)
	realOwners := make(map[string]*stack.Stack) // real (unnarrowed) owning stacks, for file/profile resolution
	var stackOrder []string
	var services []string

	addService := func(svc string, owner *stack.Stack) {
		if seen[svc] {
			return
		}
		seen[svc] = true
		services = append(services, svc)

		synth, ok := byStack[owner.Name]
		if !ok {
			synth = &stack.Stack{
				Name:        owner.Name,
				ComposeFile: owner.ComposeFile,
				Timeout:     owner.Timeout,
			}
			byStack[owner.Name] = synth
			realOwners[owner.Name] = owner
			stackOrder = append(stackOrder, owner.Name)
		}
		synth.Services = append(synth.Services, svc)
	}

	for _, arg := range args {
		if s, ok := registry.Get(arg); ok {
			for _, svc := range s.Services {
				addService(svc, s)
			}
			continue
		}
		s, ferr := registry.FindByService(arg)
		if ferr != nil {
			return nil, ambiguousRebuildTargetError(arg, ferr)
		}
		if s != nil {
			addService(arg, s)
			continue
		}
		return nil, unknownRebuildTargetError(registry, arg)
	}

	rt := &rebuildTarget{services: services}
	var owners []*stack.Stack
	for _, name := range stackOrder {
		rt.stacks = append(rt.stacks, byStack[name])
		owners = append(owners, realOwners[name])
	}
	inv := buildStackInvocation(owners)
	rt.files = inv.Files
	rt.profiles = inv.Profiles
	return rt, nil
}

// ambiguousRebuildTargetError wraps a stack.Registry.FindByService
// disambiguation error (a service declared in more than one stack, unable
// to be resolved deterministically even after preferring an
// aggregate-covered stack -- see FindByService's doc comment) into a
// *output.CLIError with the right exit code for rebuild's usage-error path.
func ambiguousRebuildTargetError(arg string, cause error) *output.CLIError {
	return &output.CLIError{
		Summary:    fmt.Sprintf("cannot resolve %q to a single stack", arg),
		Detail:     cause.Error(),
		Suggestion: "Pass the owning stack name explicitly instead of the bare service name",
		ExitCode:   output.ExitUsageError,
	}
}

// unknownRebuildTargetError builds a helpful "unknown service or stack"
// error naming close matches (typo suggestions) drawn from every stack and
// service name the registry knows about.
func unknownRebuildTargetError(registry *stack.Registry, arg string) *output.CLIError {
	var candidates []string
	candidates = append(candidates, registry.Names()...)
	for _, s := range registry.All() {
		candidates = append(candidates, s.Services...)
	}

	suggestion := "Run 'altctl list --services' to see available stacks and services"
	if matches := closestMatches(arg, candidates, 3); len(matches) > 0 {
		suggestion = fmt.Sprintf("Did you mean: %s? %s", strings.Join(matches, ", "), suggestion)
	}

	return &output.CLIError{
		Summary:    fmt.Sprintf("unknown service or stack: %s", arg),
		Suggestion: suggestion,
		ExitCode:   output.ExitUsageError,
	}
}

// closestMatches ranks candidates by Levenshtein distance to name and
// returns up to max of the closest ones, for "did you mean" suggestions on
// an unknown service/stack argument. A candidate is only offered when it's
// close enough to plausibly be a typo -- otherwise the caller falls back to
// pointing at 'altctl list --services' instead of a noisy guess.
func closestMatches(name string, candidates []string, max int) []string {
	type scored struct {
		name string
		dist int
	}

	seen := make(map[string]bool)
	var ranked []scored
	for _, c := range candidates {
		if seen[c] {
			continue
		}
		seen[c] = true

		d := levenshtein(name, c)
		threshold := 3
		if half := len(c) / 2; half > 0 && half < threshold {
			threshold = half
		}
		if d <= threshold {
			ranked = append(ranked, scored{name: c, dist: d})
		}
	}

	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].dist != ranked[j].dist {
			return ranked[i].dist < ranked[j].dist
		}
		return ranked[i].name < ranked[j].name
	})

	if len(ranked) > max {
		ranked = ranked[:max]
	}
	out := make([]string, len(ranked))
	for i, r := range ranked {
		out[i] = r.name
	}
	return out
}

// levenshtein computes the classic edit distance between a and b (single
// character insert/delete/substitute cost 1), operating rune-wise so it
// stays correct for any UTF-8 input even though stack/service names are
// ASCII in practice.
func levenshtein(a, b string) int {
	ar, br := []rune(a), []rune(b)
	la, lb := len(ar), len(br)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}

	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if ar[i-1] == br[j-1] {
				cost = 0
			}
			del := prev[j] + 1
			ins := curr[j-1] + 1
			sub := prev[j-1] + cost
			m := del
			if ins < m {
				m = ins
			}
			if sub < m {
				m = sub
			}
			curr[j] = m
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

func runRebuild(cmd *cobra.Command, args []string) error {
	printer := newPrinter()
	registry, err := loadRegistry()
	if err != nil {
		return err
	}

	target, err := resolveRebuildTargets(registry, args)
	if err != nil {
		return err
	}

	if len(target.files) == 0 {
		printer.Warning("No compose files to rebuild")
		return nil
	}

	printer.Header("Rebuilding Services")
	for _, svc := range target.services {
		printer.Info("  • %s", printer.Bold(svc))
	}
	fmt.Println()

	noCache, _ := cmd.Flags().GetBool("no-cache")
	detachFlag, _ := cmd.Flags().GetBool("detach")
	timeout, _ := cmd.Flags().GetDuration("timeout")

	client := newComposeClient()

	// Build phase: only the targeted services' images, not everything else
	// defined in the same compose file(s).
	printer.Header("Building")
	buildCtx, buildCancel := context.WithTimeout(cmd.Context(), 30*time.Minute)
	defer buildCancel()

	if err := client.Build(buildCtx, compose.BuildOptions{
		Files:    target.files,
		Services: target.services,
		Profiles: target.profiles,
		NoCache:  noCache,
	}); err != nil {
		printer.Error("Failed to build: %v", err)
		return err
	}

	// Recreate phase: --no-deps because rebuild must only touch the named
	// services, not pull in their dependents/dependencies; --force-recreate
	// because plain `docker compose up` won't recreate a container whose
	// image tag is unchanged, so a freshly rebuilt image would otherwise
	// leave the stale container running -- exactly the "forgot --build"
	// failure this command exists to prevent (ADR-000761 / PM-2026-005).
	printer.Header("Recreating")
	upCtx, upCancel := context.WithTimeout(cmd.Context(), timeout)
	defer upCancel()

	err = client.Up(upCtx, compose.UpOptions{
		Files:         target.files,
		Services:      target.services,
		Profiles:      target.profiles,
		Detach:        true,
		NoDeps:        true,
		ForceRecreate: true,
		Timeout:       timeout,
	})
	if err != nil {
		printer.Error("Failed to recreate services: %v", err)

		// Diagnose partial rebuild using the same classify/render helpers
		// up.go uses -- target.stacks are already narrowed to just the
		// rebuilt services, so the diagnostic table only reports on what
		// this invocation actually touched.
		psCtx, psCancel := context.WithTimeout(cmd.Context(), 15*time.Second)
		defer psCancel()
		statuses, psErr := client.PS(psCtx, target.files)
		if psErr == nil {
			diag := classifyServices(target.stacks, statuses)
			if cliErr := buildPartialStartupError(diag, err); cliErr != nil {
				fmt.Println()
				printDiagnostic(printer, diag)
				return cliErr
			}
		}
		return err
	}

	if detachFlag {
		printer.Success("Services rebuilt (detached) — not verified Ready")
		printer.PrintHints("rebuild")
		return nil
	}

	if dryRun {
		printer.Success("Services rebuilt successfully (dry-run: skipping Ready-wait)")
		printer.PrintHints("rebuild")
		return nil
	}

	// Trustworthy success: same Ready-wait `up`/`restart` use (see
	// internal/health), scoped to just the rebuilt services.
	printer.Header("Waiting for Services to Become Ready")
	waitTimeout := maxStartupTimeout(target.stacks)
	result, waitErr := waitForReady(cmd.Context(), printer, client, target.files, target.stacks, waitTimeout)
	if waitErr != nil {
		return waitErr
	}
	if cliErr := renderReadyFailure(cmd.Context(), printer, target.files, target.stacks, result); cliErr != nil {
		return cliErr
	}

	printer.Success("Services rebuilt successfully — all %d services Ready", len(result.States))
	printer.PrintHints("rebuild")
	return nil
}
