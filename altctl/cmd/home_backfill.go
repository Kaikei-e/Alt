package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/alt-project/altctl/internal/output"
)

var backfillCmd = &cobra.Command{
	Use:   "backfill",
	Short: "Manage Knowledge Home backfill jobs",
	Long: `Trigger, pause, resume, and check status of backfill jobs.

Examples:
  altctl home backfill trigger --projection-version=2
  altctl home backfill status --job-id=<uuid>
  altctl home backfill pause --job-id=<uuid>
  altctl home backfill resume --job-id=<uuid>`,
}

var backfillTriggerCmd = &cobra.Command{
	Use:   "trigger",
	Short: "Trigger a new backfill job",
	RunE:  runBackfillTrigger,
}

func runBackfillTrigger(cmd *cobra.Command, args []string) error {
	client, err := newAdminClient(cmd)
	if err != nil {
		return err
	}

	projVersion, _ := cmd.Flags().GetInt32("projection-version")

	reqBody := map[string]interface{}{
		"projectionVersion": projVersion,
	}

	var resp struct {
		Job struct {
			JobID             string `json:"jobId"`
			Status            string `json:"status"`
			ProjectionVersion int    `json:"projectionVersion"`
			CreatedAt         string `json:"createdAt"`
		} `json:"job"`
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := client.Call(ctx, "TriggerBackfill", reqBody, &resp); err != nil {
		return fmt.Errorf("trigger backfill: %w", err)
	}

	printer := newPrinter()
	printer.Success("Backfill triggered")

	table := output.NewTable([]string{"FIELD", "VALUE"})
	table.AddRow([]string{"Job ID", resp.Job.JobID})
	table.AddRow([]string{"Status", resp.Job.Status})
	table.AddRow([]string{"Projection Version", fmt.Sprintf("%d", resp.Job.ProjectionVersion)})
	if resp.Job.CreatedAt != "" {
		table.AddRow([]string{"Created At", resp.Job.CreatedAt})
	}
	table.Render()

	return nil
}

var backfillStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Get backfill job status",
	RunE:  runBackfillStatus,
}

func runBackfillStatus(cmd *cobra.Command, args []string) error {
	client, err := newAdminClient(cmd)
	if err != nil {
		return err
	}

	jobID, _ := cmd.Flags().GetString("job-id")
	if jobID == "" {
		return fmt.Errorf("required flag \"job-id\" not set")
	}

	reqBody := map[string]string{"jobId": jobID}
	var resp struct {
		Job struct {
			JobID           string `json:"jobId"`
			Status          string `json:"status"`
			TotalEvents     int    `json:"totalEvents"`
			ProcessedEvents int    `json:"processedEvents"`
			ErrorMessage    string `json:"errorMessage"`
			StartedAt       string `json:"startedAt"`
			CompletedAt     string `json:"completedAt"`
		} `json:"job"`
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := client.Call(ctx, "GetBackfillStatus", reqBody, &resp); err != nil {
		return fmt.Errorf("get backfill status: %w", err)
	}

	printer := newPrinter()
	printer.Header("Backfill Job Status")

	j := resp.Job
	table := output.NewTable([]string{"FIELD", "VALUE"})
	table.AddRow([]string{"Job ID", j.JobID})
	table.AddRow([]string{"Status", j.Status})
	table.AddRow([]string{"Total Events", fmt.Sprintf("%d", j.TotalEvents)})
	table.AddRow([]string{"Processed Events", fmt.Sprintf("%d", j.ProcessedEvents)})
	if j.TotalEvents > 0 {
		pct := float64(j.ProcessedEvents) / float64(j.TotalEvents) * 100
		table.AddRow([]string{"Progress", fmt.Sprintf("%.1f%%", pct)})
	}
	if j.ErrorMessage != "" {
		table.AddRow([]string{"Error", j.ErrorMessage})
	}
	if j.StartedAt != "" {
		table.AddRow([]string{"Started At", j.StartedAt})
	}
	if j.CompletedAt != "" {
		table.AddRow([]string{"Completed At", j.CompletedAt})
	}
	table.Render()

	return nil
}

var backfillPauseCmd = &cobra.Command{
	Use:   "pause",
	Short: "Pause a running backfill job",
	RunE:  runBackfillPause,
}

func runBackfillPause(cmd *cobra.Command, args []string) error {
	client, err := newAdminClient(cmd)
	if err != nil {
		return err
	}

	jobID, _ := cmd.Flags().GetString("job-id")
	if jobID == "" {
		return fmt.Errorf("required flag \"job-id\" not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := client.Call(ctx, "PauseBackfill", map[string]string{"jobId": jobID}, nil); err != nil {
		return fmt.Errorf("pause backfill: %w", err)
	}

	printer := newPrinter()
	printer.Success("Backfill paused: %s", jobID)
	return nil
}

var backfillResumeCmd = &cobra.Command{
	Use:   "resume",
	Short: "Resume a paused backfill job",
	RunE:  runBackfillResume,
}

func runBackfillResume(cmd *cobra.Command, args []string) error {
	client, err := newAdminClient(cmd)
	if err != nil {
		return err
	}

	jobID, _ := cmd.Flags().GetString("job-id")
	if jobID == "" {
		return fmt.Errorf("required flag \"job-id\" not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := client.Call(ctx, "ResumeBackfill", map[string]string{"jobId": jobID}, nil); err != nil {
		return fmt.Errorf("resume backfill: %w", err)
	}

	printer := newPrinter()
	printer.Success("Backfill resumed: %s", jobID)
	return nil
}

func init() {
	homeCmd.AddCommand(backfillCmd)

	for _, cmd := range []*cobra.Command{backfillTriggerCmd, backfillStatusCmd, backfillPauseCmd, backfillResumeCmd} {
		addAdminFlags(cmd)
	}

	backfillTriggerCmd.Flags().Int32("projection-version", 1, "projection version to backfill")

	for _, cmd := range []*cobra.Command{backfillStatusCmd, backfillPauseCmd, backfillResumeCmd} {
		cmd.Flags().String("job-id", "", "backfill job ID (required)")
	}

	backfillCmd.AddCommand(backfillTriggerCmd)
	backfillCmd.AddCommand(backfillStatusCmd)
	backfillCmd.AddCommand(backfillPauseCmd)
	backfillCmd.AddCommand(backfillResumeCmd)
}
