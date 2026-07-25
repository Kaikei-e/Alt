package cmd

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/alt-project/altctl/internal/compose"
	"github.com/alt-project/altctl/internal/health"
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
	upCmd.Flags().BoolP("detach", "d", false, "start stacks without waiting for services to become Ready (fire-and-forget)")
	upCmd.Flags().Bool("no-deps", false, "don't start dependent stacks")
	upCmd.Flags().Bool("all", false, "start all stacks including optional ones")
	upCmd.Flags().Duration("timeout", 5*time.Minute, "timeout for container startup")
	upCmd.Flags().Bool("remove-orphans", false, "remove orphan containers")
	upCmd.Flags().String("progress", "auto", "set type of progress output (auto, tty, plain, quiet) (implies --build)")
}

func runUp(cmd *cobra.Command, args []string) error {
	printer := newPrinter()
	registry, err := loadRegistry()
	if err != nil {
		return err
	}
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
	detachFlag, _ := cmd.Flags().GetBool("detach")
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

		buildCtx, buildCancel := context.WithTimeout(cmd.Context(), 30*time.Minute)
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
	ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
	defer cancel()

	err = client.Up(ctx, compose.UpOptions{
		Files: files,
		// Always ask compose for -d: we need control back immediately so
		// we can poll `docker compose ps` ourselves below. --detach (the
		// CLI flag) controls whether *we* then wait for Ready, not whether
		// compose itself backgrounds the containers.
		Detach:        true,
		Build:         build,
		NoDeps:        false, // We've already resolved deps
		Timeout:       timeout,
		RemoveOrphans: removeOrphans,
	})

	if err != nil {
		printer.Error("Failed to start stacks: %v", err)

		// Diagnose partial startup
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

	if detachFlag {
		printer.Success("Stacks started (detached) — not verified Ready")
		printer.PrintHints("up")
		return nil
	}

	if dryRun {
		printer.Success("Stacks started successfully (dry-run: skipping Ready-wait)")
		printer.PrintHints("up")
		return nil
	}

	// Trustworthy success: don't report "started" until every target
	// service is actually Ready (see internal/health). Timeout is the max
	// startup_timeout declared across the resolved stacks, not the
	// --timeout flag above (which only bounds the `docker compose up`
	// invocation itself).
	printer.Header("Waiting for Services to Become Ready")
	waitTimeout := maxStartupTimeout(stacks)
	result, waitErr := waitForReady(cmd.Context(), printer, client, files, stacks, waitTimeout)
	if waitErr != nil {
		return waitErr
	}
	if cliErr := renderReadyFailure(cmd.Context(), printer, files, stacks, result); cliErr != nil {
		return cliErr
	}

	printer.Success("Stacks started successfully — all %d services Ready", len(result.States))
	printer.PrintHints("up")
	return nil
}

// completeStackNames provides shell completion for stack names
func completeStackNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	registry, err := loadRegistry()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
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

// --- Ready-wait: shared by `up` and `restart` so neither reports success
// until every target service is actually usable (see internal/health and
// altctl/CLAUDE.md's "trustworthy success" design). ---

// maxStartupTimeout returns the largest per-stack startup timeout across
// stacks (stack.Stack.GetTimeout, which itself falls back to a 5-minute
// default per stack). This -- not the `--timeout` flag, which only bounds
// the `docker compose up` invocation -- is the deadline the Ready-wait poll
// loop is given, so e.g. resolving the "ai" stack (1200s per .altctl.yaml)
// doesn't get cut off by a shorter default.
func maxStartupTimeout(stacks []*stack.Stack) time.Duration {
	longest := 5 * time.Minute
	for _, s := range stacks {
		if t := s.GetTimeout(); t > longest {
			longest = t
		}
	}
	return longest
}

// waitForReady polls until every service across stacks is Ready, rendering
// live progress via printer (a no-op in --quiet mode). It returns the
// terminal health.Result; a non-nil error means the wait itself could not
// complete cleanly (ctx cancelled -- e.g. Ctrl-C -- or `docker compose ps`
// itself failed), as opposed to services simply not being Ready yet, which
// is reported via Result.TimedOut/Result.States for the caller to render
// with renderReadyFailure.
func waitForReady(ctx context.Context, printer *output.Printer, client *compose.Client, files []string, stacks []*stack.Stack, timeout time.Duration) (*health.Result, error) {
	var targets []health.Target
	for _, s := range stacks {
		for _, svc := range s.Services {
			targets = append(targets, health.Target{Service: svc, Stack: s.Name})
		}
	}
	if len(targets) == 0 {
		return &health.Result{Ready: true}, nil
	}

	poller := func(pollCtx context.Context) ([]health.ServiceStatus, error) {
		statuses, err := client.PS(pollCtx, files)
		if err != nil {
			return nil, err
		}
		out := make([]health.ServiceStatus, len(statuses))
		for i, s := range statuses {
			out[i] = health.ServiceStatus{Name: s.Name, State: s.State, Health: s.Health, ExitCode: s.ExitCode}
		}
		return out, nil
	}

	waiter := health.NewWaiter(poller)
	total := len(targets)

	result, err := waiter.WaitReady(ctx, targets, health.Options{
		Timeout:      timeout,
		PollInterval: 2 * time.Second,
		OnProgress: func(states []health.State) {
			printReadyProgress(printer, states, total)
		},
	})
	if err != nil {
		return &result, err
	}
	return &result, nil
}

// printReadyProgress renders one line of live per-service progress, e.g.
// "12/17 Ready — waiting: alt-backend (starting), rerank-local (health: starting)".
// No-op in --quiet mode (printer.Info already suppresses itself there).
func printReadyProgress(printer *output.Printer, states []health.State, total int) {
	ready := 0
	var waiting []string
	for _, s := range states {
		if s.Ready {
			ready++
		} else {
			waiting = append(waiting, fmt.Sprintf("%s (%s)", s.Service, s.Reason))
		}
	}
	if len(waiting) == 0 {
		printer.Info("%d/%d Ready", ready, total)
		return
	}
	printer.Info("%d/%d Ready — waiting: %s", ready, total, strings.Join(waiting, ", "))
}

// diagnosticFromStates adapts a health.Result's terminal States into the
// same serviceDiag shape classifyServices produces from `docker compose ps`
// statuses, so a not-Ready wait outcome renders through the identical
// printDiagnostic table as a hard compose failure.
func diagnosticFromStates(stacks []*stack.Stack, states []health.State) serviceDiag {
	expected := make(map[string]string)
	for _, s := range stacks {
		for _, svc := range s.Services {
			expected[svc] = s.Name
		}
	}

	var diag serviceDiag
	diag.expected = expected
	for _, st := range states {
		switch {
		case st.Ready:
			diag.running = append(diag.running, st.Service)
		case st.Reason == "missing":
			diag.missing = append(diag.missing, st.Service)
		default:
			diag.unhealthy = append(diag.unhealthy, st.Service)
		}
	}
	sort.Strings(diag.running)
	sort.Strings(diag.unhealthy)
	sort.Strings(diag.missing)
	return diag
}

// renderReadyFailure prints the classifyServices-style diagnostic table for
// a not-Ready health.Result plus a captured log tail for every not-Ready
// service, and returns the CLIError the caller's RunE should return so
// main.go exits with the right code: output.ExitTimeout when WaitReady
// itself timed out (Critical Rule: exit code 5 must actually be returned on
// wait timeout), output.ExitComposeError otherwise (e.g. a one-shot
// migrator that exited non-zero, or a service compose never reported at
// all). Returns nil when result is nil or already Ready -- nothing to
// report.
func renderReadyFailure(ctx context.Context, printer *output.Printer, files []string, stacks []*stack.Stack, result *health.Result) *output.CLIError {
	if result == nil || result.Ready {
		return nil
	}

	diag := diagnosticFromStates(stacks, result.States)
	fmt.Println()
	printDiagnostic(printer, diag)

	var notReady []string
	for _, s := range result.States {
		if !s.Ready {
			notReady = append(notReady, s.Service)
		}
	}

	if len(notReady) > 0 {
		logs := captureFailureLogs(ctx, files, notReady)
		printer.Header("Recent Logs (not-Ready services)")
		for _, svc := range notReady {
			printer.Info("--- %s ---", svc)
			text := strings.TrimRight(logs[svc], "\n")
			if text == "" {
				text = "(no log output captured)"
			}
			printer.Print("%s", text)
		}
	}

	exitCode := output.ExitComposeError
	summary := fmt.Sprintf("%d of %d services not Ready", len(notReady), len(result.States))
	if result.TimedOut {
		exitCode = output.ExitTimeout
		summary = fmt.Sprintf("timed out waiting for %d of %d services to become Ready", len(notReady), len(result.States))
	}

	return &output.CLIError{
		Summary:    summary,
		Suggestion: "Run 'altctl status' to see current state, or 'altctl logs <service>' to follow logs",
		ExitCode:   exitCode,
	}
}

// captureFailureLogs runs `docker compose logs --tail 20 --no-color <svc>`
// for each not-Ready service, best-effort: a capture failure for one
// service (e.g. the container was removed) is folded into that service's
// text rather than aborting diagnostics for the rest. In --dry-run mode
// (or when dryRun is set by tests) no real docker invocation happens.
func captureFailureLogs(ctx context.Context, files []string, services []string) map[string]string {
	logs := make(map[string]string, len(services))

	if dryRun {
		for _, svc := range services {
			logs[svc] = fmt.Sprintf("[dry-run] docker compose logs --tail 20 --no-color %s", svc)
		}
		return logs
	}

	composeDir := getComposeDir()
	root := getProjectRoot()

	for _, svc := range services {
		args := []string{"compose"}
		for _, f := range files {
			args = append(args, "-f", filepath.Join(composeDir, f))
		}
		args = append(args, "logs", "--tail", "20", "--no-color", svc)

		logCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		c := exec.CommandContext(logCtx, "docker", args...)
		c.Dir = root
		out, err := c.CombinedOutput()
		cancel()

		if err != nil && len(out) == 0 {
			logs[svc] = fmt.Sprintf("(failed to capture logs for %s: %v)", svc, err)
			continue
		}
		logs[svc] = string(out)
	}
	return logs
}
