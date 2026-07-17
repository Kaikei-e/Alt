package cmd

import (
	"fmt"

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
	reqBody := map[string]interface{}{}
	var resp struct {
		Indicators []sloIndicator `json:"indicators"`
		UpdatedAt  string         `json:"updatedAt"`
	}

	if err := callAdminRPC(cmd, "GetSLOStatus", reqBody, &resp); err != nil {
		return err
	}

	printer := newPrinter()
	printer.Header("Knowledge Home SLO Status")

	table := output.NewTable([]string{"SLI NAME", "CURRENT", "TARGET", "STATUS", "BUDGET USED"})
	for _, ind := range resp.Indicators {
		table.AddRow([]string{
			ind.Name,
			fmt.Sprintf("%.2f%%", ind.Current*100),
			fmt.Sprintf("%.2f%%", ind.Target*100),
			ind.Status,
			fmt.Sprintf("%.1f%%", ind.BudgetUsed*100),
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
	addAdminFlags(homeSLOCmd)
}
