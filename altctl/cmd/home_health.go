package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/alt-project/altctl/internal/output"
)

var homeHealthCmd = &cobra.Command{
	Use:   "health",
	Short: "Show Knowledge Home projection health",
	Long: `Display projection health including active version, checkpoint, and backfill jobs.

Example:
  altctl home health`,
	RunE: runHomeHealth,
}

func runHomeHealth(cmd *cobra.Command, args []string) error {
	client, err := newAdminClient(cmd)
	if err != nil {
		return err
	}

	reqBody := map[string]interface{}{}
	var resp struct {
		ActiveVersion int    `json:"activeVersion"`
		CheckpointSeq int64  `json:"checkpointSeq"`
		LastUpdated   string `json:"lastUpdated"`
		BackfillJobs  []struct {
			JobID             string `json:"jobId"`
			Status            string `json:"status"`
			ProjectionVersion int    `json:"projectionVersion"`
			TotalEvents       int    `json:"totalEvents"`
			ProcessedEvents   int    `json:"processedEvents"`
		} `json:"backfillJobs"`
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := client.Call(ctx, "GetProjectionHealth", reqBody, &resp); err != nil {
		return fmt.Errorf("get projection health: %w", err)
	}

	printer := newPrinter()
	printer.Header("Projection Health")

	table := output.NewTable([]string{"FIELD", "VALUE"})
	table.AddRow([]string{"Active Version", fmt.Sprintf("%d", resp.ActiveVersion)})
	table.AddRow([]string{"Checkpoint Seq", fmt.Sprintf("%d", resp.CheckpointSeq)})
	if resp.LastUpdated != "" {
		table.AddRow([]string{"Last Updated", resp.LastUpdated})
	}
	table.Render()

	if len(resp.BackfillJobs) > 0 {
		fmt.Println()
		printer.Header("Backfill Jobs")

		jobTable := output.NewTable([]string{"JOB ID", "STATUS", "VERSION", "PROGRESS"})
		for _, j := range resp.BackfillJobs {
			progress := "-"
			if j.TotalEvents > 0 {
				progress = fmt.Sprintf("%d/%d (%.1f%%)", j.ProcessedEvents, j.TotalEvents,
					float64(j.ProcessedEvents)/float64(j.TotalEvents)*100)
			}
			jobTable.AddRow([]string{
				j.JobID,
				j.Status,
				fmt.Sprintf("%d", j.ProjectionVersion),
				progress,
			})
		}
		jobTable.Render()
	}

	return nil
}

func init() {
	homeCmd.AddCommand(homeHealthCmd)
	addAdminFlags(homeHealthCmd)
}
