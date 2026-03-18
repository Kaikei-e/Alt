package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/alt-project/altctl/internal/output"
)

var homeSLOCmd = &cobra.Command{
	Use:   "slo",
	Short: "Show Knowledge Home SLO status",
	Long: `Display the current SLO (Service Level Objective) status for Knowledge Home.

Shows each SLI with its current value, target, compliance status, and error budget usage.

Example:
  altctl home slo
  altctl home slo --backend-url=http://my-backend:9001`,
	RunE: runHomeSLO,
}

type sloIndicator struct {
	Name       string  `json:"name"`
	Current    float64 `json:"current"`
	Target     float64 `json:"target"`
	Status     string  `json:"status"`
	BudgetUsed float64 `json:"budgetUsed"`
}

func runHomeSLO(cmd *cobra.Command, args []string) error {
	client, err := newAdminClient(cmd)
	if err != nil {
		return err
	}

	reqBody := map[string]interface{}{}
	var resp struct {
		Indicators []sloIndicator `json:"indicators"`
		UpdatedAt  string         `json:"updatedAt"`
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := client.Call(ctx, "GetSLOStatus", reqBody, &resp); err != nil {
		return fmt.Errorf("get SLO status: %w", err)
	}

	printer := newPrinter()
	printer.Header("Knowledge Home SLO Status")

	table := output.NewTable([]string{"SLI NAME", "CURRENT", "TARGET", "STATUS", "BUDGET USED"})
	for _, ind := range resp.Indicators {
		status := ind.Status
		budgetUsed := fmt.Sprintf("%.1f%%", ind.BudgetUsed*100)
		table.AddRow([]string{
			ind.Name,
			fmt.Sprintf("%.2f%%", ind.Current*100),
			fmt.Sprintf("%.2f%%", ind.Target*100),
			status,
			budgetUsed,
		})
	}
	table.Render()

	if resp.UpdatedAt != "" {
		fmt.Println()
		printer.Info("Last updated: %s", resp.UpdatedAt)
	}

	return nil
}

func init() {
	homeCmd.AddCommand(homeSLOCmd)

	homeSLOCmd.Flags().String("backend-url", "http://localhost:9001", "alt-backend admin API URL")
	homeSLOCmd.Flags().String("service-token", "", "service token (overrides SERVICE_TOKEN env var)")
}
