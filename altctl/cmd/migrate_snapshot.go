package cmd

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/alt-project/altctl/internal/migrate"
	"github.com/alt-project/altctl/internal/output"
)

var migrateSnapshotCmd = &cobra.Command{
	Use:   "snapshot",
	Short: "Quick hot backup of all PostgreSQL databases",
	Long: `Create a fast backup of all PostgreSQL databases only.

This is a shortcut for 'altctl migrate backup --profile db --force' optimized
for quick database snapshots while services are running. PostgreSQL pg_dump
creates consistent snapshots even against a live database.

Backed up databases:
  - db_data_17 (main application database)
  - kratos_db_data (identity database)
  - recap_db_data (recap worker database)
  - rag_db_data (RAG database)
  - knowledge-sovereign-db-data (Knowledge Sovereign database)
  - pre_processor_db_data (pre-processor database)

Examples:
  altctl migrate snapshot                     # Quick DB snapshot
  altctl migrate snapshot --output /mnt/bak   # Custom output directory
  altctl migrate snapshot --concurrency 2     # Limit parallelism`,
	RunE: runMigrateSnapshot,
}

func init() {
	migrateCmd.AddCommand(migrateSnapshotCmd)

	migrateSnapshotCmd.Flags().StringP("output", "o", "./backups", "output directory for backups")
	migrateSnapshotCmd.Flags().Int("concurrency", 4, "max parallel pg_dump operations")
}

func runMigrateSnapshot(cmd *cobra.Command, args []string) error {
	printer := newPrinter()

	outputDir, _ := cmd.Flags().GetString("output")
	concurrency, _ := cmd.Flags().GetInt("concurrency")

	absOutput, err := filepath.Abs(outputDir)
	if err != nil {
		return &output.CLIError{
			Summary:  "invalid output path",
			Detail:   err.Error(),
			ExitCode: output.ExitUsageError,
		}
	}

	printer.Header("Database Snapshot")
	printer.Info("Profile: db (PostgreSQL databases only)")
	printer.Info("Output directory: %s", absOutput)
	printer.Info("Concurrency: %d", concurrency)
	fmt.Println()

	migrator := migrate.NewMigrator(
		getComposeDir(),
		"alt",
		logger,
		dryRun,
	)

	startTime := time.Now()

	result, err := migrator.Backup(cmd.Context(), migrate.BackupOptions{
		OutputDir:     absOutput,
		Force:         true, // Hot backup — always force
		AltctlVersion: version,
		Profile:       migrate.ProfileDB,
		Concurrency:   concurrency,
	})

	if err != nil {
		printer.Error("Snapshot failed: %v", err)
		return err
	}

	// Print per-volume timing
	fmt.Println()
	printer.Success("Snapshot complete!")
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

	elapsed := time.Since(startTime)
	fmt.Println()
	printer.Info("Databases: %d", len(result.Manifest.Volumes))
	printer.Info("Total size: %s", migrate.FormatSize(totalSize))
	printer.Info("Elapsed: %s", elapsed.Round(time.Millisecond))
	printer.PrintHints("migrate snapshot")

	return nil
}
