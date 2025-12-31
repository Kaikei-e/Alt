package cmd

import (
	"github.com/spf13/cobra"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Backup and restore persistent data for migration",
	Long: `Backup and restore Docker Compose volumes for "Shikinen Sengu" migration.

This command provides full backup and restore capabilities for all persistent
volumes in the Alt platform, enabling safe execution of 'docker compose down -v'.

Supported volumes:
  PostgreSQL databases (pg_dump):
    - db_data_17 (main application database)
    - kratos_db_data (identity database)
    - recap_db_data (recap worker database)
    - rag_db_data (RAG database)

  Tar-based volumes:
    - meili_data (Meilisearch index)
    - clickhouse_data (ClickHouse analytics)
    - news_creator_models (Ollama LLM models)
    - rask_log_aggregator_data (log aggregator)
    - oauth_token_data (OAuth tokens)

Example usage:
  altctl migrate backup                    # Create full backup
  altctl migrate backup --output ./bak     # Custom output directory
  altctl migrate restore --from ./bak/xxx  # Restore from backup
  altctl migrate list                      # List available backups
  altctl migrate verify --backup ./bak/xxx # Verify backup integrity`,
}

func init() {
	rootCmd.AddCommand(migrateCmd)
}
