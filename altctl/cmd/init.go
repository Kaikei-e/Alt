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
		specs, err := setup.DefaultSecretSpecs()
		if err != nil {
			return &output.CLIError{
				Summary:    "failed to derive required secrets",
				Detail:     err.Error(),
				Suggestion: "Ensure compose/base.yaml exists and has a valid top-level secrets: block; run altctl from within the Alt repo checkout",
				ExitCode:   output.ExitConfigError,
			}
		}

		if dryRun {
			counts := map[setup.SecretClass]int{}
			for _, s := range specs {
				counts[s.Class]++
			}
			printer.Info("[dry-run] Would generate %d random secret files in secrets/", counts[setup.SecretClassRandom])
			printer.Info("[dry-run] Would create %d operator-provided placeholder files", counts[setup.SecretClassOperatorProvided])
			printer.Info("[dry-run] Would leave %d step-ca provisioner secrets to %s", counts[setup.SecretClassPKIProvisioner], setup.PKIBootstrapScript)
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
				printer.Info("Skipped %d existing files (--force regenerates random secrets only)", len(result.Skipped))
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
	var specs []setup.SecretSpec
	if !skipSecrets {
		var err error
		specs, err = setup.DefaultSecretSpecs()
		if err != nil {
			return &output.CLIError{
				Summary:    "failed to derive required secrets",
				Detail:     err.Error(),
				Suggestion: "Ensure compose/base.yaml exists and has a valid top-level secrets: block; run altctl from within the Alt repo checkout",
				ExitCode:   output.ExitConfigError,
			}
		}
	}
	missing := 0
	for _, spec := range specs {
		if spec.Class == setup.SecretClassRandom {
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

	pendingActions := 0
	if !dryRun {
		printer.Success("All generated files present")
		pending, err := setup.FindPendingSecrets(secretsDir, specs)
		if err != nil {
			return &output.CLIError{
				Summary:  "failed to check operator-provisioned secrets",
				Detail:   err.Error(),
				ExitCode: output.ExitConfigError,
			}
		}
		pendingActions = reportPendingSecrets(printer, pending)
	} else {
		printer.Info("[dry-run] Validation skipped")
	}
	fmt.Println()

	// Phase 6: Next steps
	if pendingActions > 0 {
		printer.Warning("Initialization complete with %d secret file(s) still needing operator action (see Validation above)", pendingActions)
	} else {
		printer.Success("Initialization complete")
	}
	fmt.Println()
	printer.Info("Next steps:")
	printer.Info("  altctl up          # Start default stacks (db, auth, core, workers)")
	printer.Info("  altctl up --all    # Start all stacks")
	printer.Info("  altctl status      # Check service status")

	return nil
}

// reportPendingSecrets prints what the operator still has to provide and
// returns how many secret files are pending.
func reportPendingSecrets(printer *output.Printer, pending setup.PendingSecrets) int {
	for _, spec := range pending.OperatorProvided {
		printer.Warning("Operator-provided: secrets/%s is empty — fill it in: %s", spec.Filename, spec.Description)
	}
	if len(pending.PKIProvisioner) > 0 {
		printer.Warning("%d step-ca provisioner secret(s) missing; altctl never generates these, the password must match a registered provisioner:", len(pending.PKIProvisioner))
		for _, spec := range pending.PKIProvisioner {
			printer.Warning("  secrets/%s", spec.Filename)
		}
		printer.Warning("Start step-ca (altctl up pki), then run: bash %s", setup.PKIBootstrapScript)
	}
	return len(pending.OperatorProvided) + len(pending.PKIProvisioner)
}
