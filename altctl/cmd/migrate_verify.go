package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/alt-project/altctl/internal/migrate"
	"github.com/alt-project/altctl/internal/output"
)

var migrateVerifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Verify backup integrity",
	Long: `Verify the integrity of a backup by checking:

  - Manifest file exists and is valid JSON
  - All backup files exist
  - File sizes match manifest
  - SHA256 checksums match

Examples:
  altctl migrate verify --backup ./backups/20251231_120000`,
	RunE: runMigrateVerify,
}

func init() {
	migrateCmd.AddCommand(migrateVerifyCmd)

	migrateVerifyCmd.Flags().StringP("backup", "b", "", "backup directory to verify (required)")
	_ = migrateVerifyCmd.MarkFlagRequired("backup")
}

func runMigrateVerify(cmd *cobra.Command, args []string) error {
	printer := output.NewPrinter(cfg.Output.Colors)

	backupDir, _ := cmd.Flags().GetString("backup")

	printer.Header("Verifying Backup")
	printer.Info("Backup directory: %s", backupDir)
	fmt.Println()

	// Verify backup
	manifest, err := migrate.VerifyBackup(backupDir)
	if err != nil {
		printer.Error("Verification FAILED: %v", err)
		return err
	}

	// Print verification results
	printer.Success("Backup integrity verified!")
	fmt.Println()

	printer.Info("Manifest version: %s", manifest.Version)
	printer.Info("Created: %s", manifest.CreatedAt.Format("2006-01-02 15:04:05 MST"))
	printer.Info("Altctl version: %s", manifest.AltctlVersion)
	printer.Info("Manifest checksum: %s", manifest.Checksum)
	fmt.Println()

	printer.Info("Volumes verified: %d", len(manifest.Volumes))

	var totalSize int64
	for _, v := range manifest.Volumes {
		totalSize += v.Size
		printer.Info("  ✓ %-30s %10s  %s",
			v.Name,
			migrate.FormatSize(v.Size),
			v.Checksum[:20]+"...",
		)
	}

	fmt.Println()
	printer.Info("Total size: %s", migrate.FormatSize(totalSize))

	return nil
}
