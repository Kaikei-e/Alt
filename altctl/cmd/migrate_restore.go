package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/alt-project/altctl/internal/migrate"
	"github.com/alt-project/altctl/internal/output"
)

var migrateRestoreCmd = &cobra.Command{
	Use:   "restore",
	Short: "Restore volumes from a backup",
	Long: `Restore persistent volumes from a backup, optionally filtered by profile or volume names.

The restore process:
  1. Verifies backup integrity (checksums)
  2. Stops running containers (if --force is used)
  3. Starts database containers for PostgreSQL restore
  4. Restores PostgreSQL databases using pg_restore
  5. Restores tar-based volumes

Filtering:
  --profile db       Restore only PostgreSQL databases
  --profile essential Restore critical + data + search volumes
  --volumes x,y      Restore only the named volumes

IMPORTANT: This operation will OVERWRITE existing data. Make sure you have
a recent backup before proceeding.

Examples:
  altctl migrate restore --from ./backups/20260409_120000 --force
  altctl migrate restore --from ./backups/20260409_120000 --profile db --force
  altctl migrate restore --from ./backups/20260409_120000 --volumes db_data_17 --force`,
	RunE: runMigrateRestore,
}

func init() {
	migrateCmd.AddCommand(migrateRestoreCmd)

	migrateRestoreCmd.Flags().String("from", "", "backup directory to restore from (required)")
	migrateRestoreCmd.Flags().BoolP("force", "f", false, "stop running containers and restore")
	migrateRestoreCmd.Flags().Bool("verify", true, "verify backup integrity before restore")
	migrateRestoreCmd.Flags().String("profile", "", "restore only volumes matching this profile: db, essential, all")
	migrateRestoreCmd.Flags().StringSlice("volumes", nil, "restore only these specific volumes")

	_ = migrateRestoreCmd.MarkFlagRequired("from")
}

func runMigrateRestore(cmd *cobra.Command, args []string) error {
	printer := newPrinter()

	backupDir, _ := cmd.Flags().GetString("from")
	force, _ := cmd.Flags().GetBool("force")
	verify, _ := cmd.Flags().GetBool("verify")
	profile, _ := cmd.Flags().GetString("profile")
	volumes, _ := cmd.Flags().GetStringSlice("volumes")

	printer.Header("Restoring from Backup")
	printer.Info("Backup directory: %s", backupDir)
	if profile != "" {
		printer.Info("Profile filter: %s", profile)
	}
	if len(volumes) > 0 {
		printer.Info("Volume filter: %v", volumes)
	}

	// Show backup summary
	summary, err := migrate.GetBackupSummary(backupDir)
	if err != nil {
		return &output.CLIError{
			Summary:    "failed reading backup",
			Detail:     err.Error(),
			Suggestion: "Check the backup directory path and permissions",
			ExitCode:   output.ExitGeneral,
		}
	}

	fmt.Println()
	fmt.Println(summary)

	// Confirm if not force
	if !force && !dryRun {
		printer.Warning("This will OVERWRITE existing data!")
		printer.Warning("Use --force to proceed without confirmation")
		return &output.CLIError{
			Summary:    "restore aborted",
			Suggestion: "Use --force to proceed",
			ExitCode:   output.ExitUsageError,
		}
	}

	// Create migrator
	migrator := migrate.NewMigrator(
		getComposeDir(),
		"alt",
		logger,
		dryRun,
	)

	// Run restore
	err = migrator.Restore(cmd.Context(), migrate.RestoreOptions{
		BackupDir: backupDir,
		Force:     force,
		Verify:    verify,
		Profile:   migrate.BackupProfile(profile),
		Volumes:   volumes,
	})

	if err != nil {
		printer.Error("Restore failed: %v", err)
		return err
	}

	fmt.Println()
	printer.Success("Restore complete!")
	printer.Info("You may now start the services with: altctl up")
	printer.PrintHints("migrate restore")

	return nil
}
