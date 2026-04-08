package cmd

import (
	"github.com/spf13/cobra"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Backup and restore persistent data for migration",
	Long: `Backup and restore Docker Compose volumes with profile-based filtering.

Profiles control which volumes are included:
  db         PostgreSQL databases only (6 volumes, fastest)
  essential  Critical + operational + search data (10 volumes, no metrics/models)
  all        All registered volumes (14 volumes, complete backup)

PostgreSQL databases (pg_dump, parallel):
  - db_data_17, kratos_db_data, recap_db_data, rag_db_data
  - knowledge-sovereign-db-data, pre_processor_db_data

Example usage:
  altctl migrate snapshot                           # Quick DB-only hot backup
  altctl migrate backup --profile db --force        # DB-only backup
  altctl migrate backup --force                     # Essential profile (default)
  altctl migrate backup --profile all --force       # Complete backup
  altctl migrate restore --from ./backups/xxx --force
  altctl migrate restore --from ./backups/xxx --profile db --force
  altctl migrate list                               # List available backups
  altctl migrate verify --backup ./backups/xxx      # Verify backup integrity`,
}

func init() {
	rootCmd.AddCommand(migrateCmd)
}
