package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/alt-project/altctl/internal/output"
	"github.com/alt-project/altctl/internal/setup"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize the Alt platform environment",
	Long: `Initialize the environment for running the Alt platform.

This command performs the following steps:
  1. Check prerequisites (Docker, Docker Compose)
  2. Create .env from .env.example
  3. Generate secret files in secrets/
  4. Regenerate Atlas migration checksums
  5. Validate the setup

After initialization, run 'altctl up' to start the platform.

The command is idempotent — existing files are not overwritten unless --force is used.

Examples:
  altctl init                # Initialize environment
  altctl init --force        # Overwrite existing .env and secrets
  altctl init --skip-secrets # Skip secret generation (external management)
  altctl init --dry-run      # Show what would be done`,
	Args: cobra.NoArgs,
	RunE: runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)

	initCmd.Flags().Bool("force", false, "overwrite existing .env and secret files")
	initCmd.Flags().Bool("skip-secrets", false, "skip secret file generation")
}

func runInit(cmd *cobra.Command, args []string) error {
	printer := newPrinter()
	root := getProjectRoot()
	force, _ := cmd.Flags().GetBool("force")
	skipSecrets, _ := cmd.Flags().GetBool("skip-secrets")

	// Phase 1: Prerequisites
	printer.Header("Prerequisites")
	checks := setup.CheckPrerequisites()
	allOK := true
	for _, c := range checks {
		if c.OK {
			if c.Version != "" {
				printer.Success("%s v%s", c.Name, c.Version)
			} else {
				printer.Success("%s %s", c.Name, c.Detail)
			}
		} else {
			printer.Error("%s: %s", c.Name, c.Detail)
			allOK = false
		}
	}

	if !allOK {
		return &output.CLIError{
			Summary:    "prerequisites not met",
			Suggestion: "Install Docker and ensure the daemon is running",
			ExitCode:   output.ExitConfigError,
		}
	}
	fmt.Println()

	// Phase 2: Environment file
	printer.Header("Environment File")
	if dryRun {
		printer.Info("[dry-run] Would copy .env.example → .env")
	} else {
		created, err := setup.CreateEnvFile(root, force)
		if err != nil {
			return &output.CLIError{
				Summary:    "failed to create .env",
				Detail:     err.Error(),
				Suggestion: "Ensure .env.example exists in project root",
				ExitCode:   output.ExitConfigError,
			}
		}
		if created {
			printer.Success("Created .env from .env.example")
		} else {
			printer.Info("Skipped .env (already exists, use --force to overwrite)")
		}
	}
	fmt.Println()

	// Phase 3: Secrets
	if skipSecrets {
		printer.Header("Secrets")
		printer.Info("Skipped (--skip-secrets)")
		fmt.Println()
	} else {
		printer.Header("Secrets")
		secretsDir := filepath.Join(root, "secrets")
		specs := setup.DefaultSecretSpecs()

		if dryRun {
			autoCount := 0
			optionalCount := 0
			for _, s := range specs {
				if s.AutoGenerate {
					autoCount++
				} else {
					optionalCount++
				}
			}
			printer.Info("[dry-run] Would generate %d secret files in secrets/", autoCount)
			printer.Info("[dry-run] Would create %d optional placeholder files", optionalCount)
		} else {
			result, err := setup.GenerateSecrets(secretsDir, specs, force)
			if err != nil {
				return &output.CLIError{
					Summary:    "failed to generate secrets",
					Detail:     err.Error(),
					Suggestion: "Check permissions on secrets/ directory",
					ExitCode:   output.ExitConfigError,
				}
			}

			if len(result.Created) > 0 {
				printer.Success("Created %d secret files in secrets/", len(result.Created))
			}
			if len(result.Skipped) > 0 {
				printer.Info("Skipped %d existing files (use --force to overwrite)", len(result.Skipped))
			}

			// Warn about optional user-provided secrets
			for _, spec := range specs {
				if !spec.AutoGenerate {
					printer.Warning("Optional: secrets/%s (%s)", spec.Filename, spec.Description)
				}
			}
		}
		fmt.Println()
	}

	// Phase 4: Atlas migration checksums
	printer.Header("Migration Checksums")
	migrationDirs := setup.DefaultMigrationDirs()
	if dryRun {
		printer.Info("[dry-run] Would regenerate atlas.sum for %d migration dirs", len(migrationDirs))
	} else {
		for _, dir := range migrationDirs {
			dirPath := filepath.Join(root, dir.Path)
			if _, err := os.Stat(dirPath); os.IsNotExist(err) {
				printer.Info("Skipped %s (directory not found)", dir.Name)
				continue
			}
			if err := setup.RegenerateAtlasHash(root, dir); err != nil {
				printer.Warning("Failed to hash %s: %v", dir.Name, err)
			} else {
				printer.Success("Regenerated atlas.sum for %s", dir.Name)
			}
		}
	}
	fmt.Println()

	// Phase 5: Validation
	printer.Header("Validation")
	secretsDir := filepath.Join(root, "secrets")
	specs := setup.DefaultSecretSpecs()
	missing := 0
	for _, spec := range specs {
		if spec.AutoGenerate {
			path := filepath.Join(secretsDir, spec.Filename)
			if _, err := os.Stat(path); err != nil {
				printer.Error("Missing: secrets/%s", spec.Filename)
				missing++
			}
		}
	}

	envPath := filepath.Join(root, ".env")
	if _, err := os.Stat(envPath); err != nil {
		printer.Error("Missing: .env")
		missing++
	}

	if missing > 0 && !dryRun && !skipSecrets {
		return &output.CLIError{
			Summary:    fmt.Sprintf("%d required files missing", missing),
			Suggestion: "Run 'altctl init --force' to regenerate",
			ExitCode:   output.ExitConfigError,
		}
	}

	if !dryRun {
		printer.Success("All required files present")
	} else {
		printer.Info("[dry-run] Validation skipped")
	}
	fmt.Println()

	// Phase 6: Next steps
	printer.Success("Initialization complete")
	fmt.Println()
	printer.Info("Next steps:")
	printer.Info("  altctl up          # Start default stacks (db, auth, core, workers)")
	printer.Info("  altctl up --all    # Start all stacks")
	printer.Info("  altctl status      # Check service status")

	return nil
}
