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
	Long: `Restore all persistent volumes from a backup.

The restore process:
  1. Verifies backup integrity (checksums)
  2. Stops running containers (if --force is used)
  3. Starts database containers for PostgreSQL restore
  4. Restores PostgreSQL databases using pg_restore
  5. Restores tar-based volumes

IMPORTANT: This operation will OVERWRITE existing data. Make sure you have
a recent backup before proceeding.

Examples:
  altctl migrate restore --from ./backups/20251231_120000
  altctl migrate restore --from ./backups/20251231_120000 --force
  altctl migrate restore --from ./backups/20251231_120000 --verify`,
	RunE: runMigrateRestore,
}

func init() {
	migrateCmd.AddCommand(migrateRestoreCmd)

	migrateRestoreCmd.Flags().String("from", "", "backup directory to restore from (required)")
	migrateRestoreCmd.Flags().BoolP("force", "f", false, "stop running containers and restore")
	migrateRestoreCmd.Flags().Bool("verify", true, "verify backup integrity before restore")

	_ = migrateRestoreCmd.MarkFlagRequired("from")
}

func runMigrateRestore(cmd *cobra.Command, args []string) error {
	printer := output.NewPrinter(cfg.Output.Colors)

	backupDir, _ := cmd.Flags().GetString("from")
	force, _ := cmd.Flags().GetBool("force")
	verify, _ := cmd.Flags().GetBool("verify")

	printer.Header("Restoring from Backup")
	printer.Info("Backup directory: %s", backupDir)

	// Show backup summary
	summary, err := migrate.GetBackupSummary(backupDir)
	if err != nil {
		return fmt.Errorf("reading backup: %w", err)
	}

	fmt.Println()
	fmt.Println(summary)

	// Confirm if not force
	if !force && !dryRun {
		printer.Warning("This will OVERWRITE existing data!")
		printer.Warning("Use --force to proceed without confirmation")
		return fmt.Errorf("restore aborted: use --force to proceed")
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
	})

	if err != nil {
		printer.Error("Restore failed: %v", err)
		return err
	}

	fmt.Println()
	printer.Success("Restore complete!")
	printer.Info("You may now start the services with: altctl up")

	return nil
}
