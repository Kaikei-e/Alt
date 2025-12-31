package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/alt-project/altctl/internal/migrate"
	"github.com/alt-project/altctl/internal/output"
)

var migrateBackupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Create full backup of all persistent volumes",
	Long: `Create a full backup of all persistent volumes for migration.

The backup includes:
  - PostgreSQL databases: dumped using pg_dump (custom format)
  - Other volumes: archived using tar.gz

Backups are stored in timestamped directories (e.g., ./backups/20251231_120000/)
with a manifest.json file containing checksums for verification.

IMPORTANT: For data consistency, it's recommended to stop containers before backup.
Use --force to backup while containers are running (may cause inconsistent data).

Examples:
  altctl migrate backup                      # Backup to ./backups/
  altctl migrate backup --output /mnt/bak    # Custom output directory
  altctl migrate backup --force              # Backup with running containers`,
	RunE: runMigrateBackup,
}

func init() {
	migrateCmd.AddCommand(migrateBackupCmd)

	migrateBackupCmd.Flags().StringP("output", "o", "./backups", "output directory for backups")
	migrateBackupCmd.Flags().BoolP("force", "f", false, "backup even if containers are running")
}

func runMigrateBackup(cmd *cobra.Command, args []string) error {
	printer := output.NewPrinter(cfg.Output.Colors)

	outputDir, _ := cmd.Flags().GetString("output")
	force, _ := cmd.Flags().GetBool("force")

	// Get absolute path
	absOutput, err := filepath.Abs(outputDir)
	if err != nil {
		return fmt.Errorf("invalid output path: %w", err)
	}

	printer.Header("Creating Backup")
	printer.Info("Output directory: %s", absOutput)

	// Create migrator
	migrator := migrate.NewMigrator(
		getComposeDir(),
		"alt", // project name prefix for Docker volumes
		logger,
		dryRun,
	)

	// Run backup
	manifest, err := migrator.Backup(cmd.Context(), migrate.BackupOptions{
		OutputDir:     absOutput,
		Force:         force,
		AltctlVersion: version,
	})

	if err != nil {
		printer.Error("Backup failed: %v", err)
		return err
	}

	// Print summary
	fmt.Println()
	printer.Success("Backup complete!")
	fmt.Println()

	printer.Info("Volumes backed up: %d", len(manifest.Volumes))

	var totalSize int64
	for _, v := range manifest.Volumes {
		totalSize += v.Size
		printer.Info("  • %-30s %10s  %s",
			v.Name,
			migrate.FormatSize(v.Size),
			v.TypeString,
		)
	}

	fmt.Println()
	printer.Info("Total size: %s", migrate.FormatSize(totalSize))
	printer.Info("Manifest checksum: %s", manifest.Checksum)

	return nil
}
