package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/alt-project/altctl/internal/output"
)

var storageCmd = &cobra.Command{
	Use:   "storage",
	Short: "Show Knowledge Home table storage statistics",
	Long: `Display storage size and row counts for all knowledge tables.

Example:
  altctl home storage`,
	RunE: runStorage,
}

func runStorage(cmd *cobra.Command, args []string) error {
	client := newSovereignClient(cmd)

	var resp struct {
		Tables []struct {
			Name       string `json:"name"`
			TotalSize  string `json:"total_size"`
			TableSize  string `json:"table_size"`
			IndexSize  string `json:"index_size"`
			RowCount   int64  `json:"row_count"`
		} `json:"tables"`
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := client.Get(ctx, "/admin/storage/stats", &resp); err != nil {
		return fmt.Errorf("get storage stats: %w", err)
	}

	printer := newPrinter()
	printer.Header("Knowledge Table Storage")

	if len(resp.Tables) == 0 {
		printer.Info("No tables found")
		return nil
	}

	table := output.NewTable([]string{"TABLE", "TOTAL SIZE", "TABLE SIZE", "INDEX SIZE", "ROWS"})
	for _, t := range resp.Tables {
		table.AddRow([]string{
			t.Name,
			t.TotalSize,
			t.TableSize,
			t.IndexSize,
			fmt.Sprintf("%d", t.RowCount),
		})
	}
	table.Render()

	return nil
}

func init() {
	homeCmd.AddCommand(storageCmd)
	addSovereignFlags(storageCmd)
}
