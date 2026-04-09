package cmd

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/alt-project/altctl/internal/migrate"
	"github.com/alt-project/altctl/internal/output"
)

var migrateBackupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Create backup of persistent volumes",
	Long: `Create a backup of persistent volumes filtered by profile.

Profiles:
  db         PostgreSQL databases only (fastest)
  essential  Critical data + operational data + search (no metrics/models)
  all        All registered volumes (complete backup)

The backup includes:
  - PostgreSQL databases: dumped using pg_dump (custom format, parallel)
  - Other volumes: archived using tar.gz (sequential)

Backups are stored in timestamped directories (e.g., ./backups/20260409_120000/)
with a manifest.json file containing checksums for verification.

IMPORTANT: For data consistency, it's recommended to stop containers before backup.
Use --force to backup while containers are running (may cause inconsistent data).
PostgreSQL pg_dump creates consistent snapshots even against live databases.

Examples:
  altctl migrate backup                            # Essential profile (default)
  altctl migrate backup --profile db --force       # DB-only hot backup
  altctl migrate backup --profile all --force      # Full backup
  altctl migrate backup --exclude clickhouse_data  # Skip specific volumes
  altctl migrate backup --output /mnt/bak          # Custom output directory`,
	RunE: runMigrateBackup,
}

func init() {
	migrateCmd.AddCommand(migrateBackupCmd)

	migrateBackupCmd.Flags().StringP("output", "o", "./backups", "output directory for backups")
	migrateBackupCmd.Flags().BoolP("force", "f", false, "backup even if containers are running")
	migrateBackupCmd.Flags().String("profile", "essential", "backup profile: db, essential, all")
	migrateBackupCmd.Flags().StringSlice("include", nil, "only include these volume names")
	migrateBackupCmd.Flags().StringSlice("exclude", nil, "exclude these volume names")
	migrateBackupCmd.Flags().Int("concurrency", 4, "max parallel pg_dump operations")
	migrateBackupCmd.Flags().Bool("compress", false, "compress backup into a single .tar.gz archive")
}

func runMigrateBackup(cmd *cobra.Command, args []string) error {
	printer := newPrinter()

	outputDir, _ := cmd.Flags().GetString("output")
	force, _ := cmd.Flags().GetBool("force")
	profile, _ := cmd.Flags().GetString("profile")
	include, _ := cmd.Flags().GetStringSlice("include")
	exclude, _ := cmd.Flags().GetStringSlice("exclude")
	concurrency, _ := cmd.Flags().GetInt("concurrency")
	compress, _ := cmd.Flags().GetBool("compress")

	// Get absolute path
	absOutput, err := filepath.Abs(outputDir)
	if err != nil {
		return &output.CLIError{
			Summary:  "invalid output path",
			Detail:   err.Error(),
			ExitCode: output.ExitUsageError,
		}
	}

	printer.Header("Creating Backup")
	printer.Info("Profile: %s", profile)
	printer.Info("Output directory: %s", absOutput)
	if len(include) > 0 {
		printer.Info("Include: %v", include)
	}
	if len(exclude) > 0 {
		printer.Info("Exclude: %v", exclude)
	}
	fmt.Println()

	// Create migrator
	migrator := migrate.NewMigrator(
		getComposeDir(),
		"alt",
		logger,
		dryRun,
	)

	// Run backup
	result, err := migrator.Backup(cmd.Context(), migrate.BackupOptions{
		OutputDir:     absOutput,
		Force:         force,
		AltctlVersion: version,
		Profile:       migrate.BackupProfile(profile),
		Include:       include,
		Exclude:       exclude,
		Concurrency:   concurrency,
		Compress:      compress,
	})

	if err != nil {
		printer.Error("Backup failed: %v", err)
		return err
	}

	// Print per-volume timing
	fmt.Println()
	printer.Success("Backup complete!")
	fmt.Println()

	var totalSize int64
	for _, timing := range result.VolumeTimings {
		if timing.Error != nil {
			printer.Error("  %-30s  FAILED: %v", timing.Name, timing.Error)
			continue
		}
		totalSize += timing.Size
		printer.Info("  %-30s %8s  %s",
			timing.Name,
			migrate.FormatSize(timing.Size),
			timing.Elapsed.Round(time.Millisecond),
		)
	}

	fmt.Println()
	printer.Info("Volumes backed up: %d", len(result.Manifest.Volumes))
	printer.Info("Total size: %s", migrate.FormatSize(totalSize))
	printer.Info("Elapsed: %s", result.Elapsed.Round(time.Millisecond))
	printer.Info("Manifest checksum: %s", result.Manifest.Checksum)
	if result.ArchivePath != "" {
		printer.Info("Archive: %s", result.ArchivePath)
	}
	printer.PrintHints("migrate backup")

	return nil
}
