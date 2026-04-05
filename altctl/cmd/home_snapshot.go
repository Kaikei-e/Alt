package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/alt-project/altctl/internal/output"
)

var snapshotCmd = &cobra.Command{
	Use:   "snapshot",
	Short: "Manage Knowledge Home projection snapshots",
	Long: `Create, list, and inspect Knowledge Home projection snapshots.

Examples:
  altctl home snapshot list
  altctl home snapshot latest
  altctl home snapshot create`,
}

var snapshotListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all projection snapshots",
	RunE:  runSnapshotList,
}

func runSnapshotList(cmd *cobra.Command, args []string) error {
	client := newSovereignClient(cmd)

	var resp struct {
		Snapshots []struct {
			SnapshotID       string `json:"snapshot_id"`
			Status           string `json:"status"`
			ProjectionVersion int   `json:"projection_version"`
			EventSeqBoundary int64  `json:"event_seq_boundary"`
			ItemsRowCount    int    `json:"items_row_count"`
			SnapshotAt       string `json:"snapshot_at"`
		} `json:"snapshots"`
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := client.Get(ctx, "/admin/snapshots/list", &resp); err != nil {
		return fmt.Errorf("list snapshots: %w", err)
	}

	printer := newPrinter()
	printer.Header("Projection Snapshots")

	if len(resp.Snapshots) == 0 {
		printer.Info("No snapshots found")
		return nil
	}

	table := output.NewTable([]string{"SNAPSHOT ID", "STATUS", "VERSION", "EVENT SEQ", "ITEMS", "CREATED AT"})
	for _, s := range resp.Snapshots {
		table.AddRow([]string{
			s.SnapshotID,
			s.Status,
			fmt.Sprintf("%d", s.ProjectionVersion),
			fmt.Sprintf("%d", s.EventSeqBoundary),
			fmt.Sprintf("%d", s.ItemsRowCount),
			s.SnapshotAt,
		})
	}
	table.Render()

	return nil
}

var snapshotLatestCmd = &cobra.Command{
	Use:   "latest",
	Short: "Show the latest valid snapshot",
	RunE:  runSnapshotLatest,
}

func runSnapshotLatest(cmd *cobra.Command, args []string) error {
	client := newSovereignClient(cmd)

	var resp struct {
		SnapshotID        string `json:"snapshot_id"`
		Status            string `json:"status"`
		ProjectionVersion int    `json:"projection_version"`
		EventSeqBoundary  int64  `json:"event_seq_boundary"`
		ItemsRowCount     int    `json:"items_row_count"`
		DigestRowCount    int    `json:"digest_row_count"`
		RecallRowCount    int    `json:"recall_row_count"`
		SnapshotAt        string `json:"snapshot_at"`
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := client.Get(ctx, "/admin/snapshots/latest", &resp); err != nil {
		return fmt.Errorf("get latest snapshot: %w", err)
	}

	printer := newPrinter()
	printer.Header("Latest Snapshot")

	table := output.NewTable([]string{"FIELD", "VALUE"})
	table.AddRow([]string{"Snapshot ID", resp.SnapshotID})
	table.AddRow([]string{"Status", resp.Status})
	table.AddRow([]string{"Projection Version", fmt.Sprintf("%d", resp.ProjectionVersion)})
	table.AddRow([]string{"Event Seq Boundary", fmt.Sprintf("%d", resp.EventSeqBoundary)})
	table.AddRow([]string{"Items Rows", fmt.Sprintf("%d", resp.ItemsRowCount)})
	table.AddRow([]string{"Digest Rows", fmt.Sprintf("%d", resp.DigestRowCount)})
	table.AddRow([]string{"Recall Rows", fmt.Sprintf("%d", resp.RecallRowCount)})
	if resp.SnapshotAt != "" {
		table.AddRow([]string{"Created At", resp.SnapshotAt})
	}
	table.Render()

	return nil
}

var snapshotCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new projection snapshot",
	RunE:  runSnapshotCreate,
}

func runSnapshotCreate(cmd *cobra.Command, args []string) error {
	client := newSovereignClient(cmd)

	var resp struct {
		SnapshotID    string `json:"snapshot_id"`
		Status        string `json:"status"`
		ItemsRowCount int    `json:"items_row_count"`
		SnapshotAt    string `json:"snapshot_at"`
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := client.Post(ctx, "/admin/snapshots/create", map[string]interface{}{}, &resp); err != nil {
		return fmt.Errorf("create snapshot: %w", err)
	}

	printer := newPrinter()
	printer.Success("Snapshot created")

	table := output.NewTable([]string{"FIELD", "VALUE"})
	table.AddRow([]string{"Snapshot ID", resp.SnapshotID})
	table.AddRow([]string{"Status", resp.Status})
	table.AddRow([]string{"Items Rows", fmt.Sprintf("%d", resp.ItemsRowCount)})
	if resp.SnapshotAt != "" {
		table.AddRow([]string{"Created At", resp.SnapshotAt})
	}
	table.Render()

	return nil
}

func init() {
	homeCmd.AddCommand(snapshotCmd)

	for _, cmd := range []*cobra.Command{snapshotListCmd, snapshotLatestCmd, snapshotCreateCmd} {
		addSovereignFlags(cmd)
	}

	snapshotCmd.AddCommand(snapshotListCmd)
	snapshotCmd.AddCommand(snapshotLatestCmd)
	snapshotCmd.AddCommand(snapshotCreateCmd)
}
