package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/alt-project/altctl/internal/output"
)

var retentionCmd = &cobra.Command{
	Use:   "retention",
	Short: "Manage event data retention and archival",
	Long: `Run retention archival, check eligible partitions, and view retention log.

Examples:
  altctl home retention status             # View retention log
  altctl home retention eligible           # List eligible partitions
  altctl home retention run                # Dry-run retention
  altctl home retention run --live         # Execute retention`,
}

var retentionRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run retention archival (dry-run by default)",
	RunE:  runRetentionRun,
}

func runRetentionRun(cmd *cobra.Command, args []string) error {
	client := newSovereignClient(cmd)
	live, _ := cmd.Flags().GetBool("live")

	reqBody := map[string]interface{}{
		"dry_run": !live,
	}

	var resp struct {
		Status         string `json:"status"`
		PartitionsRead int    `json:"partitions_read"`
		RowsExported   int64  `json:"rows_exported"`
		DryRun         bool   `json:"dry_run"`
		Message        string `json:"message"`
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if err := client.Post(ctx, "/admin/retention/run", reqBody, &resp); err != nil {
		return fmt.Errorf("run retention: %w", err)
	}

	printer := newPrinter()
	if resp.DryRun {
		printer.Info("Retention dry-run completed")
	} else {
		printer.Success("Retention executed")
	}

	table := output.NewTable([]string{"FIELD", "VALUE"})
	table.AddRow([]string{"Status", resp.Status})
	table.AddRow([]string{"Dry Run", fmt.Sprintf("%v", resp.DryRun)})
	table.AddRow([]string{"Partitions", fmt.Sprintf("%d", resp.PartitionsRead)})
	table.AddRow([]string{"Rows Exported", fmt.Sprintf("%d", resp.RowsExported)})
	if resp.Message != "" {
		table.AddRow([]string{"Message", resp.Message})
	}
	table.Render()

	return nil
}

var retentionStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show retention log",
	RunE:  runRetentionStatus,
}

func runRetentionStatus(cmd *cobra.Command, args []string) error {
	client := newSovereignClient(cmd)

	var resp struct {
		Logs []struct {
			LogID          string `json:"log_id"`
			Action         string `json:"action"`
			TargetTable    string `json:"target_table"`
			TargetPartition string `json:"target_partition"`
			RowsAffected   int64  `json:"rows_affected"`
			DryRun         bool   `json:"dry_run"`
			Status         string `json:"status"`
			RunAt          string `json:"run_at"`
		} `json:"logs"`
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := client.Get(ctx, "/admin/retention/status", &resp); err != nil {
		return fmt.Errorf("get retention status: %w", err)
	}

	printer := newPrinter()
	printer.Header("Retention Log")

	if len(resp.Logs) == 0 {
		printer.Info("No retention runs found")
		return nil
	}

	table := output.NewTable([]string{"ACTION", "TABLE", "PARTITION", "ROWS", "DRY RUN", "STATUS", "RUN AT"})
	for _, l := range resp.Logs {
		table.AddRow([]string{
			l.Action,
			l.TargetTable,
			l.TargetPartition,
			fmt.Sprintf("%d", l.RowsAffected),
			fmt.Sprintf("%v", l.DryRun),
			l.Status,
			l.RunAt,
		})
	}
	table.Render()

	return nil
}

var retentionEligibleCmd = &cobra.Command{
	Use:   "eligible",
	Short: "List partitions eligible for archival",
	RunE:  runRetentionEligible,
}

func runRetentionEligible(cmd *cobra.Command, args []string) error {
	client := newSovereignClient(cmd)

	var resp struct {
		Partitions []struct {
			TableName     string `json:"table_name"`
			PartitionName string `json:"partition_name"`
			RangeStart    string `json:"range_start"`
			RangeEnd      string `json:"range_end"`
			RowCount      int64  `json:"row_count"`
			SizeBytes     int64  `json:"size_bytes"`
		} `json:"partitions"`
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := client.Get(ctx, "/admin/retention/eligible", &resp); err != nil {
		return fmt.Errorf("get eligible partitions: %w", err)
	}

	printer := newPrinter()
	printer.Header("Eligible Partitions for Archive")

	if len(resp.Partitions) == 0 {
		printer.Info("No eligible partitions")
		return nil
	}

	table := output.NewTable([]string{"TABLE", "PARTITION", "RANGE START", "RANGE END", "ROWS", "SIZE"})
	for _, p := range resp.Partitions {
		size := fmt.Sprintf("%d B", p.SizeBytes)
		if p.SizeBytes > 1024*1024 {
			size = fmt.Sprintf("%.1f MB", float64(p.SizeBytes)/(1024*1024))
		}
		table.AddRow([]string{
			p.TableName,
			p.PartitionName,
			p.RangeStart,
			p.RangeEnd,
			fmt.Sprintf("%d", p.RowCount),
			size,
		})
	}
	table.Render()

	return nil
}

func init() {
	homeCmd.AddCommand(retentionCmd)

	for _, cmd := range []*cobra.Command{retentionRunCmd, retentionStatusCmd, retentionEligibleCmd} {
		addSovereignFlags(cmd)
	}

	retentionRunCmd.Flags().Bool("live", false, "execute retention (default is dry-run)")

	retentionCmd.AddCommand(retentionRunCmd)
	retentionCmd.AddCommand(retentionStatusCmd)
	retentionCmd.AddCommand(retentionEligibleCmd)
}
