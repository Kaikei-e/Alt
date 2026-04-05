package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/alt-project/altctl/internal/output"
)

var homeFlagsCmd = &cobra.Command{
	Use:   "flags",
	Short: "Show Knowledge Home feature flags",
	Long: `Display current feature flag configuration for Knowledge Home.

Example:
  altctl home flags`,
	RunE: runHomeFlags,
}

func runHomeFlags(cmd *cobra.Command, args []string) error {
	client, err := newAdminClient(cmd)
	if err != nil {
		return err
	}

	reqBody := map[string]interface{}{}
	var resp struct {
		EnableHomePage      bool `json:"enableHomePage"`
		EnableTracking      bool `json:"enableTracking"`
		EnableProjectionV2  bool `json:"enableProjectionV2"`
		RolloutPercentage   int  `json:"rolloutPercentage"`
		EnableRecallRail    bool `json:"enableRecallRail"`
		EnableLens          bool `json:"enableLens"`
		EnableStreamUpdates bool `json:"enableStreamUpdates"`
		EnableSupersedeUx   bool `json:"enableSupersedeUx"`
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := client.Call(ctx, "GetFeatureFlags", reqBody, &resp); err != nil {
		return fmt.Errorf("get feature flags: %w", err)
	}

	printer := newPrinter()
	printer.Header("Knowledge Home Feature Flags")

	table := output.NewTable([]string{"FLAG", "VALUE"})
	table.AddRow([]string{"enable_home_page", fmt.Sprintf("%v", resp.EnableHomePage)})
	table.AddRow([]string{"enable_tracking", fmt.Sprintf("%v", resp.EnableTracking)})
	table.AddRow([]string{"enable_projection_v2", fmt.Sprintf("%v", resp.EnableProjectionV2)})
	table.AddRow([]string{"rollout_percentage", fmt.Sprintf("%d%%", resp.RolloutPercentage)})
	table.AddRow([]string{"enable_recall_rail", fmt.Sprintf("%v", resp.EnableRecallRail)})
	table.AddRow([]string{"enable_lens", fmt.Sprintf("%v", resp.EnableLens)})
	table.AddRow([]string{"enable_stream_updates", fmt.Sprintf("%v", resp.EnableStreamUpdates)})
	table.AddRow([]string{"enable_supersede_ux", fmt.Sprintf("%v", resp.EnableSupersedeUx)})
	table.Render()

	return nil
}

func init() {
	homeCmd.AddCommand(homeFlagsCmd)
	addAdminFlags(homeFlagsCmd)
}
