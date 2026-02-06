package cmd

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/alt-project/altctl/internal/migrate"
	"github.com/alt-project/altctl/internal/output"
)

var migrateStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show backup status and health",
	Long: `Show the current backup status including:

  - Latest backup timestamp and age
  - Expected vs actual volume count
  - Missing volumes
  - Total backup size
  - Checksum verification status
  - Overall health assessment

The health is determined by:
  - GOOD:     Recent backup with all volumes and valid checksums
  - WARNING:  Backup is stale (>25h), has missing volumes, or checksum issues
  - CRITICAL: No backups found or backup is very stale (>50h)

Examples:
  altctl migrate status                    # Check status in ./backups/
  altctl migrate status --path /mnt/bak    # Custom backup directory`,
	RunE: runMigrateStatus,
}

func init() {
	migrateCmd.AddCommand(migrateStatusCmd)

	migrateStatusCmd.Flags().StringP("path", "p", "./backups", "directory containing backups")
}

func runMigrateStatus(cmd *cobra.Command, args []string) error {
	printer := newPrinter()

	backupPath, _ := cmd.Flags().GetString("path")

	absPath, err := filepath.Abs(backupPath)
	if err != nil {
		return &output.CLIError{
			Summary:  "invalid path",
			Detail:   err.Error(),
			ExitCode: output.ExitUsageError,
		}
	}

	printer.Header("Backup Status")
	printer.Info("Directory: %s", absPath)
	fmt.Println()

	status, err := migrate.GetBackupStatus(absPath)
	if err != nil {
		return &output.CLIError{
			Summary:    "failed checking backup status",
			Detail:     err.Error(),
			Suggestion: "Check backup directory path and permissions",
			ExitCode:   output.ExitGeneral,
		}
	}

	// Health banner
	switch status.Health {
	case migrate.HealthGood:
		printer.Success("Health: %s", status.Health)
	case migrate.HealthWarning:
		printer.Warning("Health: %s", status.Health)
	case migrate.HealthCritical:
		printer.Error("Health: %s", status.Health)
	}
	fmt.Println()

	if !status.HasBackup {
		printer.Warning("No backups found")
		printer.Info("Run: altctl migrate backup")
		printer.PrintHints("migrate status")
		return nil
	}

	// Latest backup info
	printer.Info("Latest backup:  %s", status.LatestBackup)
	printer.Info("Created:        %s", status.LatestTimestamp.Format("2006-01-02 15:04:05 MST"))
	printer.Info("Age:            %s", formatDuration(status.Age))
	fmt.Println()

	// Volume info
	printer.Info("Volumes:        %d / %d", status.ActualVolumes, status.ExpectedVolumes)
	printer.Info("Total size:     %s", migrate.FormatSize(status.TotalSize))

	if status.ChecksumOK {
		printer.Success("Checksums:      OK")
	} else {
		printer.Error("Checksums:      FAILED")
	}

	// Missing volumes
	if len(status.MissingVolumes) > 0 {
		fmt.Println()
		printer.Warning("Missing volumes:")
		for _, v := range status.MissingVolumes {
			printer.Warning("  - %s", v)
		}
	}

	printer.PrintHints("migrate status")

	return nil
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}

	hours := int(d.Hours())
	if hours < 24 {
		return fmt.Sprintf("%dh %dm", hours, int(d.Minutes())%60)
	}

	days := hours / 24
	remainingHours := hours % 24
	parts := []string{fmt.Sprintf("%dd", days)}
	if remainingHours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", remainingHours))
	}
	return strings.Join(parts, " ")
}
