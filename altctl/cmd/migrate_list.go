package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/alt-project/altctl/internal/migrate"
	"github.com/alt-project/altctl/internal/output"
)

var migrateListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available backups",
	Long: `List all available backups in the specified directory.

Shows backup name, creation date, number of volumes, and total size.

Examples:
  altctl migrate list                    # List backups in ./backups/
  altctl migrate list --path /mnt/bak    # Custom backup directory`,
	RunE: runMigrateList,
}

func init() {
	migrateCmd.AddCommand(migrateListCmd)

	migrateListCmd.Flags().StringP("path", "p", "./backups", "directory containing backups")
}

func runMigrateList(cmd *cobra.Command, args []string) error {
	printer := newPrinter()

	backupPath, _ := cmd.Flags().GetString("path")

	// Get absolute path
	absPath, err := filepath.Abs(backupPath)
	if err != nil {
		return &output.CLIError{
			Summary:  "invalid path",
			Detail:   err.Error(),
			ExitCode: output.ExitUsageError,
		}
	}

	printer.Header("Available Backups")
	printer.Info("Directory: %s", absPath)
	fmt.Println()

	backups, err := migrate.ListBackups(absPath)
	if err != nil {
		return &output.CLIError{
			Summary:    "failed listing backups",
			Detail:     err.Error(),
			Suggestion: "Check backup directory path and permissions",
			ExitCode:   output.ExitGeneral,
		}
	}

	if len(backups) == 0 {
		printer.Warning("No backups found")
		return nil
	}

	// Create table
	table := output.NewTable([]string{"Name", "Created", "Volumes", "Size"})

	for _, b := range backups {
		table.AddRow([]string{
			b.Name,
			b.CreatedAt.Format("2006-01-02 15:04:05"),
			fmt.Sprintf("%d", b.VolumeCount),
			migrate.FormatSize(b.TotalSize),
		})
	}

	table.Render()

	fmt.Println()
	printer.Info("Total: %d backup(s)", len(backups))

	return nil
}
