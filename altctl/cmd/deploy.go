package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/alt-project/altctl/internal/compose"
	"github.com/alt-project/altctl/internal/output"
	"github.com/alt-project/altctl/internal/stack"
)

var deployCmd = &cobra.Command{
	Use:   "deploy [stacks...]",
	Short: "Pull latest code and deploy stacks",
	Long: `Pull the latest code from git and deploy stacks with rebuild.

Deploy performs the following steps:
  1. git fetch + pull (fast-forward only)
  2. Build images for the target stacks
  3. Start/restart services with the new images
  4. Run smoke tests to verify deployment

If no stacks are specified, deploys the default stacks.
Dependencies are automatically resolved.

Examples:
  altctl deploy                   # Deploy default stacks
  altctl deploy core              # Deploy core stack only
  altctl deploy --no-pull         # Skip git pull, just rebuild and restart
  altctl deploy --no-smoke        # Skip smoke tests after deploy
  altctl deploy --no-cache        # Build without Docker cache`,
	Args:              cobra.ArbitraryArgs,
	ValidArgsFunction: completeStackNames,
	RunE:              runDeploy,
}

func init() {
	rootCmd.AddCommand(deployCmd)

	deployCmd.Flags().Bool("no-pull", false, "skip git pull")
	deployCmd.Flags().Bool("no-smoke", false, "skip smoke tests after deploy")
	deployCmd.Flags().Bool("no-cache", false, "build without Docker cache")
	deployCmd.Flags().Bool("pull", false, "pull base images before building")
	deployCmd.Flags().Bool("no-deps", false, "don't deploy dependent stacks")
	deployCmd.Flags().Duration("build-timeout", 30*time.Minute, "timeout for build phase")
	deployCmd.Flags().Duration("startup-timeout", 5*time.Minute, "timeout for container startup")
}

func runDeploy(cmd *cobra.Command, args []string) error {
	printer := newPrinter()
	registry := stack.NewRegistry()
	resolver := stack.NewDependencyResolver(registry)

	// Determine which stacks to deploy
	var stackNames []string
	if len(args) > 0 {
		stackNames = args
	} else {
		stackNames = cfg.Defaults.Stacks
	}

	// Resolve dependencies unless --no-deps
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
		printer.Warning("No compose files to deploy")
		return nil
	}

	// Phase 1: Git pull
	noPull, _ := cmd.Flags().GetBool("no-pull")
	if !noPull {
		printer.Header("Pulling Latest Code")
		updated, pullErr := gitPull(printer)
		if pullErr != nil {
			return &output.CLIError{
				Summary:    "git pull failed",
				Detail:     pullErr.Error(),
				Suggestion: "Check git status or use --no-pull to skip",
				ExitCode:   output.ExitGeneral,
			}
		}
		if !updated {
			printer.Info("Already up to date")
		}
		fmt.Println()
	}

	// Phase 2: Build
	printer.Header("Building Images")
	for _, s := range stacks {
		printer.Info("  • %s", printer.Bold(s.Name))
	}
	fmt.Println()

	noCache, _ := cmd.Flags().GetBool("no-cache")
	pullImages, _ := cmd.Flags().GetBool("pull")
	buildTimeout, _ := cmd.Flags().GetDuration("build-timeout")

	client := compose.NewClient(
		getProjectRoot(),
		getComposeDir(),
		logger,
		dryRun,
	)

	buildCtx, buildCancel := context.WithTimeout(context.Background(), buildTimeout)
	defer buildCancel()

	err = client.Build(buildCtx, compose.BuildOptions{
		Files:    files,
		NoCache:  noCache,
		Pull:     pullImages,
		Parallel: true,
		Progress: "auto",
	})
	if err != nil {
		printer.Error("Build failed: %v", err)
		return &output.CLIError{
			Summary:    "deployment build failed",
			Detail:     err.Error(),
			Suggestion: "Run 'altctl build' to diagnose build issues",
			ExitCode:   output.ExitComposeError,
		}
	}
	printer.Success("Images built")
	fmt.Println()

	// Phase 3: Start services
	printer.Header("Starting Services")
	startupTimeout, _ := cmd.Flags().GetDuration("startup-timeout")

	startCtx, startCancel := context.WithTimeout(context.Background(), startupTimeout)
	defer startCancel()

	err = client.Up(startCtx, compose.UpOptions{
		Files:         files,
		Detach:        true,
		Build:         false,
		Timeout:       startupTimeout,
		RemoveOrphans: false,
	})
	if err != nil {
		printer.Error("Failed to start services: %v", err)

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
	printer.Success("Services started")
	fmt.Println()

	// Phase 4: Smoke tests
	noSmoke, _ := cmd.Flags().GetBool("no-smoke")
	if !noSmoke {
		printer.Header("Running Smoke Tests")
		smokeErr := runSmokeTests(printer)
		if smokeErr != nil {
			printer.Warning("Smoke tests failed: %v", smokeErr)
			printer.Info("Services are running but may not be fully healthy")
		} else {
			printer.Success("Smoke tests passed")
		}
		fmt.Println()
	}

	printer.Success("Deployment completed successfully")
	printer.PrintHints("deploy")
	return nil
}

// gitPull fetches and pulls the latest code. Returns true if changes were pulled.
func gitPull(printer *output.Printer) (bool, error) {
	root := getProjectRoot()

	if dryRun {
		fmt.Println("[dry-run] git fetch origin main")
		fmt.Println("[dry-run] git pull --ff-only origin main")
		return true, nil
	}

	// Fetch
	fetchCmd := exec.Command("git", "fetch", "origin", "main", "--quiet")
	fetchCmd.Dir = root
	if err := fetchCmd.Run(); err != nil {
		return false, fmt.Errorf("git fetch failed: %w", err)
	}

	// Compare
	localCmd := exec.Command("git", "rev-parse", "HEAD")
	localCmd.Dir = root
	localOut, err := localCmd.Output()
	if err != nil {
		return false, fmt.Errorf("git rev-parse HEAD failed: %w", err)
	}

	remoteCmd := exec.Command("git", "rev-parse", "origin/main")
	remoteCmd.Dir = root
	remoteOut, err := remoteCmd.Output()
	if err != nil {
		return false, fmt.Errorf("git rev-parse origin/main failed: %w", err)
	}

	local := strings.TrimSpace(string(localOut))
	remote := strings.TrimSpace(string(remoteOut))

	if local == remote {
		return false, nil
	}

	printer.Info("  %s → %s", local[:8], remote[:8])

	// Pull
	pullCmd := exec.Command("git", "pull", "--ff-only", "origin", "main")
	pullCmd.Dir = root
	pullCmd.Stdout = os.Stdout
	pullCmd.Stderr = os.Stderr
	if err := pullCmd.Run(); err != nil {
		return false, fmt.Errorf("git pull --ff-only failed: %w", err)
	}

	return true, nil
}

// runSmokeTests executes the smoke test script if it exists.
func runSmokeTests(printer *output.Printer) error {
	root := getProjectRoot()
	smokeScript := filepath.Join(root, "deploy-system", "smoke-test.sh")

	if _, err := os.Stat(smokeScript); os.IsNotExist(err) {
		printer.Info("No smoke-test.sh found, skipping")
		return nil
	}

	if dryRun {
		fmt.Printf("[dry-run] bash %s\n", smokeScript)
		return nil
	}

	cmd := exec.Command("bash", smokeScript)
	cmd.Dir = root
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
