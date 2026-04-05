package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/alt-project/altctl/internal/output"
)

var auditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Run a projection correctness audit",
	Long: `Sample Knowledge Home items and verify projection correctness against the event log.

Examples:
  altctl home audit
  altctl home audit --sample-size=200
  altctl home audit --projection-version=2`,
	RunE: runAudit,
}

func runAudit(cmd *cobra.Command, args []string) error {
	client, err := newAdminClient(cmd)
	if err != nil {
		return err
	}

	projName, _ := cmd.Flags().GetString("projection-name")
	projVersion, _ := cmd.Flags().GetString("projection-version")
	sampleSize, _ := cmd.Flags().GetInt32("sample-size")

	reqBody := map[string]interface{}{
		"projectionName":    projName,
		"projectionVersion": projVersion,
		"sampleSize":        sampleSize,
	}

	var resp struct {
		Audit struct {
			AuditID           string `json:"auditId"`
			ProjectionName    string `json:"projectionName"`
			ProjectionVersion string `json:"projectionVersion"`
			CheckedAt         string `json:"checkedAt"`
			SampleSize        int    `json:"sampleSize"`
			MismatchCount     int    `json:"mismatchCount"`
			DetailsJSON       string `json:"detailsJson"`
		} `json:"audit"`
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := client.Call(ctx, "RunProjectionAudit", reqBody, &resp); err != nil {
		return fmt.Errorf("run audit: %w", err)
	}

	printer := newPrinter()
	printer.Header("Projection Audit Result")

	a := resp.Audit
	table := output.NewTable([]string{"FIELD", "VALUE"})
	table.AddRow([]string{"Audit ID", a.AuditID})
	table.AddRow([]string{"Projection", a.ProjectionName})
	table.AddRow([]string{"Version", a.ProjectionVersion})
	table.AddRow([]string{"Sample Size", fmt.Sprintf("%d", a.SampleSize)})
	table.AddRow([]string{"Mismatches", fmt.Sprintf("%d", a.MismatchCount)})
	if a.CheckedAt != "" {
		table.AddRow([]string{"Checked At", a.CheckedAt})
	}
	table.Render()

	if a.MismatchCount > 0 {
		printer.Warning("Found %d mismatches", a.MismatchCount)
	} else {
		printer.Success("No mismatches detected")
	}

	return nil
}

func init() {
	homeCmd.AddCommand(auditCmd)
	addAdminFlags(auditCmd)

	auditCmd.Flags().String("projection-name", "knowledge_home_items", "projection table name")
	auditCmd.Flags().String("projection-version", "", "projection version to audit (default: active)")
	auditCmd.Flags().Int32("sample-size", 100, "number of items to sample")
}
